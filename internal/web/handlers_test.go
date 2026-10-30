package web

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"go-parafia/internal/calendar"
	"go-parafia/internal/gcatholic"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

var testSchedules = []masses.Schedule{
	{Name: "Porządek zwykły", Masses: [7][]string{time.Monday: {"06:30", "18:00"}, time.Sunday: {"07:00", "10:00"}}},
	{Name: "Wakacje", Masses: [7][]string{time.Sunday: {"09:00"}}},
}

func newTestHandler(t *testing.T, now time.Time) *handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&intentions.Intention{}, &settings.Setting{}, &calendar.Day{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}
	err = masses.Migrate(db)
	if err != nil {
		t.Fatalf("migrating schedules: %v", err)
	}
	for _, s := range testSchedules {
		_, err = masses.Save(db, s)
		if err != nil {
			t.Fatalf("saving schedule %q: %v", s.Name, err)
		}
	}

	return &handler{
		db:       db,
		now:      func() time.Time { return now },
		validate: intentions.NewValidator(),
		fetchCelebrations: func(context.Context, int) ([]gcatholic.Celebration, error) {
			return nil, errors.New("no network in tests")
		},
	}
}

func getWeek(t *testing.T, h *handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.week(rec, req)

	return rec
}

func TestWeekDefaultsToUpcomingWeek(t *testing.T) {
	tests := []struct {
		name        string
		now         time.Time
		wantHeading string
	}{
		{
			name:        "thursday shows the week from next monday",
			now:         time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone),
			wantHeading: "<h1>INTENCJE MSZY ŚWIĘTYCH – 2.11.2026 r. – 8.11.2026 r.</h1>",
		},
		{
			name:        "monday shows the week from next monday",
			now:         time.Date(2026, 11, 2, 7, 0, 0, 0, intentions.TimeZone),
			wantHeading: "<h1>INTENCJE MSZY ŚWIĘTYCH – 9.11.2026 r. – 15.11.2026 r.</h1>",
		},
		{
			name:        "sunday shows the week from the next day",
			now:         time.Date(2026, 11, 8, 20, 0, 0, 0, intentions.TimeZone),
			wantHeading: "<h1>INTENCJE MSZY ŚWIĘTYCH – 9.11.2026 r. – 15.11.2026 r.</h1>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getWeek(t, newTestHandler(t, tt.now), "/")

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			body := rec.Body.String()
			if !strings.Contains(body, tt.wantHeading) {
				t.Errorf("body does not contain %q", tt.wantHeading)
			}
			if !strings.Contains(body, `aria-current="true"`) {
				t.Error(`upcoming week link is not marked with aria-current="true"`)
			}
		})
	}
}

func TestWeekSelectedByQuery(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getWeek(t, h, "/?week=2026-11-11")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/html; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<link rel="stylesheet" href="/static/app.css">`,
		`<script src="/static/app.js" defer></script>`,
		"<h1>INTENCJE MSZY ŚWIĘTYCH – 9.11.2026 r. – 15.11.2026 r.</h1>",
		`href="/?week=2026-11-02"`,
		`href="/?week=2026-11-16"`,
		`id="d-2026-11-09"`,
		`id="d-2026-11-15"`,
		"Poniedziałek 9.11.",
		"Niedziela 15.11.",
		"W tym tygodniu nie ma jeszcze żadnych intencji.",
		`<a class="button button-primary button-large" href="/?new=1&amp;week=2026-11-09">+ Dodaj intencję</a>`,
		`<a class="button toolbar-print" href="/print?week=2026-11-09">Drukuj / PDF</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	if got := strings.Count(body, "brak intencji"); got != 7 {
		t.Errorf(`"brak intencji" appears %d times, want 7`, got)
	}
	if strings.Contains(body, "aria-current") {
		t.Error("a week other than the upcoming one must not be marked with aria-current")
	}
	if strings.Contains(body, `id="panel-title"`) {
		t.Error("panel is open without new=1")
	}
	if strings.Contains(body, `class="pill"`) {
		t.Error("day pills are rendered without an open form")
	}
}

func TestWeekWithNewIntentionPanel(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 18:00", false, "+ Wiesław Więckiewicz")

	rec := getWeek(t, h, "/?week=2026-11-02&new=1&day=2026-11-03&time=18:00")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<body class="has-panel">`,
		`<h2 class="panel-title" id="panel-title">Nowa intencja</h2>`,
		`<form class="panel-form" method="post" action="/intentions">`,
		`<input type="hidden" name="week" value="2026-11-02">`,
		`<legend class="label">Dzień · 2.11 – 8.11.2026</legend>`,
		`value="2026-11-03" checked required>`,
		`<option value="18:00" selected>18:00</option>`,
		`<a class="panel-close" href="/?week=2026-11-02" aria-label="Zamknij" data-close>×</a>`,
		`<section class="day is-selected" id="d-2026-11-03">`,
		`<span class="pill">wybrany w formularzu</span>`,
		`<div class="mass is-selected" data-time="18:00">`,
		// html/template writes & in attributes as &amp;.
		`href="/?day=2026-10-27&amp;new=1&amp;time=18%3A00&amp;week=2026-10-26"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	if got := strings.Count(body, " checked "); got != 1 {
		t.Errorf("%d days are checked, want 1", got)
	}
	if got := strings.Count(body, `<span class="pill">wybrany w formularzu</span>`); got != 7 {
		t.Errorf("day pills = %d, want 7 (CSS shows only the selected one, JS moves the selection)", got)
	}
}

