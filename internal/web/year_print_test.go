package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

var yearNow = time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone)

func TestRangeInWords(t *testing.T) {
	day := func(y int, m time.Month, d int) time.Time { return time.Date(y, m, d, 0, 0, 0, 0, intentions.TimeZone) }
	tests := []struct {
		from, to time.Time
		want     string
	}{
		{from: day(2025, 1, 1), to: day(2025, 1, 5), want: "1 – 5 stycznia 2025"},
		{from: day(2025, 12, 29), to: day(2025, 12, 31), want: "29 – 31 grudnia 2025"},
		{from: day(2025, 1, 1), to: day(2025, 1, 1), want: "1 stycznia 2025"},
		{from: day(2025, 3, 31), to: day(2025, 4, 6), want: "31 marca – 6 kwietnia 2025"},
	}

	for _, tt := range tests {
		if got := rangeInWords(tt.from, tt.to); got != tt.want {
			t.Errorf("rangeInWords(%s, %s) = %q, want %q", tt.from.Format("2.1"), tt.to.Format("2.1"), got, tt.want)
		}
	}
}

func yearIntentions(t *testing.T, values ...string) []intentions.Intention {
	t.Helper()

	var list []intentions.Intention
	for i, value := range values {
		intention := registerIntention(t, value, "Intencja "+value)
		intention.ID = i + 1
		list = append(list, intention)
	}

	return list
}

func TestSplitWeeks(t *testing.T) {
	list := yearIntentions(t, "2025-01-01 18:00", "2025-01-06 07:00", "2025-12-31 18:00")

	weeks := splitWeeks(2025, list)

	if got, want := len(weeks), 53; got != want {
		t.Fatalf("weeks = %d, want %d", got, want)
	}
	if got := weeks[0].Start.Format(intentions.DateLayout); got != "2024-12-30" {
		t.Errorf("first week starts %s, want 2024-12-30", got)
	}
	if got := weeks[52].Start.Format(intentions.DateLayout); got != "2025-12-29" {
		t.Errorf("last week starts %s, want 2025-12-29", got)
	}
	count := func(view weekView) int {
		n := 0
		forEachIntention(view.Days, func(*intentionView) { n++ })
		return n
	}
	if count(weeks[0]) != 1 || count(weeks[1]) != 1 || count(weeks[52]) != 1 {
		t.Errorf("intentions per week: first %d, second %d, last %d, want 1 each", count(weeks[0]), count(weeks[1]), count(weeks[52]))
	}
}

func TestLayoutYearClipsWeeksAndSkipsEmptyOnes(t *testing.T) {
	list := yearIntentions(t, "2025-01-01 18:00", "2025-03-05 07:00", "2025-12-31 18:00")
	weeks := splitWeeks(2025, list)

	pages := layoutYear(2025, weeks, pdfOptions{}, fakeMeasurer{})

	if len(pages) != 3 {
		t.Fatalf("pages = %d, want 3 (only weeks with intentions)", len(pages))
	}
	for i, want := range []string{"1 – 5 stycznia 2025", "3 – 9 marca 2025", "29 – 31 grudnia 2025"} {
		texts := pageTexts(pages[i])
		if !slices.Contains(texts, want) {
			t.Errorf("page %d has no range %q", i+1, want)
		}
		if label := "Intencje mszy świętych – rok 2025 · str. " + string(rune('1'+i)) + " z 3"; !slices.Contains(texts, label) {
			t.Errorf("page %d has no footer %q", i+1, label)
		}
		for _, text := range texts {
			if strings.Contains(text, "grudnia 2024") || strings.Contains(text, "stycznia 2026") {
				t.Errorf("page %d prints a day outside 2025: %q", i+1, text)
			}
		}
	}
	first := pageTexts(pages[0])
	if slices.Contains(first, "Poniedziałek") || slices.Contains(first, "Wtorek") {
		t.Error("first page prints 30 and 31 December 2024")
	}
}

func TestLayoutYearWithoutIntentions(t *testing.T) {
	pages := layoutYear(2025, splitWeeks(2025, nil), pdfOptions{}, fakeMeasurer{})

	if len(pages) != 1 || !slices.Contains(pageTexts(pages[0]), "Brak intencji w roku 2025.") {
		t.Errorf("pages = %+v, want one page with a message", pages)
	}
}

func TestLayoutYearHiddenIntentions(t *testing.T) {
	list := yearIntentions(t, "2025-05-06 18:00")
	list[0].IsHidden = true
	weeks := splitWeeks(2025, list)

	if pages := layoutYear(2025, weeks, pdfOptions{}, fakeMeasurer{}); !slices.Contains(pageTexts(pages[0]), "Brak intencji w roku 2025.") {
		t.Error("hidden intention printed without showHidden")
	}
	if pages := layoutYear(2025, weeks, pdfOptions{hidden: hiddenInclude}, fakeMeasurer{}); !slices.Contains(pageTexts(pages[0]), "Intencja 2025-05-06 18:00") {
		t.Error("hidden intention missing with showHidden")
	}
}

