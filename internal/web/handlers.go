package web

import (
	"bytes"
	"cmp"
	"context"
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
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"go-parafia/internal/calendar"
	"go-parafia/internal/gcatholic"
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

var settingsTemplate = template.Must(template.ParseFS(templateFS, "templates/settings.html"))

var scheduleTemplate = template.Must(template.ParseFS(templateFS, "templates/schedule.html"))

type handler struct {
	now               func() time.Time
	db                *gorm.DB
	validate          *validator.Validate
	slotsMu           sync.Mutex
	fetchCelebrations func(ctx context.Context, year int) ([]gcatholic.Celebration, error)
}

func RegisterRoutes(router chi.Router, db *gorm.DB) {
	client := &http.Client{Timeout: 20 * time.Second}
	h := &handler{
		now:      time.Now,
		db:       db,
		validate: intentions.NewValidator(),
		fetchCelebrations: func(ctx context.Context, year int) ([]gcatholic.Celebration, error) {
			return gcatholic.Fetch(ctx, client, gcatholic.BaseURL, year)
		},
	}

	router.Get("/", h.week)
	router.Post("/intentions", h.create)
	router.Post("/intentions/{id}", h.update)
	router.Post("/intentions/{id}/delete", h.delete)
	router.Post("/slots", h.createSlots)
	router.Post("/clear", h.clearWeek)
	router.Post("/days/{date}", h.saveFeast)
	router.Post("/feasts/import", h.importFeasts)
	router.Get("/settings", h.settingsPage)
	router.Post("/settings", h.saveSettings)
	router.Get("/settings/schedules/new", h.scheduleForm)
	router.Get("/settings/schedules/{id}", h.scheduleForm)
	router.Post("/settings/schedules", h.saveSchedule)
	router.Post("/settings/schedules/restore", h.restoreSchedules)
	router.Post("/settings/schedules/{id}", h.saveSchedule)
	router.Post("/settings/schedules/{id}/delete", h.deleteSchedule)
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
	case query.Get("feast") != "":
		day, err := time.ParseInLocation(intentions.DateLayout, query.Get("feast"), intentions.TimeZone)
		if err != nil {
			http.Error(w, "nieprawidłowa data dnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
			return
		}

		f := newFeastView(day)
		f.Feast, err = calendar.Feast(h.db, day.Format(intentions.DateLayout))
		if err != nil {
			log.Printf("error loading feast for %s: %v", day.Format(intentions.DateLayout), err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		state.feast = &f
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

	feasts, err := calendar.FeastsForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week feasts: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	view := newWeekView(start, list)
	setFeasts(view.Days, feasts)
	view.ParishName = setting.ParishName
	view.Address = setting.Address

	var buf bytes.Buffer
	err = writeWeekPDF(&buf, view)
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

func (h *handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	query := r.URL.Query()
	view, ok := h.settingsView(w, setting, query.Get("week"))
	if !ok {
		return
	}
	view.Saved = query.Get("saved") == "1"

	closeURL := settingsURL(view.Week, false) + "#porzadki"
	switch {
	case query.Get("delete_schedule") != "":
		id, err := strconv.Atoi(query.Get("delete_schedule"))
		if err != nil {
			http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
			return
		}

		schedule, err := masses.Find(h.db, id)
		if errors.Is(err, masses.ErrScheduleNotFound) {
			http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("error loading schedule %d: %v", id, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}

		view.Dialog = &settingsDialog{
			Title:    "Usunąć porządek mszy?",
			Text:     fmt.Sprintf("„%s” (%s). Intencje i godziny już wczytane do tygodni zostaną bez zmian.", schedule.Name, scheduleSummary(schedule)),
			Action:   "/settings/schedules/" + strconv.Itoa(id) + "/delete",
			Button:   "Usuń porządek",
			Week:     view.Week,
			CloseURL: closeURL,
		}
	case query.Get("restore") == "1":
		view.Dialog = &settingsDialog{
			Title:    "Przywrócić domyślne porządki?",
			Text:     "Wszystkie porządki mszy zostaną zastąpione domyślnymi z programu. Własne zmiany w porządkach przepadną.",
			Action:   "/settings/schedules/restore",
			Button:   "Przywróć domyślne",
			Week:     view.Week,
			CloseURL: closeURL,
		}
	}

	render(w, http.StatusOK, settingsTemplate, view)
}

func (h *handler) settingsView(w http.ResponseWriter, setting settings.Setting, week string) (settingsView, bool) {
	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return settingsView{}, false
	}

	return newSettingsView(setting, schedules, week), true
}

func (h *handler) scheduleForm(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week")

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		render(w, http.StatusOK, scheduleTemplate, newScheduleFormView(masses.Schedule{}, week))
		return
	}

	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return
	}

	schedule, err := masses.Find(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error loading schedule %d: %v", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	render(w, http.StatusOK, scheduleTemplate, newScheduleFormView(schedule, week))
}

const duplicateScheduleMessage = "Porządek o tej nazwie już istnieje."

func (h *handler) saveSchedule(w http.ResponseWriter, r *http.Request) {
	id := 0
	if idParam := chi.URLParam(r, "id"); idParam != "" {
		var err error
		id, err = strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
			return
		}
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := r.PostForm.Get("week")

	schedule, errs := scheduleFromForm(r.PostForm)
	schedule.ID = id
	if errs["name"] == "" {
		taken, err := masses.NameTaken(h.db, schedule.Name, id)
		if err != nil {
			log.Printf("error checking schedule name: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if taken {
			errs["name"] = duplicateScheduleMessage
		}
	}
	if len(errs) == 0 {
		_, err = masses.Save(h.db, schedule)
		switch {
		case errors.Is(err, masses.ErrDuplicateName):
			errs["name"] = duplicateScheduleMessage
		case errors.Is(err, masses.ErrScheduleNotFound):
			http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
			return
		case err != nil:
			log.Printf("error saving schedule %d: %v", id, err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	if len(errs) > 0 {
		view := newScheduleFormView(schedule, week)
		view.Name = r.PostForm.Get("name")
		for i := range view.Days {
			view.Days[i].Value = r.PostForm.Get(view.Days[i].Field)
			view.Days[i].Error = errs[view.Days[i].Field]
		}
		view.Errors = errs
		render(w, http.StatusUnprocessableEntity, scheduleTemplate, view)
		return
	}

	http.Redirect(w, r, settingsURL(week, true)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	err = masses.Delete(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error deleting schedule %d: %v", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, settingsURL(r.PostForm.Get("week"), false)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) restoreSchedules(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	err = masses.RestoreDefaults(h.db)
	if err != nil {
		log.Printf("error restoring default schedules: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, settingsURL(r.PostForm.Get("week"), false)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	setting := settingsFromForm(r.PostForm)
	view, ok := h.settingsView(w, setting, r.PostForm.Get("week"))
	if !ok {
		return
	}

	if errs := settingsErrors(setting); len(errs) > 0 {
		view.Errors = errs
		render(w, http.StatusUnprocessableEntity, settingsTemplate, view)
		return
	}

	err = settings.SaveParish(h.db, setting)
	if err != nil {
		log.Printf("error saving settings: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, settingsURL(view.Week, true), http.StatusSeeOther)
}

const importErrorMessage = "Nie udało się pobrać kalendarza z GCatholic. Sprawdź połączenie z internetem i spróbuj ponownie."

func (h *handler) importFeasts(w http.ResponseWriter, r *http.Request) {
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

	years := []int{start.Year()}
	if end := start.AddDate(0, 0, 6); end.Year() != start.Year() {
		years = append(years, end.Year())
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	feasts := make(map[string]string)
	for _, year := range years {
		celebrations, err := h.fetchCelebrations(r.Context(), year)
		if err != nil {
			log.Printf("error fetching GCatholic calendar %d: %v", year, err)
			h.renderWeek(w, http.StatusBadGateway, start, weekState{importError: importErrorMessage})
			return
		}

		celebrations = slices.DeleteFunc(celebrations, func(c gcatholic.Celebration) bool {
			return skipped(setting.FeastImport, c.Kind())
		})
		texts := gcatholic.Texts(celebrations)
		for i := range 7 {
			date := start.AddDate(0, 0, i).Format(intentions.DateLayout)
			if text, ok := texts[date]; ok {
				feasts[date] = text
			}
		}
	}

	err = calendar.AddMissing(h.db, feasts)
	if err != nil {
		log.Printf("error saving imported feasts for week %s: %v", start.Format(intentions.DateLayout), err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout), http.StatusSeeOther)
}

func (h *handler) saveFeast(w http.ResponseWriter, r *http.Request) {
	day, err := time.ParseInLocation(intentions.DateLayout, chi.URLParam(r, "date"), intentions.TimeZone)
	if err != nil {
		http.Error(w, "nieprawidłowa data dnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	date := day.Format(intentions.DateLayout)
	start := intentions.WeekStart(day)
	feast := strings.TrimSpace(r.PostForm.Get("feast"))

	if utf8.RuneCountInString(feast) > maxFeastLength {
		f := newFeastView(day)
		f.Feast = feast
		f.Error = fmt.Sprintf("Maksymalnie %d znaków.", maxFeastLength)
		h.renderWeek(w, http.StatusUnprocessableEntity, start, weekState{feast: &f})
		return
	}

	err = calendar.SaveFeast(h.db, date, feast)
	if err != nil {
		log.Printf("error saving feast for %s: %v", date, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout)+"#d-"+date, http.StatusSeeOther)
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

	id, err := strconv.Atoi(r.PostForm.Get("schedule"))
	if err != nil {
		http.Error(w, "nieznany porządek mszy", http.StatusBadRequest)
		return
	}

	schedule, err := masses.Find(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nieznany porządek mszy", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("error loading schedule %d: %v", id, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
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

	slots := schedule.Slots(start, existing)
	if len(slots) > 0 {
		err = h.db.Create(&slots).Error
		if err != nil {
			log.Printf("error creating slots for week %s: %v", start.Format(intentions.DateLayout), err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
	}

	err = settings.SaveSchedule(h.db, schedule.ID)
	if err != nil {
		log.Printf("error saving selected schedule %d: %v", schedule.ID, err)
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
	form        *formView
	delete      *deleteView
	feast       *feastView
	clear       bool
	saved       int
	importError string
}

func (h *handler) renderWeek(w http.ResponseWriter, status int, start time.Time, state weekState) {
	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	feasts, err := calendar.FeastsForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week feasts: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	view := newWeekView(start, list)
	setFeasts(view.Days, feasts)
	view.IsUpcoming = start.Equal(intentions.NextWeekStart(h.now()))
	view.Schedules = schedules

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}
	view.SelectedScheduleID = setting.SelectedScheduleID

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
	view.Feast = state.feast
	view.ImportError = state.importError
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