func TestWeekShowsIntentions(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 06:30", false, "++ Jan i Rozalia Leżanka")
	seedIntention(t, h.db, "2026-11-02 06:30", true, "+ Anna Kutyba (greg.)")
	seedIntention(t, h.db, "2026-11-04 18:00", false, "<script>alert(1)</script>")

	rec := getWeek(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<h3 class="mass-time">6:30</h3>`,
		// html/template escapes "+" as "&#43;"; the browser still shows "+".
		`<span>&#43;&#43; Jan i Rozalia Leżanka</span>`,
		`<a class="action" href="/?edit=1&amp;week=2026-11-02" aria-label="Edytuj intencję 1. z godz. 6:30">Edytuj</a>`,
		`<a class="action action-delete" href="/?delete=1&amp;week=2026-11-02" aria-label="Usuń intencję 1. z godz. 6:30">Usuń</a>`,
		`<p class="outside-label">Poza Parafią:</p>`,
		`<span>&#43; Anna Kutyba (greg.)</span>`,
		`<a class="action" href="/?edit=2&amp;week=2026-11-02" aria-label="Edytuj intencję poza parafią 1. z godz. 6:30">Edytuj</a>`,
		`<h3 class="mass-time">18:00</h3>`,
		"&lt;script&gt;alert(1)&lt;/script&gt;",
		`<a class="button button-primary toolbar-add" href="/?new=1&amp;week=2026-11-02">+ Dodaj intencję</a>`,
		`<a class="button-small" href="/?day=2026-11-02&amp;new=1&amp;week=2026-11-02" aria-label="Dodaj intencję – Poniedziałek 2.11.">+ Dodaj</a>`,
		`<a class="add-mass" href="/?day=2026-11-02&amp;new=1&amp;time=06%3A30&amp;week=2026-11-02" aria-label="Dodaj intencję na godz. 6:30">+</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	if strings.Contains(body, "<script>alert(1)") {
		t.Error("intention content is not escaped")
	}
	if got := strings.Count(body, "brak intencji"); got != 5 {
		t.Errorf(`"brak intencji" appears %d times, want 5`, got)
	}
	if strings.Contains(body, "W tym tygodniu nie ma jeszcze żadnych intencji.") {
		t.Error("week with intentions must not show the empty week message")
	}
}

func seedIntention(t *testing.T, db *gorm.DB, massAt string, outside bool, content string) {
	t.Helper()

	intention := newIntention(t, 0, massAt, outside, content)
	err := db.Create(&intention).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}
}

func TestWeekRejectsInvalidDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	for _, target := range []string{
		"/?week=bzdura",
		"/?week=2026-02-30",
		"/?week=02.11.2026",
	} {
		t.Run(target, func(t *testing.T) {
			rec := getWeek(t, h, target)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
		})
	}
}

func TestWeekDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&intentions.Intention{})
	if err != nil {
		t.Fatalf("dropping intentions table: %v", err)
	}

	rec := getWeek(t, h, "/")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestStaticFiles(t *testing.T) {
	router := chi.NewRouter()
	RegisterRoutes(router, nil)

	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Type"), "text/css; charset=utf-8"; got != want {
		t.Errorf("Content-Type = %q, want %q", got, want)
	}
	if !strings.Contains(rec.Body.String(), "--accent: #7A1F2B;") {
		t.Error("app.css does not contain the --accent token")
	}
	if !strings.Contains(rec.Body.String(), "flex: none;\n\twhite-space: nowrap;") {
		t.Error("app.css does not keep buttons on one line (flex: none, white-space: nowrap)")
	}
}

func postIntention(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/intentions", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.create(rec, req)

	return rec
}

func validForm() url.Values {
	return url.Values{
		"week":    {"2026-11-02"},
		"day":     {"2026-11-03"},
		"time":    {"18:00"},
		"content": {"  + Jan Kowalski  "},
		"outside": {"on"},
	}
}

func countIntentions(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	err := db.Model(&intentions.Intention{}).Count(&count).Error
	if err != nil {
		t.Fatalf("counting intentions: %v", err)
	}

	return count
}

func TestCreateIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postIntention(t, h, validForm())

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-02&saved=1#i-1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	var saved []intentions.Intention
	err := h.db.Find(&saved).Error
	if err != nil {
		t.Fatalf("finding intentions: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("intentions in db = %d, want 1", len(saved))
	}

	got := saved[0]
	if want := time.Date(2026, 11, 3, 18, 0, 0, 0, intentions.TimeZone); !got.MassAt.Equal(want) {
		t.Errorf("MassAt = %v, want %v", got.MassAt, want)
	}
	if want := "+ Jan Kowalski"; got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
	if !got.IsOutsideParish {
		t.Error("IsOutsideParish = false, want true")
	}
}

func TestCreateIntentionRedirectsToWeekOfTheDay(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	form := validForm()
	form.Set("day", "2026-11-15")

	rec := postIntention(t, h, form)

	if got, want := rec.Header().Get("Location"), "/?week=2026-11-09&saved=1#i-1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestCreateIntentionInvalid(t *testing.T) {
	tests := []struct {
		name   string
		change func(form url.Values)
		want   []string
	}{
		{
			name: "nothing filled in",
			change: func(form url.Values) {
				form.Del("day")
				form.Del("time")
				form.Set("content", "")
			},
			want: []string{
				`<p class="field-error" id="day-error">Pole jest wymagane.</p>`,
				`<p class="field-error" id="time-error">Pole jest wymagane.</p>`,
				`<p class="field-error" id="content-error">Pole jest wymagane.</p>`,
			},
		},
		{
			name:   "time not on a quarter hour keeps the other values",
			change: func(form url.Values) { form.Set("time", "18:10") },
			want: []string{
				`<p class="field-error" id="time-error">Godzina musi być pełnym kwadransem (:00, :15, :30, :45).</p>`,
				`aria-invalid="true" aria-describedby="time-error"`,
				`<option value="18:10" selected>18:10</option>`,
				`value="2026-11-03" checked`,
				`&#43; Jan Kowalski`,
				`name="outside" checked`,
			},
		},
		{
			name:   "whitespace only content",
			change: func(form url.Values) { form.Set("content", "   ") },
			want: []string{
				`<p class="field-error" id="content-error">Pole jest wymagane.</p>`,
				`aria-invalid="true" aria-describedby="content-error"`,
			},
		},
		{
			name:   "content too long",
			change: func(form url.Values) { form.Set("content", strings.Repeat("a", 501)) },
			want: []string{
				`<p class="field-error" id="content-error">Maksymalnie 500 znaków.</p>`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			form := validForm()
			tt.change(form)

			rec := postIntention(t, h, form)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}

			body := rec.Body.String()
			want := append([]string{
				`<div class="form-alert" role="alert">`,
				"<h1>INTENCJE MSZY ŚWIĘTYCH – 2.11.2026 r. – 8.11.2026 r.</h1>",
			}, tt.want...)
			for _, w := range want {
				if !strings.Contains(body, w) {
					t.Errorf("body does not contain %q", w)
				}
			}

			if count := countIntentions(t, h.db); count != 0 {
				t.Errorf("intentions in db = %d, want 0", count)
			}
		})
	}
}

func TestCreateIntentionDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&intentions.Intention{})
	if err != nil {
		t.Fatalf("dropping intentions table: %v", err)
	}

	rec := postIntention(t, h, validForm())

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateIntentionRoute(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := postToRouter(t, db, "/intentions", validForm())

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func postToRouter(t *testing.T, db *gorm.DB, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	RegisterRoutes(router, db)

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func TestWeekHighlightsSavedIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 06:30", false, "+ Jan Kowalski")

	tests := []struct {
		target    string
		wantSaved bool
	}{
		{target: "/?week=2026-11-02&saved=1", wantSaved: true},
		{target: "/?week=2026-11-02&saved=2", wantSaved: false},
		{target: "/?week=2026-11-02&saved=abc", wantSaved: false},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := getWeek(t, h, tt.target)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			gotSaved := strings.Contains(rec.Body.String(), `<li class="intention is-saved" id="i-1">`)
			if gotSaved != tt.wantSaved {
				t.Errorf("intention highlighted = %v, want %v", gotSaved, tt.wantSaved)
			}
		})
	}
}

