package web

import (
	"bytes"
	"cmp"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var weekTemplate = template.Must(template.ParseFS(templateFS, "templates/week.html", "templates/panel.html"))

var printTemplate = template.Must(template.ParseFS(templateFS, "templates/print.html"))

type handler struct {
	now       func() time.Time
	db        *gorm.DB
	validate  *validator.Validate
	schedules []masses.Schedule
	slotsMu   sync.Mutex
}

func RegisterRoutes(router chi.Router, db *gorm.DB, schedules []masses.Schedule) {
	h := &handler{now: time.Now, db: db, validate: intentions.NewValidator(), schedules: schedules}

	router.Get("/", h.week)
	router.Post("/intentions", h.create)
	router.Post("/intentions/{id}", h.update)
	router.Post("/intentions/{id}/delete", h.delete)
	router.Post("/slots", h.createSlots)
	router.Post("/clear", h.clearWeek)
	router.Get("/pdf", h.pdf)
	router.Get("/print", h.printPreview)
	router.Handle("/static/*", http.FileServerFS(staticFS))
}

func (h *handler) week(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	start, err := h.weekStart(query)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	var state weekState
	switch {
	case query.Get("delete") != "":
		intention, ok := h.findIntention(w, query.Get("delete"))
		if !ok {
			return
		}

		d := newDeleteView(start, intention)
		state.delete = &d
	case query.Get("edit") != "":
		intention, ok := h.findIntention(w, query.Get("edit"))
		if !ok {
			return
		}

		at := intention.MassAt.In(intentions.TimeZone)
		day := cmp.Or(query.Get("day"), at.Format(intentions.DateLayout))
		massTime := cmp.Or(query.Get("time"), at.Format("15:04"))

		f := newFormView(start, day, massTime)
		f.EditID = intention.ID
		f.Content = intention.Content
		f.IsOutsideParish = intention.IsOutsideParish
		state.form = &f
	case query.Get("new") == "1":
		f := newFormView(start, query.Get("day"), query.Get("time"))
		state.form = &f
	case query.Get("clear") == "1":
		state.clear = true
	}

	state.saved, _ = strconv.Atoi(query.Get("saved"))

	h.renderWeek(w, http.StatusOK, start, state)
}

func (h *handler) weekStart(query url.Values) (time.Time, error) {
	param := query.Get("week")
	if param == "" {
		return intentions.NextWeekStart(h.now()), nil
	}

	day, err := time.ParseInLocation(intentions.DateLayout, param, intentions.TimeZone)
	if err != nil {
		return time.Time{}, err
	}

	return intentions.WeekStart(day), nil
}

func (h *handler) printPreview(w http.ResponseWriter, r *http.Request) {
	start, err := h.weekStart(r.URL.Query())
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	render(w, http.StatusOK, printTemplate, newWeekView(start, nil))
}

func (h *handler) pdf(w http.ResponseWriter, r *http.Request) {
	start, err := h.weekStart(r.URL.Query())
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	var buf bytes.Buffer
	err = writeWeekPDF(&buf, newWeekView(start, list))
	if err != nil {
		log.Printf("error generating pdf for week %s: %v", start.Format(intentions.DateLayout), err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	filename := "intencje-" + start.Format(intentions.DateLayout) + ".pdf"
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)

	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, intentions.Intention{})
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	h.save(w, r, intention)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	err := h.db.Delete(&intention).Error
	if err != nil {
		log.Printf("error deleting intention %d: %v", intention.ID, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	week := intentions.WeekStart(intention.MassAt).Format(intentions.DateLayout)
	http.Redirect(w, r, "/?week="+week, http.StatusSeeOther)
}

func (h *handler) clearWeek(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	err = intentions.DeleteWeek(h.db, start)
	if err != nil {
		log.Printf("error clearing week %s: %v", start.Format(intentions.DateLayout), err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout), http.StatusSeeOther)
}

func postedWeekStart(r *http.Request) (time.Time, error) {
	day, err := time.ParseInLocation(intentions.DateLayout, r.PostForm.Get("week"), intentions.TimeZone)
	if err != nil {
		return time.Time{}, err
	}

	return intentions.WeekStart(day), nil
}

func (h *handler) createSlots(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	name := r.PostForm.Get("schedule")
	i := slices.IndexFunc(h.schedules, func(s masses.Schedule) bool {
		return s.Name == name
	})
	if i < 0 {
		http.Error(w, "nieznany porządek mszy", http.StatusBadRequest)
		return
	}

	h.slotsMu.Lock()
	defer h.slotsMu.Unlock()

	existing, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	slots := h.schedules[i].Slots(start, existing)
	if len(slots) > 0 {
		err = h.db.Create(&slots).Error
		if err != nil {
			log.Printf("error creating slots for week %s: %v", start.Format(intentions.DateLayout), err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	err = settings.SaveSchedule(h.db, name)
	if err != nil {
		log.Printf("error saving selected schedule %q: %v", name, err)
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout), http.StatusSeeOther)
}

func (h *handler) findIntention(w http.ResponseWriter, idParam string) (intentions.Intention, bool) {
	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator intencji", http.StatusBadRequest)
		return intentions.Intention{}, false
	}

	var intention intentions.Intention
	err = h.db.First(&intention, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "nie znaleziono intencji", http.StatusNotFound)
		return intentions.Intention{}, false
	}
	if err != nil {
		log.Printf("error finding intention %d: %v", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return intentions.Intention{}, false
	}

	return intention, true
}

func (h *handler) save(w http.ResponseWriter, r *http.Request, intention intentions.Intention) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	day := r.PostForm.Get("day")
	massTime := r.PostForm.Get("time")
	content := r.PostForm.Get("content")
	outside := r.PostForm.Get("outside") == "on"

	input := intentions.Input{
		MassAt:          day + " " + massTime,
		Content:         strings.TrimSpace(content),
		IsOutsideParish: outside,
	}

	err = h.validate.Struct(input)
	messages, ok := intentions.ValidationMessages(err)
	if err != nil && !ok {
		log.Printf("unexpected validation error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	if errs := formErrors(day, massTime, messages); len(errs) > 0 {
		start := intentions.NextWeekStart(h.now())
		if week, parseErr := time.ParseInLocation(intentions.DateLayout, r.PostForm.Get("week"), intentions.TimeZone); parseErr == nil {
			start = intentions.WeekStart(week)
		}

		form := newFormView(start, day, massTime)
		form.Content = content
		form.IsOutsideParish = outside
		form.Errors = errs
		form.EditID = intention.ID

		h.renderWeek(w, http.StatusUnprocessableEntity, start, weekState{form: &form})
		return
	}

	intention.Apply(input)

	err = h.db.Save(&intention).Error
	if err != nil {
		log.Printf("error saving intention %d: %v", intention.ID, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	week := intentions.WeekStart(intention.MassAt).Format(intentions.DateLayout)
	http.Redirect(w, r, fmt.Sprintf("/?week=%s&saved=%d#i-%d", week, intention.ID, intention.ID), http.StatusSeeOther)
}

func formErrors(day, massTime string, messages map[string]string) map[string]string {
	errs := make(map[string]string)

	if day == "" {
		errs["day"] = intentions.RequiredMessage
	}
	if massTime == "" {
		errs["time"] = intentions.RequiredMessage
	}
	if message, ok := messages["mass_at"]; ok && day != "" && massTime != "" {
		errs["time"] = message
	}
	if message, ok := messages["content"]; ok {
		errs["content"] = message
	}

	return errs
}

type weekState struct {
	form   *formView
	delete *deleteView
	clear  bool
	saved  int
}

func (h *handler) renderWeek(w http.ResponseWriter, status int, start time.Time, state weekState) {
	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	view := newWeekView(start, list)
	view.IsUpcoming = start.Equal(intentions.NextWeekStart(h.now()))
	view.Schedules = h.schedules

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}
	view.SelectedSchedule = setting.SelectedSchedule

	if state.form != nil {
		view.Form = state.form
		markSelected(view.Days, state.form.Day, state.form.Time)
		markEditing(view.Days, state.form.EditID)
	}
	view.Delete = state.delete
	if state.clear && len(list) > 0 {
		c := newClearView(list)
		view.Clear = &c
	}
	markSaved(view.Days, state.saved)

	render(w, status, weekTemplate, view)
}

func render(w http.ResponseWriter, status int, tmpl *template.Template, data any) {
	var buf bytes.Buffer

	err := tmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("error rendering template %s: %v", tmpl.Name(), err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}
