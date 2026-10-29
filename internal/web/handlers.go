package web

import (
	"bytes"
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var weekTemplate = template.Must(template.ParseFS(templateFS, "templates/week.html"))

type handler struct {
	now func() time.Time
	db  *gorm.DB
}

func RegisterRoutes(router chi.Router, db *gorm.DB) {
	h := &handler{now: time.Now, db: db}

	router.Get("/", h.week)
	router.Handle("/static/*", http.FileServerFS(staticFS))
}

func (h *handler) week(w http.ResponseWriter, r *http.Request) {
	upcoming := intentions.NextWeekStart(h.now())

	start := upcoming
	if param := r.URL.Query().Get("week"); param != "" {
		day, err := time.ParseInLocation(intentions.DateLayout, param, intentions.TimeZone)
		if err != nil {
			http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
			return
		}

		start = intentions.WeekStart(day)
	}

	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	view := newWeekView(start, list)
	view.IsUpcoming = start.Equal(upcoming)

	render(w, weekTemplate, view)
}

func render(w http.ResponseWriter, tmpl *template.Template, data any) {
	var buf bytes.Buffer

	err := tmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("error rendering template %s: %v", tmpl.Name(), err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}