func TestWeekWithEditPanel(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 18:00", true, "+ Anna Kutyba (greg.)")

	rec := getWeek(t, h, "/?week=2026-11-02&edit=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<h2 class="panel-title" id="panel-title">Edycja intencji</h2>`,
		`<form class="panel-form" method="post" action="/intentions/1">`,
		`value="2026-11-03" checked`,
		`<option value="18:00" selected>18:00</option>`,
		`>&#43; Anna Kutyba (greg.)</textarea>`,
		`name="outside" checked`,
		`<a class="delete-link" href="/?delete=1&amp;week=2026-11-02">Usuń intencję</a>`,
		`<li class="intention is-editing" id="i-1">`,
		`href="/?day=2026-10-27&amp;edit=1&amp;time=18%3A00&amp;week=2026-10-26"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestWeekEditPanelInAnotherWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 18:00", false, "+ Jan Kowalski")

	rec := getWeek(t, h, "/?week=2026-11-09&edit=1&day=2026-11-10&time=18:00")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<h2 class="panel-title" id="panel-title">Edycja intencji</h2>`,
		`value="2026-11-10" checked`,
		`>&#43; Jan Kowalski</textarea>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestWeekEditPanelForMissingIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	tests := []struct {
		target string
		want   int
	}{
		{target: "/?week=2026-11-02&edit=999", want: http.StatusNotFound},
		{target: "/?week=2026-11-02&edit=abc", want: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := getWeek(t, h, tt.target)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
		})
	}
}

func TestUpdateIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 18:00", true, "+ Jan Kowalski")

	form := url.Values{
		"week":    {"2026-11-02"},
		"day":     {"2026-11-10"},
		"time":    {"06:30"},
		"content": {" + Jan Kowalski (greg.) "},
	}
	rec := postToRouter(t, h.db, "/intentions/1", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-09&saved=1#i-1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	var saved []intentions.Intention
	err := h.db.Find(&saved).Error
	if err != nil {
		t.Fatalf("finding intentions: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("intentions in db = %d, want 1 (update must not create a new one)", len(saved))
	}

	got := saved[0]
	if want := time.Date(2026, 11, 10, 6, 30, 0, 0, intentions.TimeZone); !got.MassAt.Equal(want) {
		t.Errorf("MassAt = %v, want %v", got.MassAt, want)
	}
	if want := "+ Jan Kowalski (greg.)"; got.Content != want {
		t.Errorf("Content = %q, want %q", got.Content, want)
	}
	if got.IsOutsideParish {
		t.Error("IsOutsideParish = true, want false (unchecked box is not sent)")
	}
}

func TestUpdateIntentionInvalid(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 18:00", false, "+ Jan Kowalski")

	form := validForm()
	form.Set("content", "")
	rec := postToRouter(t, h.db, "/intentions/1", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<h2 class="panel-title" id="panel-title">Edycja intencji</h2>`,
		`<form class="panel-form" method="post" action="/intentions/1">`,
		`<p class="field-error" id="content-error">Pole jest wymagane.</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	var intention intentions.Intention
	err := h.db.First(&intention, 1).Error
	if err != nil {
		t.Fatalf("finding intention: %v", err)
	}
	if want := "+ Jan Kowalski"; intention.Content != want {
		t.Errorf("Content = %q, want %q (invalid form must not change the intention)", intention.Content, want)
	}
}

func TestUpdateIntentionNotFound(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	tests := []struct {
		path string
		want int
	}{
		{path: "/intentions/999", want: http.StatusNotFound},
		{path: "/intentions/abc", want: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := postToRouter(t, db, tt.path, validForm())

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			if count := countIntentions(t, db); count != 0 {
				t.Errorf("intentions in db = %d, want 0", count)
			}
		})
	}
}

func TestWeekWithDeleteDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-03 06:30", true, "+ Anna Kutyba (greg.)")

	rec := getWeek(t, h, "/?week=2026-11-02&delete=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<div class="dialog" role="dialog" aria-modal="true" aria-labelledby="dialog-title" aria-describedby="dialog-when">`,
		`<h2 class="dialog-title" id="dialog-title">Usunąć intencję?</h2>`,
		`<p class="dialog-when" id="dialog-when">Wtorek 3.11., godz. 6:30 · poza parafią</p>`,
		`<p class="dialog-content">&#43; Anna Kutyba (greg.)</p>`,
		`<form class="dialog-actions" method="post" action="/intentions/1/delete">`,
		`<button class="button button-danger" type="submit" autofocus>Usuń</button>`,
		`<a class="button" href="/?week=2026-11-02" data-close>Anuluj</a>`,
		`<main inert>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	if strings.Contains(body, `id="panel-title"`) {
		t.Error("side panel is open together with the delete dialog")
	}
}

func TestWeekDeleteDialogForMissingIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getWeek(t, h, "/?week=2026-11-02&delete=999")

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestDeleteIntention(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-10 18:00", false, "+ Jan Kowalski")
	seedIntention(t, h.db, "2026-11-10 18:00", false, "+ Anna Nowak")

	rec := postToRouter(t, h.db, "/intentions/1/delete", url.Values{})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-09"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	var left []intentions.Intention
	err := h.db.Find(&left).Error
	if err != nil {
		t.Fatalf("finding intentions: %v", err)
	}
	if len(left) != 1 || left[0].ID != 2 {
		t.Errorf("intentions left = %+v, want only intention 2", left)
	}
}

func TestDeleteIntentionNotFound(t *testing.T) {
	h := newTestHandler(t, time.Now())
	seedIntention(t, h.db, "2026-11-10 18:00", false, "+ Jan Kowalski")

	tests := []struct {
		path string
		want int
	}{
		{path: "/intentions/999/delete", want: http.StatusNotFound},
		{path: "/intentions/abc/delete", want: http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			rec := postToRouter(t, h.db, tt.path, url.Values{})

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}
			if count := countIntentions(t, h.db); count != 1 {
				t.Errorf("intentions in db = %d, want 1", count)
			}
		})
	}
}

func getPDF(t *testing.T, h *handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.pdf(rec, req)

	return rec
}

func TestWeekPDF(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantFilename string
	}{
		{name: "week from the address", target: "/pdf?week=2026-11-11", wantFilename: "intencje-2026-11-09.pdf"},
		{name: "upcoming week by default", target: "/pdf", wantFilename: "intencje-2026-11-02.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			seedIntention(t, h.db, "2026-11-10 18:00", true, "+ Anna Kutyba (greg.)")

			rec := getPDF(t, h, tt.target)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got, want := rec.Header().Get("Content-Type"), "application/pdf"; got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="`+tt.wantFilename+`"`; got != want {
				t.Errorf("Content-Disposition = %q, want %q", got, want)
			}
			if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
				t.Errorf("body does not start with %%PDF-")
			}
		})
	}
}

func TestWeekPDFRejectsInvalidDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getPDF(t, h, "/pdf?week=bzdura")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWeekPDFDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&intentions.Intention{})
	if err != nil {
		t.Fatalf("dropping intentions table: %v", err)
	}

	rec := getPDF(t, h, "/pdf?week=2026-11-02")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Error("a PDF was sent despite the error")
	}
}