func getYear(t *testing.T, h *handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	if strings.HasPrefix(target, "/pdf/") {
		h.yearPDF(rec, req)
	} else {
		h.yearPrintPage(rec, req)
	}

	return rec
}

func TestYearPDF(t *testing.T) {
	h := newTestHandler(t, yearNow)
	seedIntention(t, h.db, "2025-06-10 18:00", false, "+ Jan Kowalski")

	rec := getYear(t, h, "/pdf/year?year=2025")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="intencje-2025.pdf"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Error("body does not start with %PDF-")
	}
}

func TestYearPrintRejectsCurrentAndFutureYears(t *testing.T) {
	h := newTestHandler(t, yearNow)

	for _, target := range []string{"/pdf/year?year=2026", "/pdf/year?year=2027", "/pdf/year?year=abc", "/print/year?year=2026", "/print/year"} {
		if rec := getYear(t, h, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestYearPrintPreview(t *testing.T) {
	h := newTestHandler(t, yearNow)
	hidden := newIntention(t, 0, "2025-06-10 18:00", false, "ukryta")
	hidden.IsHidden = true
	h.db.Create(&hidden)

	body := getYear(t, h, "/print/year?year=2025&week=2026-11-23").Body.String()
	checkBody(t, body,
		`<title>Wydruk roku 2025 – Go Parafia</title>`,
		`<a class="button" href="/?week=2026-11-23&amp;calendar=2025" data-close>← Wróć do kalendarza</a>`,
		`<input type="checkbox" name="hidden" value="1" data-autosubmit> Pokaż ukryte intencje (1)`,
		`<form class="print-save" method="post" action="/print/year/save">`,
		`<iframe class="print-preview" src="/pdf/year?year=2025"`,
	)

	body = getYear(t, h, "/print/year?year=2025&week=2026-11-23&hidden=1&saved="+url.QueryEscape(`C:\Users\x\intencje-2025.pdf`)).Body.String()
	checkBody(t, body,
		`name="hidden" value="1" checked data-autosubmit`,
		`src="/pdf/year?hidden=1&amp;year=2025"`,
		`<p class="print-message" role="status">Zapisano plik: C:\Users\x\intencje-2025.pdf</p>`,
	)
}

func postYearSave(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/print/year/save", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.saveYearPDF(rec, req)

	return rec
}

func TestSaveYearPDF(t *testing.T) {
	form := url.Values{"year": {"2025"}, "week": {"2026-11-23"}}

	t.Run("download without a save dialog", func(t *testing.T) {
		h := newTestHandler(t, yearNow)
		rec := postYearSave(t, h, form)
		if got, want := rec.Header().Get("Content-Disposition"), `attachment; filename="intencje-2025.pdf"`; got != want {
			t.Errorf("Content-Disposition = %q, want %q", got, want)
		}
	})

	t.Run("save dialog", func(t *testing.T) {
		h := newTestHandler(t, yearNow)
		path := filepath.Join(t.TempDir(), "rok.pdf")
		var offered string
		h.saveFile = func(name string) (string, error) {
			offered = name
			return path, nil
		}

		rec := postYearSave(t, h, form)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		if offered != "intencje-2025.pdf" {
			t.Errorf("offered name %q, want intencje-2025.pdf", offered)
		}
		if got, want := rec.Header().Get("Location"), "/print/year?saved="+url.QueryEscape(path)+"&week=2026-11-23&year=2025"; got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
		data, err := os.ReadFile(path)
		if err != nil || !strings.HasPrefix(string(data), "%PDF-") {
			t.Errorf("saved file: %v, starts with %q", err, string(data[:min(len(data), 5)]))
		}
	})

	t.Run("dialog cancelled", func(t *testing.T) {
		h := newTestHandler(t, yearNow)
		h.saveFile = func(string) (string, error) { return "", nil }

		rec := postYearSave(t, h, form)
		if got, want := rec.Header().Get("Location"), "/print/year?week=2026-11-23&year=2025"; got != want {
			t.Errorf("Location = %q, want %q", got, want)
		}
	})

	t.Run("dialog error", func(t *testing.T) {
		h := newTestHandler(t, yearNow)
		h.saveFile = func(string) (string, error) { return "", errors.New("window is not ready") }

		if rec := postYearSave(t, h, form); rec.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
		}
	})
}

func TestCalendarOffersYearPrintOnlyForPastYearsWithIntentions(t *testing.T) {
	h := newTestHandler(t, yearNow)
	seedIntention(t, h.db, "2025-06-10 18:00", false, "+ Jan Kowalski")

	link := `href="/print/year?week=2026-11-23&amp;year=2025"`
	if body := getWeek(t, h, "/?week=2026-11-23&calendar=2025").Body.String(); !strings.Contains(body, link) {
		t.Errorf("calendar 2025 has no %q", link)
	}
	for _, year := range []string{"2024", "2026"} {
		if body := getWeek(t, h, "/?week=2026-11-23&calendar="+year).Body.String(); strings.Contains(body, "/print/year") {
			t.Errorf("calendar %s offers a year print", year)
		}
	}
}
