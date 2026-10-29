package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/intentions"
)

func newTestHandler(now time.Time) *handler {
	return &handler{now: func() time.Time { return now }}
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
			rec := getWeek(t, newTestHandler(tt.now), "/")

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
	h := newTestHandler(time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

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
		"<h1>INTENCJE MSZY ŚWIĘTYCH – 9.11.2026 r. – 15.11.2026 r.</h1>",
		`href="/?week=2026-11-02"`,
		`href="/?week=2026-11-16"`,
		`id="d-2026-11-09"`,
		`id="d-2026-11-15"`,
		"Poniedziałek 9.11.",
		"Niedziela 15.11.",
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
}

func TestWeekRejectsInvalidDate(t *testing.T) {
	h := newTestHandler(time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

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