func TestPrintPreview(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	req := httptest.NewRequest(http.MethodGet, "/print?week=2026-11-04", nil)
	rec := httptest.NewRecorder()
	h.printPreview(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<a class="button" href="/?week=2026-11-02" data-close>← Wróć do tygodnia</a>`,
		`<iframe class="print-preview" src="/pdf?week=2026-11-02" title="Podgląd PDF: intencje 2.11.2026 r. – 8.11.2026 r."></iframe>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/print?week=bzdura", nil)
	rec = httptest.NewRecorder()
	h.printPreview(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("invalid week: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func postSlots(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/slots", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.createSlots(rec, req)

	return rec
}

func placeholderTimes(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var slots []intentions.Intention
	err := db.Where("is_placeholder = ?", true).Order("mass_at").Find(&slots).Error
	if err != nil {
		t.Fatalf("loading slots: %v", err)
	}

	times := make([]string, 0, len(slots))
	for _, slot := range slots {
		times = append(times, slot.MassAt.In(intentions.TimeZone).Format(intentions.MassAtLayout))
	}

	return times
}

func TestCreateSlots(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")

	rec := postSlots(t, h, url.Values{"week": {"2026-11-04"}, "schedule": {"1"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-02"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	want := []string{"2026-11-02 06:30", "2026-11-08 07:00", "2026-11-08 10:00"}
	if got := placeholderTimes(t, h.db); !slices.Equal(got, want) {
		t.Errorf("slots = %q, want %q", got, want)
	}
}

func TestCreateSlotsUsesChosenSchedule(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	postSlots(t, h, url.Values{"week": {"2026-11-02"}, "schedule": {"2"}})

	want := []string{"2026-11-08 09:00"}
	if got := placeholderTimes(t, h.db); !slices.Equal(got, want) {
		t.Errorf("slots = %q, want %q", got, want)
	}
}

func TestCreateSlotsTwiceDoesNotDuplicate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	form := url.Values{"week": {"2026-11-02"}, "schedule": {"1"}}

	postSlots(t, h, form)
	postSlots(t, h, form)

	if got := countIntentions(t, h.db); got != 4 {
		t.Errorf("intentions = %d, want 4", got)
	}
}

func TestCreateSlotsConcurrently(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	form := url.Values{"week": {"2026-11-02"}, "schedule": {"1"}}

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			rec := postSlots(t, h, form)
			if rec.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
		})
	}
	wg.Wait()

	if got := countIntentions(t, h.db); got != 4 {
		t.Errorf("intentions = %d, want 4", got)
	}
}

func TestCreateSlotsInvalid(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
	}{
		{name: "invalid week", form: url.Values{"week": {"jutro"}, "schedule": {"1"}}},
		{name: "unknown schedule", form: url.Values{"week": {"2026-11-02"}, "schedule": {"99"}}},
		{name: "schedule is not a number", form: url.Values{"week": {"2026-11-02"}, "schedule": {"Porządek zwykły"}}},
		{name: "missing schedule", form: url.Values{"week": {"2026-11-02"}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

			rec := postSlots(t, h, tt.form)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			if got := countIntentions(t, h.db); got != 0 {
				t.Errorf("intentions = %d, want 0", got)
			}
		})
	}
}

func TestCreateSlotsDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&intentions.Intention{})
	if err != nil {
		t.Fatalf("dropping intentions table: %v", err)
	}

	rec := postSlots(t, h, url.Values{"week": {"2026-11-02"}, "schedule": {"1"}})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateSlotsRoute(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := postToRouter(t, db, "/slots", url.Values{"week": {"2026-11-02"}, "schedule": {"1"}})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestWeekShowsScheduleForm(t *testing.T) {
	rec := getWeek(t, newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)), "/")

	body := rec.Body.String()
	for _, want := range []string{
		`<form class="schedule-load" method="post" action="/slots">`,
		`<input type="hidden" name="week" value="2026-11-02">`,
		`<option value="1">Porządek zwykły</option>`,
		`<option value="2">Wakacje</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestWeekWithoutSchedulesHidesScheduleForm(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := masses.Delete(h.db, 1)
	if err == nil {
		err = masses.Delete(h.db, 2)
	}
	if err != nil {
		t.Fatalf("deleting schedules: %v", err)
	}

	body := getWeek(t, h, "/").Body.String()

	if strings.Contains(body, "schedule-load") {
		t.Error("body contains the schedule form, want none without schedules")
	}
}

func seedPlaceholder(t *testing.T, db *gorm.DB, massAt string) int {
	t.Helper()

	slot := newIntention(t, 0, massAt, false, "")
	slot.IsPlaceholder = true
	err := db.Create(&slot).Error
	if err != nil {
		t.Fatalf("creating slot: %v", err)
	}

	return slot.ID
}

func TestWeekShowsPlaceholder(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	id := seedPlaceholder(t, h.db, "2026-11-02 06:30")

	body := getWeek(t, h, "/").Body.String()

	for _, want := range []string{
		`<span class="number"></span>`,
		`<span class="placeholder">do uzupełnienia</span>`,
		fmt.Sprintf(`<a class="action" href="/?edit=%d&amp;week=2026-11-02" aria-label="Uzupełnij intencję z godz. 6:30">Uzupełnij</a>`, id),
		fmt.Sprintf(`<a class="action action-delete" href="/?delete=%d&amp;week=2026-11-02" aria-label="Usuń nieuzupełnioną intencję z godz. 6:30">Usuń</a>`, id),
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if strings.Contains(body, "Edytuj intencję") {
		t.Error("placeholder has the regular edit link, want Uzupełnij")
	}
}

func TestWeekNumbersIntentionsWithoutPlaceholders(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPlaceholder(t, h.db, "2026-11-02 18:00")
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")

	body := getWeek(t, h, "/").Body.String()

	if !strings.Contains(body, `aria-label="Edytuj intencję 1. z godz. 18:00">Edytuj</a>`) {
		t.Error("the only real intention is not number 1")
	}
	if strings.Contains(body, "intencję 2.") {
		t.Error("the placeholder is counted in the numbering")
	}
}

func TestPlaceholderEditPanelHasEmptyContent(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	id := seedPlaceholder(t, h.db, "2026-11-02 06:30")

	body := getWeek(t, h, fmt.Sprintf("/?week=2026-11-02&edit=%d", id)).Body.String()

	if !strings.Contains(body, `autofocus></textarea>`) {
		t.Error("edit panel for a placeholder does not have an empty content field")
	}
}

func TestPlaceholderDeleteDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	id := seedPlaceholder(t, h.db, "2026-11-02 06:30")

	body := getWeek(t, h, fmt.Sprintf("/?week=2026-11-02&delete=%d", id)).Body.String()

	if !strings.Contains(body, `<p class="dialog-content placeholder">do uzupełnienia</p>`) {
		t.Error("delete dialog for a placeholder does not say it is empty")
	}
}

func TestCreateSlotsRemembersSchedule(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	postSlots(t, h, url.Values{"week": {"2026-11-02"}, "schedule": {"2"}})

	setting, err := settings.Load(h.db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if setting.SelectedScheduleID != 2 {
		t.Errorf("SelectedScheduleID = %d, want 2 (Wakacje)", setting.SelectedScheduleID)
	}
}

func TestCreateSlotsRemembersScheduleWhenNothingToCreate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-08 09:00", false, "+ Jan Kowalski")

	postSlots(t, h, url.Values{"week": {"2026-11-02"}, "schedule": {"2"}})

	if got := countIntentions(t, h.db); got != 1 {
		t.Fatalf("intentions = %d, want 1 (the only mass of Wakacje is taken)", got)
	}
	setting, err := settings.Load(h.db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if setting.SelectedScheduleID != 2 {
		t.Errorf("SelectedScheduleID = %d, want 2 (Wakacje)", setting.SelectedScheduleID)
	}
}

func TestCreateSlotsWithoutSettingsTable(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&settings.Setting{})
	if err != nil {
		t.Fatalf("dropping settings table: %v", err)
	}

	rec := postSlots(t, h, url.Values{"week": {"2026-11-02"}, "schedule": {"1"}})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (slots are saved, settings are only a convenience)", rec.Code, http.StatusSeeOther)
	}
	if got := countIntentions(t, h.db); got != 4 {
		t.Errorf("intentions = %d, want 4", got)
	}
}

func TestWeekPreselectsRememberedSchedule(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := settings.SaveSchedule(h.db, 2)
	if err != nil {
		t.Fatalf("saving schedule: %v", err)
	}

	body := getWeek(t, h, "/").Body.String()

	for _, want := range []string{
		`<option value="1">Porządek zwykły</option>`,
		`<option value="2" selected>Wakacje</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestWeekWithUnknownRememberedSchedule(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := settings.SaveSchedule(h.db, 99)
	if err != nil {
		t.Fatalf("saving schedule: %v", err)
	}

	body := getWeek(t, h, "/").Body.String()

	if strings.Contains(body, " selected>") {
		t.Error("an option is selected, want none for a deleted schedule")
	}
}

func TestWeekWithoutSettingsTable(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&settings.Setting{})
	if err != nil {
		t.Fatalf("dropping settings table: %v", err)
	}

	rec := getWeek(t, h, "/")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (settings are only a convenience)", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `<option value="2">Wakacje</option>`) {
		t.Error("schedule form is missing")
	}
}

