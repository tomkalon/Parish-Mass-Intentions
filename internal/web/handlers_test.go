package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func newTestHandler(t *testing.T, now time.Time) *handler {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&intentions.Intention{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return &handler{db: db, now: func() time.Time { return now }, validate: intentions.NewValidator()}
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
		`<a class="panel-close" href="/?week=2026-11-02" aria-label="Zamknij">×</a>`,
		`<section class="day is-selected" id="d-2026-11-03">`,
		`<span class="pill">wybrany w formularzu</span>`,
		`<div class="mass is-selected">`,
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
	router := chi.NewRouter()
	RegisterRoutes(router, db)

	req := httptest.NewRequest(http.MethodPost, "/intentions", strings.NewReader(validForm().Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
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
