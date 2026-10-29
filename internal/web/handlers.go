package web

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var weekTemplate = template.Must(template.ParseFS(templateFS, "templates/week.html", "templates/panel.html"))

type handler struct {
	now      func() time.Time
	db       *gorm.DB
	validate *validator.Validate
}

func RegisterRoutes(router chi.Router, db *gorm.DB) {
	h := &handler{now: time.Now, db: db, validate: intentions.NewValidator()}

	router.Get("/", h.week)
	router.Post("/intentions", h.create)
	router.Handle("/static/*", http.FileServerFS(staticFS))
}

func (h *handler) week(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	start := intentions.NextWeekStart(h.now())
	if param := query.Get("week"); param != "" {
		day, err := time.ParseInLocation(intentions.DateLayout, param, intentions.TimeZone)
		if err != nil {
			http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
			return
		}

		start = intentions.WeekStart(day)
	}

	var form *formView
	if query.Get("new") == "1" {
		f := newFormView(start, query.Get("day"), query.Get("time"))
		form = &f
	}

	h.renderWeek(w, http.StatusOK, start, form)
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
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

		h.renderWeek(w, http.StatusUnprocessableEntity, start, &form)
		return
	}

	var intention intentions.Intention
	intention.Apply(input)

	err = h.db.Create(&intention).Error
	if err != nil {
		log.Printf("error creating intention: %v", err)
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

func (h *handler) renderWeek(w http.ResponseWriter, status int, start time.Time, form *formView) {
	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	view := newWeekView(start, list)
	view.IsUpcoming = start.Equal(intentions.NextWeekStart(h.now()))

	if form != nil {
		view.Form = form
		markSelected(view.Days, form.Day, form.Time)
	}

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