func TestWeekShowsClearLink(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	link := `<a class="delete-link" href="/?clear=1&amp;week=2026-11-02">Wyczyść tydzień</a>`

	if body := getWeek(t, h, "/").Body.String(); strings.Contains(body, link) {
		t.Error("empty week has the clear link")
	}

	seedPlaceholder(t, h.db, "2026-11-02 06:30")

	if body := getWeek(t, h, "/").Body.String(); !strings.Contains(body, link) {
		t.Errorf("body does not contain %q", link)
	}
}

func TestWeekWithClearDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPlaceholder(t, h.db, "2026-11-02 06:30")
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")
	seedIntention(t, h.db, "2026-11-08 10:00", true, "+ Anna Nowak")
	seedIntention(t, h.db, "2026-11-09 10:00", false, "next week")

	body := getWeek(t, h, "/?week=2026-11-02&clear=1").Body.String()

	for _, want := range []string{
		`<main inert>`,
		`<h2 class="dialog-title" id="dialog-title">Wyczyścić tydzień?</h2>`,
		`<p class="dialog-when">2.11.2026 r. – 8.11.2026 r.</p>`,
		`Do usunięcia: 3 intencje (w tym 1 do uzupełnienia). Tej operacji nie można cofnąć.`,
		`<form class="dialog-actions" method="post" action="/clear">`,
		`<input type="hidden" name="week" value="2026-11-02">`,
		`<a class="button" href="/?week=2026-11-02" data-close autofocus>Anuluj</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if strings.Contains(body, `type="submit" autofocus`) {
		t.Error("the destructive button has autofocus, want it on Anuluj")
	}
}

func TestWeekClearDialogForEmptyWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	body := getWeek(t, h, "/?week=2026-11-02&clear=1").Body.String()

	if strings.Contains(body, "Wyczyścić tydzień?") || strings.Contains(body, "inert") {
		t.Error("empty week shows the clear dialog")
	}
}

func postClear(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/clear", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.clearWeek(rec, req)

	return rec
}

func TestClearWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-01 18:00", false, "previous week")
	seedPlaceholder(t, h.db, "2026-11-02 06:30")
	seedIntention(t, h.db, "2026-11-08 18:00", true, "+ Anna Nowak")
	seedIntention(t, h.db, "2026-11-09 06:30", false, "next week")

	rec := postClear(t, h, url.Values{"week": {"2026-11-05"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-02"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	var left []intentions.Intention
	err := h.db.Order("mass_at").Find(&left).Error
	if err != nil {
		t.Fatalf("loading intentions: %v", err)
	}
	var got []string
	for _, intention := range left {
		got = append(got, intention.Content)
	}
	if want := []string{"previous week", "next week"}; !slices.Equal(got, want) {
		t.Errorf("left = %q, want %q", got, want)
	}
}

func TestClearWeekInvalid(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")

	rec := postClear(t, h, url.Values{"week": {"jutro"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := countIntentions(t, h.db); got != 1 {
		t.Errorf("intentions = %d, want 1", got)
	}
}

func TestClearWeekDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&intentions.Intention{})
	if err != nil {
		t.Fatalf("dropping intentions table: %v", err)
	}

	rec := postClear(t, h, url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestClearWeekRoute(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := postToRouter(t, db, "/clear", url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestWeekShowsFeast(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := calendar.SaveFeast(h.db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}

	body := getWeek(t, h, "/").Body.String()

	want := `<h2 class="day-name">Środa 4.11.</h2>
					<span class="feast">wspomnienie św. Karola Boromeusza</span>`
	if !strings.Contains(body, want) {
		t.Errorf("body does not contain %q", want)
	}
	if got := strings.Count(body, `class="feast"`); got != 1 {
		t.Errorf("feast appears %d times, want 1", got)
	}
}

func TestWeekFeastsDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&calendar.Day{})
	if err != nil {
		t.Fatalf("dropping days table: %v", err)
	}

	if rec := getWeek(t, h, "/"); rec.Code != http.StatusInternalServerError {
		t.Errorf("week: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if rec := getPDF(t, h, "/pdf"); rec.Code != http.StatusInternalServerError {
		t.Errorf("pdf: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestWeekShowsFeastLinks(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := calendar.SaveFeast(h.db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}

	body := getWeek(t, h, "/").Body.String()

	for _, want := range []string{
		`<a class="feast-link" href="/?feast=2026-11-03&amp;week=2026-11-02" aria-label="Dodaj wspomnienie lub święto – Wtorek 3.11.">+ święto</a>`,
		`<a class="feast-link" href="/?feast=2026-11-04&amp;week=2026-11-02" aria-label="Zmień wspomnienie lub święto – Środa 4.11.">Zmień</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if got := strings.Count(body, "+ święto</a>"); got != 6 {
		t.Errorf(`"+ święto" links = %d, want 6`, got)
	}
}

func TestWeekWithFeastDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := calendar.SaveFeast(h.db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}

	body := getWeek(t, h, "/?week=2026-11-02&feast=2026-11-04").Body.String()

	for _, want := range []string{
		`<main inert>`,
		`<form method="post" action="/days/2026-11-04">`,
		`<h2 class="dialog-title" id="dialog-title">Wspomnienie / święto</h2>`,
		`<p class="dialog-when">Środa 4.11.</p>`,
		`value="wspomnienie św. Karola Boromeusza" maxlength="300" autofocus`,
		`<a class="button" href="/?week=2026-11-02" data-close>Anuluj</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestWeekFeastDialogForDayWithoutFeast(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	body := getWeek(t, h, "/?week=2026-11-02&feast=2026-11-03").Body.String()

	if !strings.Contains(body, `name="feast" value="" maxlength="300"`) {
		t.Error("dialog for a day without a feast does not have an empty field")
	}
}

func TestWeekFeastDialogInvalidDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getWeek(t, h, "/?week=2026-11-02&feast=jutro")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func loadFeast(t *testing.T, db *gorm.DB, date string) string {
	t.Helper()

	feast, err := calendar.Feast(db, date)
	if err != nil {
		t.Fatalf("loading feast: %v", err)
	}

	return feast
}

func TestSaveFeast(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postToRouter(t, h.db, "/days/2026-11-04", url.Values{"feast": {"  wspomnienie św. Karola Boromeusza  "}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-02#d-2026-11-04"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if got, want := loadFeast(t, h.db, "2026-11-04"), "wspomnienie św. Karola Boromeusza"; got != want {
		t.Errorf("feast = %q, want %q", got, want)
	}
}

func TestSaveEmptyFeastRemovesIt(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := calendar.SaveFeast(h.db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}

	rec := postToRouter(t, h.db, "/days/2026-11-04", url.Values{"feast": {"   "}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := loadFeast(t, h.db, "2026-11-04"); got != "" {
		t.Errorf("feast = %q, want it removed", got)
	}
}

func TestSaveFeastTooLong(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	long := strings.Repeat("ś", maxFeastLength+1)

	rec := postToRouter(t, h.db, "/days/2026-11-04", url.Values{"feast": {long}})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<p class="field-error" id="feast-error">Maksymalnie 300 znaków.</p>`,
		`value="` + long + `"`,
		`aria-invalid="true"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if got := loadFeast(t, h.db, "2026-11-04"); got != "" {
		t.Errorf("feast = %q, want nothing saved", got)
	}
}

func TestSaveFeastAtMaxLength(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postToRouter(t, h.db, "/days/2026-11-04", url.Values{"feast": {strings.Repeat("ś", maxFeastLength)}})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d (limit counts letters, not bytes)", rec.Code, http.StatusSeeOther)
	}
}

func TestSaveFeastInvalidDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postToRouter(t, h.db, "/days/jutro", url.Values{"feast": {"święto"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestSaveFeastDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&calendar.Day{})
	if err != nil {
		t.Fatalf("dropping days table: %v", err)
	}

	rec := postToRouter(t, h.db, "/days/2026-11-04", url.Values{"feast": {"święto"}})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func fakeCelebrations(byYear map[int][]gcatholic.Celebration, requested *[]int) func(context.Context, int) ([]gcatholic.Celebration, error) {
	return func(_ context.Context, year int) ([]gcatholic.Celebration, error) {
		if requested != nil {
			*requested = append(*requested, year)
		}
		return byYear[year], nil
	}
}

func postImport(t *testing.T, h *handler, week string) *httptest.ResponseRecorder {
	t.Helper()

	form := url.Values{"week": {week}}
	req := httptest.NewRequest(http.MethodPost, "/feasts/import", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.importFeasts(rec, req)

	return rec
}

func TestImportFeasts(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := calendar.SaveFeast(h.db, "2026-11-04", "odpust parafialny")
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}

	var requested []int
	h.fetchCelebrations = fakeCelebrations(map[int][]gcatholic.Celebration{2026: {
		{Date: "2026-11-01", Rank: "U", Name: "Wszystkich Świętych"},
		{Date: "2026-11-03", Name: "Wtorek XXXI tygodnia zwykłego"},
		{Date: "2026-11-03", Rank: "w", Name: "Św. Marcina de Porres, zakonnika"},
		{Date: "2026-11-04", Rank: "W", Name: "Św. Karola Boromeusza, biskupa"},
		{Date: "2026-11-08", Name: "XXXII Niedziela Zwykła"},
		{Date: "2026-11-09", Rank: "Ś", Name: "Rocznica poświęcenia Bazyliki Laterańskiej"},
	}}, &requested)

	rec := postImport(t, h, "2026-11-05")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-02"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if !slices.Equal(requested, []int{2026}) {
		t.Errorf("requested years = %v, want [2026]", requested)
	}

	got, err := calendar.FeastsForWeek(h.db, time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err != nil {
		t.Fatalf("loading feasts: %v", err)
	}
	want := map[string]string{
		"2026-11-03": "wspomnienie dowolne św. Marcina de Porres, zakonnika",
		"2026-11-04": "odpust parafialny",
		"2026-11-08": "XXXII Niedziela Zwykła",
	}
	if !maps.Equal(got, want) {
		t.Errorf("feasts = %q, want %q", got, want)
	}
	if feast := loadFeast(t, h.db, "2026-11-01"); feast != "" {
		t.Errorf("feast outside the week saved: %q", feast)
	}
}

func TestImportFeastsAcrossNewYear(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	var requested []int
	h.fetchCelebrations = fakeCelebrations(map[int][]gcatholic.Celebration{
		2026: {{Date: "2026-12-28", Rank: "Ś", Name: "Świętych Młodzianków, męczenników"}},
		2027: {{Date: "2027-01-01", Rank: "U", Name: "Świętej Bożej Rodzicielki Maryi"}},
	}, &requested)

	postImport(t, h, "2026-12-30")

	if !slices.Equal(requested, []int{2026, 2027}) {
		t.Errorf("requested years = %v, want [2026 2027]", requested)
	}
	if got := loadFeast(t, h.db, "2026-12-28"); got != "święto Świętych Młodzianków, męczenników" {
		t.Errorf("2026-12-28 = %q", got)
	}
	if got := loadFeast(t, h.db, "2027-01-01"); got != "uroczystość Świętej Bożej Rodzicielki Maryi" {
		t.Errorf("2027-01-01 = %q", got)
	}
}

func TestImportFeastsFetchError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	h.fetchCelebrations = func(context.Context, int) ([]gcatholic.Celebration, error) {
		return nil, errors.New("no network")
	}

	rec := postImport(t, h, "2026-11-02")

	if rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<div class="form-alert week-alert" role="alert">`,
		"<p>Nie udało się pobrać kalendarza z GCatholic. Sprawdź połączenie z internetem i spróbuj ponownie.</p>",
		`<h1>INTENCJE MSZY ŚWIĘTYCH – 2.11.2026 r. – 8.11.2026 r.</h1>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestImportFeastsInvalidWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	called := false
	h.fetchCelebrations = func(context.Context, int) ([]gcatholic.Celebration, error) {
		called = true
		return nil, nil
	}

	rec := postImport(t, h, "jutro")

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if called {
		t.Error("GCatholic was asked despite the invalid week")
	}
}

func TestImportFeastsDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	h.fetchCelebrations = fakeCelebrations(map[int][]gcatholic.Celebration{2026: {
		{Date: "2026-11-04", Rank: "W", Name: "Św. Karola Boromeusza, biskupa"},
	}}, nil)

	err := h.db.Migrator().DropTable(&calendar.Day{})
	if err != nil {
		t.Fatalf("dropping days table: %v", err)
	}

	rec := postImport(t, h, "2026-11-02")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestImportFeastsRoute(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := postToRouter(t, db, "/feasts/import", url.Values{"week": {"jutro"}})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (route registered, no request to GCatholic)", rec.Code, http.StatusBadRequest)
	}
}

func TestWeekShowsImportButton(t *testing.T) {
	body := getWeek(t, newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)), "/").Body.String()

	want := `<form method="post" action="/feasts/import">
				<input type="hidden" name="week" value="2026-11-02">
				<button class="button" type="submit">Pobierz święta z GCatholic</button>`
	if !strings.Contains(body, want) {
		t.Errorf("body does not contain %q", want)
	}
	if strings.Contains(body, "week-alert") {
		t.Error("week view shows the import error without an import")
	}
}

func getSettings(t *testing.T, h *handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.settingsPage(rec, req)

	return rec
}

func postSettings(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.saveSettings(rec, req)

	return rec
}

func TestWeekShowsSettingsLink(t *testing.T) {
	body := getWeek(t, newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)), "/").Body.String()

	want := `<a class="button button-icon" href="/settings?week=2026-11-02" aria-label="Ustawienia" title="Ustawienia">`
	if !strings.Contains(body, want) {
		t.Errorf("body does not contain %q", want)
	}
}

func TestSettingsPageDefaults(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getSettings(t, h, "/settings?week=2026-11-04")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="button" href="/?week=2026-11-02" data-close>← Wróć do tygodnia</a>`,
		`<input type="hidden" name="week" value="2026-11-02">`,
		`name="parish_name" value="Parafia pw. Trójcy Przenajświętszej i św. Leopolda w Rzezawie" maxlength="200" required`,
		`name="address" value="" maxlength="200"`,
		`<input type="checkbox" name="import" value="solemnities" checked> <span>Uroczystości</span>`,
		`<input type="checkbox" name="import" value="other-days" checked> <span>Niedziele i dni szczególne<span class="checkbox-hint">`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if got := strings.Count(body, `name="import"`); got != 5 {
		t.Errorf("checkboxes = %d, want 5", got)
	}
	if strings.Contains(body, "Zapisano.") {
		t.Error("page says Zapisano. before saving")
	}
}

func TestSettingsPageWithoutWeek(t *testing.T) {
	body := getSettings(t, newTestHandler(t, time.Now()), "/settings").Body.String()

	if !strings.Contains(body, `<a class="button" href="/" data-close>← Wróć do tygodnia</a>`) {
		t.Error("back link without a week does not go to /")
	}
}

func TestSaveSettings(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := settings.SaveSchedule(h.db, 2)
	if err != nil {
		t.Fatalf("saving schedule: %v", err)
	}

	rec := postSettings(t, h, url.Values{
		"week":        {"2026-11-02"},
		"parish_name": {"  Parafia św. Jana  "},
		"address":     {"ul. Kościelna 1, 32-765 Rzezawa"},
		"import":      {"solemnities", "feasts", "memorials"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/settings?saved=1&week=2026-11-02"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	got, err := settings.Load(h.db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	want := settings.Setting{
		ID:                 1,
		ParishName:         "Parafia św. Jana",
		Address:            "ul. Kościelna 1, 32-765 Rzezawa",
		SelectedScheduleID: 2,
		FeastImport:        settings.FeastImport{SkipOptionalMemorials: true, SkipOtherDays: true},
	}
	if got != want {
		t.Errorf("settings = %+v, want %+v", got, want)
	}

	body := getSettings(t, h, "/settings?saved=1&week=2026-11-02").Body.String()
	for _, want := range []string{
		`<p class="notice" role="status">Zapisano.</p>`,
		`<input type="checkbox" name="import" value="memorials" checked>`,
		`<input type="checkbox" name="import" value="optional-memorials"> <span>Wspomnienia dowolne</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page after saving does not contain %q", want)
		}
	}
}

func TestSaveSettingsInvalid(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
		want string
	}{
		{
			name: "empty parish name",
			form: url.Values{"parish_name": {"   "}, "address": {"ul. Kościelna 1"}, "import": {"feasts"}},
			want: `<p class="field-error" id="parish_name-error">Pole jest wymagane.</p>`,
		},
		{
			name: "address too long",
			form: url.Values{"parish_name": {"Parafia"}, "address": {strings.Repeat("ś", maxSettingLength+1)}},
			want: `<p class="field-error" id="address-error">Maksymalnie 200 znaków.</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Now())

			rec := postSettings(t, h, tt.form)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) {
				t.Errorf("body does not contain %q", tt.want)
			}
			if !strings.Contains(body, `aria-invalid="true"`) {
				t.Error("invalid field is not marked aria-invalid")
			}

			got, err := settings.Load(h.db)
			if err != nil {
				t.Fatalf("loading settings: %v", err)
			}
			if got.ParishName != settings.DefaultParishName || got.Address != "" {
				t.Errorf("settings changed despite errors: %+v", got)
			}
		})
	}
}

func TestSaveSettingsKeepsTypedValuesOnError(t *testing.T) {
	rec := postSettings(t, newTestHandler(t, time.Now()), url.Values{
		"parish_name": {""},
		"address":     {"ul. Kościelna 1"},
		"import":      {"feasts"},
	})

	body := rec.Body.String()
	for _, want := range []string{
		`name="address" value="ul. Kościelna 1"`,
		`<input type="checkbox" name="import" value="feasts" checked>`,
		`<input type="checkbox" name="import" value="solemnities"> <span>Uroczystości</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestSettingsRoutes(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := postToRouter(t, db, "/settings", url.Values{"parish_name": {"Parafia św. Jana"}})

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
}

func TestImportFeastsSkipsUncheckedKinds(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := settings.SaveParish(h.db, settings.Setting{
		ParishName:  settings.DefaultParishName,
		FeastImport: settings.FeastImport{SkipOptionalMemorials: true, SkipOtherDays: true},
	})
	if err != nil {
		t.Fatalf("saving settings: %v", err)
	}
	h.fetchCelebrations = fakeCelebrations(map[int][]gcatholic.Celebration{2026: {
		{Date: "2026-11-03", Name: "Wtorek XXXI tygodnia zwykłego"},
		{Date: "2026-11-03", Rank: "w", Name: "Św. Marcina de Porres, zakonnika"},
		{Date: "2026-11-04", Rank: "W", Name: "Św. Karola Boromeusza, biskupa"},
		{Date: "2026-11-07", Name: "Sobota XXXI tygodnia zwykłego"},
		{Date: "2026-11-07", Rank: "w", Name: "Najświętszej Maryi Panny"},
	}}, nil)

	postImport(t, h, "2026-11-02")

	got, err := calendar.FeastsForWeek(h.db, time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err != nil {
		t.Fatalf("loading feasts: %v", err)
	}
	want := map[string]string{"2026-11-04": "wspomnienie św. Karola Boromeusza, biskupa"}
	if !maps.Equal(got, want) {
		t.Errorf("feasts = %q, want %q", got, want)
	}
}

func TestWeekPDFUsesParishSettings(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	err := h.db.Migrator().DropTable(&settings.Setting{})
	if err != nil {
		t.Fatalf("dropping settings table: %v", err)
	}

	if rec := getPDF(t, h, "/pdf"); rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d (the parish name is part of the sheet)", rec.Code, http.StatusInternalServerError)
	}
}

func TestImportFeastsNeverAddsOrdinaryWeekdays(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	h.fetchCelebrations = fakeCelebrations(map[int][]gcatholic.Celebration{2026: {
		{Date: "2026-11-02", Name: "Wspomnienie wszystkich wiernych zmarłych"},
		{Date: "2026-11-05", Name: "Czwartek XXXI tygodnia zwykłego"},
		{Date: "2026-11-06", Name: "Piątek XXXI tygodnia zwykłego"},
		{Date: "2026-11-08", Name: "XXXII Niedziela Zwykła"},
	}}, nil)

	postImport(t, h, "2026-11-02")

	got, err := calendar.FeastsForWeek(h.db, time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err != nil {
		t.Fatalf("loading feasts: %v", err)
	}
	want := map[string]string{
		"2026-11-02": "Wspomnienie wszystkich wiernych zmarłych",
		"2026-11-08": "XXXII Niedziela Zwykła",
	}
	if !maps.Equal(got, want) {
		t.Errorf("feasts = %q, want %q (ordinary weekdays are never imported)", got, want)
	}
}

func serveRouter(t *testing.T, db *gorm.DB, method, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	router := chi.NewRouter()
	RegisterRoutes(router, db)

	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req := httptest.NewRequest(method, target, body)
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func TestSettingsListsSchedules(t *testing.T) {
	h := newTestHandler(t, time.Now())

	body := serveRouter(t, h.db, http.MethodGet, "/settings?week=2026-11-04", nil).Body.String()

	for _, want := range []string{
		`<section class="settings-section schedules" id="porzadki" aria-labelledby="schedules-title">`,
		`<p class="schedule-name">Porządek zwykły</p>`,
		`<p class="schedule-summary">Pn: 6:30, 18:00 · Nd: 7:00, 10:00</p>`,
		`<a class="action" href="/settings/schedules/1?week=2026-11-02" aria-label="Zmień porządek Porządek zwykły">Zmień</a>`,
		`<a class="action action-delete" href="/settings?delete_schedule=2&amp;week=2026-11-02" aria-label="Usuń porządek Wakacje">Usuń</a>`,
		`<a class="button" href="/settings/schedules/new?week=2026-11-02">+ Dodaj porządek</a>`,
		`<a class="delete-link" href="/settings?restore=1&amp;week=2026-11-02">Przywróć domyślne</a>`,
		`<p class="app-credit">Go Parafia · autor: Tomasz Kaliński, Gemma It Solutions</p>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if strings.Contains(body, "inert") {
		t.Error("settings page is inert without a dialog")
	}
}

func TestScheduleFormNewAndEdit(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	body := serveRouter(t, db, http.MethodGet, "/settings/schedules/new?week=2026-11-02", nil).Body.String()
	for _, want := range []string{
		`<h1>Nowy porządek mszy</h1>`,
		`<form class="settings-form" method="post" action="/settings/schedules">`,
		`<a class="button" href="/settings?week=2026-11-02#porzadki" data-close>← Wróć do ustawień</a>`,
		`<label class="label" for="day-1">Poniedziałek</label>`,
		`id="day-0" name="day-0" value=""`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("new form does not contain %q", want)
		}
	}

	body = serveRouter(t, db, http.MethodGet, "/settings/schedules/1", nil).Body.String()
	for _, want := range []string{
		`<h1>Zmień porządek mszy</h1>`,
		`<form class="settings-form" method="post" action="/settings/schedules/1">`,
		`name="name" value="Porządek zwykły"`,
		`id="day-1" name="day-1" value="6:30, 18:00"`,
		`id="day-0" name="day-0" value="7:00, 10:00"`,
		`id="day-2" name="day-2" value=""`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit form does not contain %q", want)
		}
	}

	if rec := serveRouter(t, db, http.MethodGet, "/settings/schedules/99", nil); rec.Code != http.StatusNotFound {
		t.Errorf("missing schedule: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestCreateAndUpdateSchedule(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := serveRouter(t, db, http.MethodPost, "/settings/schedules", url.Values{
		"week": {"2026-11-02"}, "name": {"Zimowy"}, "day-1": {"7:00"}, "day-0": {"8:00, 11:00"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/settings?saved=1&week=2026-11-02#porzadki"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	rec = serveRouter(t, db, http.MethodPost, "/settings/schedules/2", url.Values{"name": {"Wakacje 2027"}, "day-0": {"9:30"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	schedules, err := masses.Load(db)
	if err != nil {
		t.Fatalf("loading schedules: %v", err)
	}
	var got []string
	for _, s := range schedules {
		got = append(got, s.Name+" → "+scheduleSummary(s))
	}
	want := []string{
		"Porządek zwykły → Pn: 6:30, 18:00 · Nd: 7:00, 10:00",
		"Wakacje 2027 → Nd: 9:30",
		"Zimowy → Pn: 7:00 · Nd: 8:00, 11:00",
	}
	if !slices.Equal(got, want) {
		t.Errorf("schedules = %q, want %q", got, want)
	}
}

func TestSaveScheduleInvalid(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	rec := serveRouter(t, db, http.MethodPost, "/settings/schedules/1", url.Values{
		"name": {"Wakacje"}, "day-1": {"6:30, 18:00"}, "day-3": {"6:10, 18:00"},
	})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<p class="field-error" id="name-error">Porządek o tej nazwie już istnieje.</p>`,
		`<p class="field-error" id="day-3-error">Godzina „6:10” nie jest pełnym kwadransem (:00, :15, :30, :45).</p>`,
		`id="day-3" name="day-3" value="6:10, 18:00"`,
		`id="day-1" name="day-1" value="6:30, 18:00"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}

	s, err := masses.Find(db, 1)
	if err != nil {
		t.Fatalf("loading schedule: %v", err)
	}
	if s.Name != "Porządek zwykły" {
		t.Errorf("schedule changed despite errors: %q", s.Name)
	}
}

func TestDeleteScheduleWithConfirmation(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	body := serveRouter(t, db, http.MethodGet, "/settings?delete_schedule=2&week=2026-11-02", nil).Body.String()
	for _, want := range []string{
		`<main class="settings" inert>`,
		`<h2 class="dialog-title" id="dialog-title">Usunąć porządek mszy?</h2>`,
		`„Wakacje” (Nd: 9:00).`,
		`<form class="dialog-actions" method="post" action="/settings/schedules/2/delete">`,
		`<a class="button" href="/settings?week=2026-11-02#porzadki" data-close autofocus>Anuluj</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dialog does not contain %q", want)
		}
	}

	rec := serveRouter(t, db, http.MethodPost, "/settings/schedules/2/delete", url.Values{"week": {"2026-11-02"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	schedules, err := masses.Load(db)
	if err != nil {
		t.Fatalf("loading schedules: %v", err)
	}
	if len(schedules) != 1 || schedules[0].Name != "Porządek zwykły" {
		t.Errorf("schedules after delete = %+v", schedules)
	}

	if rec := serveRouter(t, db, http.MethodPost, "/settings/schedules/2/delete", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("deleting again: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestRestoreSchedulesWithConfirmation(t *testing.T) {
	db := newTestHandler(t, time.Now()).db

	body := serveRouter(t, db, http.MethodGet, "/settings?restore=1", nil).Body.String()
	if !strings.Contains(body, `<form class="dialog-actions" method="post" action="/settings/schedules/restore">`) {
		t.Error("restore dialog is missing")
	}

	rec := serveRouter(t, db, http.MethodPost, "/settings/schedules/restore", url.Values{})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	schedules, err := masses.Load(db)
	if err != nil {
		t.Fatalf("loading schedules: %v", err)
	}
	defaults, err := masses.Default()
	if err != nil {
		t.Fatalf("loading defaults: %v", err)
	}
	if len(schedules) != len(defaults) || schedules[0].Name != defaults[0].Name {
		t.Errorf("schedules after restore = %d (first %q), want the %d defaults", len(schedules), schedules[0].Name, len(defaults))
	}
}

func TestWeekShowsEmptyMassesNotice(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPlaceholder(t, h.db, "2026-11-03 06:30")

	body := getWeek(t, h, "/").Body.String()

	want := `<p class="week-notice" role="status"><span>1 msza nie ma uzupełnionej intencji.</span> <a href="#d-2026-11-03">Pokaż</a></p>`
	if !strings.Contains(body, want) {
		t.Errorf("body does not contain %q", want)
	}
}

func TestCreateIntentionWithFlags(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	form := validForm()
	form.Set("deceased", "on")
	form.Set("gregorian", "on")

	rec := postIntention(t, h, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	var saved intentions.Intention
	err := h.db.First(&saved).Error
	if err != nil {
		t.Fatalf("loading intention: %v", err)
	}
	if !saved.IsDeceased || !saved.IsGregorian {
		t.Errorf("flags = deceased %v, gregorian %v; want both true", saved.IsDeceased, saved.IsGregorian)
	}
}

func TestWeekShowsIntentionBadges(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	marked := newIntention(t, 0, "2026-11-02 18:00", true, "+ Anna Kutyba")
	marked.IsDeceased = true
	marked.IsGregorian = true
	err := h.db.Create(&marked).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}
	seedIntention(t, h.db, "2026-11-02 06:30", false, "W int. Parafian")

	body := getWeek(t, h, "/").Body.String()

	for _, want := range []string{
		`<span>&#43; Anna Kutyba <span class="badge badge-deceased" title="Intencja za zmarłych">`,
		`</svg>za zmarłych</span> <span class="badge badge-gregorian" title="Msza gregoriańska">`,
		`</svg>gregorianka</span></span>`,
		`<span>W int. Parafian</span>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
	if got := strings.Count(body, `class="badge `); got != 2 {
		t.Errorf("badges = %d, want 2 (only the marked intention)", got)
	}
}

func TestEditPanelShowsFlags(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	marked := newIntention(t, 0, "2026-11-02 18:00", false, "+ Anna Kutyba")
	marked.IsGregorian = true
	err := h.db.Create(&marked).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}

	body := getWeek(t, h, fmt.Sprintf("/?week=2026-11-02&edit=%d", marked.ID)).Body.String()

	for _, want := range []string{
		`<input type="checkbox" name="deceased"> Za zmarłych`,
		`<input type="checkbox" name="gregorian" checked> Msza gregoriańska`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit panel does not contain %q", want)
		}
	}
}
