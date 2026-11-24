package web

import (
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

var dayPrintNow = time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)

func TestHiddenModes(t *testing.T) {
	tests := []struct {
		param         string
		mode          hiddenMode
		visible, hide bool
	}{
		{"", hiddenSkip, true, false},
		{"0", hiddenSkip, true, false},
		{"1", hiddenInclude, true, true},
		{"only", hiddenOnly, false, true},
	}

	for _, tt := range tests {
		mode := parseHiddenMode(tt.param)
		if mode != tt.mode {
			t.Errorf("parseHiddenMode(%q) = %d, want %d", tt.param, mode, tt.mode)
		}
		if mode.prints(false) != tt.visible || mode.prints(true) != tt.hide {
			t.Errorf("mode %q prints visible=%v hidden=%v, want %v %v", tt.param, mode.prints(false), mode.prints(true), tt.visible, tt.hide)
		}
		if tt.param != "0" && mode.param() != tt.param {
			t.Errorf("param() = %q, want %q", mode.param(), tt.param)
		}
	}
}

func TestPrintOnlyHiddenIgnoresBlanks(t *testing.T) {
	query, _ := url.ParseQuery("hidden=only&blanks=1&lines=2")

	opts := parsePrintOptions(query, 0, nil, hiddenSkip)

	if opts.Hidden != hiddenOnly || opts.Blanks {
		t.Errorf("options = %+v, want only hidden without blanks", opts)
	}
}

func TestPrintableDaysOnlyHidden(t *testing.T) {
	got := describeMasses(printableDays(blankTestWeek(t), pdfOptions{hidden: hiddenOnly}))

	if want := []string{"04.11 6:30 ukryta"}; !reflect.DeepEqual(got, want) {
		t.Errorf("masses = %q, want %q", got, want)
	}
}

func TestParsePrintDay(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		value string
		ok    bool
		want  string
	}{
		{"", true, ""},
		{"2026-11-02", true, "2026-11-02"},
		{"2026-11-08", true, "2026-11-08"},
		{"2026-11-09", false, ""},
		{"2026-11-01", false, ""},
		{"4.11.2026", false, ""},
	}

	for _, tt := range tests {
		day, ok := parsePrintDay(tt.value, start)
		got := ""
		if !day.IsZero() {
			got = day.Format(intentions.DateLayout)
		}
		if ok != tt.ok || got != tt.want {
			t.Errorf("parsePrintDay(%q) = %q, %v, want %q, %v", tt.value, got, ok, tt.want, tt.ok)
		}
	}
}

func TestLayoutWeekForOneDay(t *testing.T) {
	view := blankTestWeek(t)
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, intentions.TimeZone)

	pages := layoutWeek(view, pdfOptions{day: day}, fakeMeasurer{})

	if len(pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(pages))
	}
	texts := strings.Join(pageTexts(pages[0]), "\n")
	for _, want := range []string{"3 listopada 2026", "+ Anna"} {
		if !strings.Contains(texts, want) {
			t.Errorf("page does not contain %q:\n%s", want, texts)
		}
	}
	for _, unwanted := range []string{"+ Jan", "Za parafian", "8 listopada"} {
		if strings.Contains(texts, unwanted) {
			t.Errorf("page contains %q from another day:\n%s", unwanted, texts)
		}
	}
	if len(pages[0].Rules) != 2 {
		t.Errorf("rules = %d, want only the two header lines (no line below the single day)", len(pages[0].Rules))
	}
}

func TestPrintFileName(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	day := time.Date(2026, 11, 4, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		opts printOptions
		want string
	}{
		{printOptions{}, "intencje-2026-11-02.pdf"},
		{printOptions{Hidden: hiddenInclude}, "intencje-2026-11-02.pdf"},
		{printOptions{Day: day}, "intencje-2026-11-04.pdf"},
		{printOptions{Day: day, Hidden: hiddenOnly}, "intencje-2026-11-04-ukryte.pdf"},
		{printOptions{Blanks: true}, "intencje-2026-11-02-do-wpisania.pdf"},
	}

	for _, tt := range tests {
		if got := printFileName(start, tt.opts); got != tt.want {
			t.Errorf("printFileName(%+v) = %q, want %q", tt.opts, got, tt.want)
		}
	}
}

func TestWeekShowsDayPrintLinks(t *testing.T) {
	body := getWeek(t, newTestHandler(t, dayPrintNow), "/?week=2026-11-02").Body.String()

	want := `<a class="button-small day-print" href="/print?week=2026-11-02&amp;day=2026-11-04" title="Drukuj intencje z tego dnia" aria-label="Drukuj intencje – Środa 4.11.">`
	if !strings.Contains(body, want) {
		t.Errorf("week does not contain %q", want)
	}
	if got := strings.Count(body, `class="button-small day-print"`); got != 7 {
		t.Errorf("day print links = %d, want 7", got)
	}
}

func TestDayPrintPreview(t *testing.T) {
	h := newTestHandler(t, dayPrintNow)
	seedHidden(t, h.db, "2026-11-04 18:00", "+ Jan Kowalski")
	seedHidden(t, h.db, "2026-11-05 18:00", "+ Anna Nowak")

	rec := getPrint(t, h, "/print?week=2026-11-02&day=2026-11-04&hidden=only")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	for _, want := range []string{
		`<title>Wydruk 4.11.2026 r. – Go Parafia</title>`,
		`<a class="button" href="/?week=2026-11-02#d-2026-11-04" data-close>← Wróć do tygodnia</a>`,
		`INTENCJE MSZY ŚWIĘTYCH – Środa, 4.11.2026 r.`,
		`<input type="hidden" name="day" value="2026-11-04">`,
		`<legend class="print-hidden-label">Ukryte intencje (1)</legend>`,
		`<input type="radio" id="p-hidden-2" name="hidden" value="only" checked data-autosubmit>`,
		`name="blanks" value="1" disabled data-autosubmit> Miejsca do wpisania`,
		`<iframe class="print-preview" src="/pdf?week=2026-11-02&amp;day=2026-11-04&amp;hidden=only"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("print page does not contain %q", want)
		}
	}
}

func TestDayPrintRejectsDayOutsideWeek(t *testing.T) {
	h := newTestHandler(t, dayPrintNow)

	for _, target := range []string{"/print?week=2026-11-02&day=2026-11-09", "/print?week=2026-11-02&day=jutro"} {
		if rec := getPrint(t, h, target); rec.Code != http.StatusBadRequest {
			t.Errorf("GET %s: status = %d, want %d", target, rec.Code, http.StatusBadRequest)
		}
	}
	if rec := getPDF(t, h, "/pdf?week=2026-11-02&day=2026-11-01"); rec.Code != http.StatusBadRequest {
		t.Errorf("pdf: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDayPDF(t *testing.T) {
	h := newTestHandler(t, dayPrintNow)
	seedIntention(t, h.db, "2026-11-04 18:00", false, "+ Jan Kowalski")
	seedHidden(t, h.db, "2026-11-04 18:00", "+ Anna Nowak")

	tests := []struct {
		target   string
		filename string
	}{
		{"/pdf?week=2026-11-02&day=2026-11-04", "intencje-2026-11-04.pdf"},
		{"/pdf?week=2026-11-02&day=2026-11-04&hidden=1", "intencje-2026-11-04.pdf"},
		{"/pdf?week=2026-11-02&day=2026-11-04&hidden=only", "intencje-2026-11-04-ukryte.pdf"},
	}

	for _, tt := range tests {
		rec := getPDF(t, h, tt.target)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want %d", tt.target, rec.Code, http.StatusOK)
		}
		if got := rec.Header().Get("Content-Disposition"); got != `inline; filename="`+tt.filename+`"` {
			t.Errorf("GET %s: Content-Disposition = %q, want %q", tt.target, got, tt.filename)
		}
		if !slices.Equal(rec.Body.Bytes()[:5], []byte("%PDF-")) {
			t.Errorf("GET %s: body is not a PDF", tt.target)
		}
	}
}

func TestParsePrintOptionsUsesFallbackOnlyWithoutParam(t *testing.T) {
	absent, _ := url.ParseQuery("week=2026-11-02")
	explicit, _ := url.ParseQuery("week=2026-11-02&hidden=")

	if got := parsePrintOptions(absent, 0, nil, hiddenOnly).Hidden; got != hiddenOnly {
		t.Errorf("without hidden: mode = %d, want the fallback %d", got, hiddenOnly)
	}
	if got := parsePrintOptions(explicit, 0, nil, hiddenOnly).Hidden; got != hiddenSkip {
		t.Errorf("with an empty hidden: mode = %d, want %d (Bez ukrytych chosen)", got, hiddenSkip)
	}
}

func TestPrintRemembersHiddenChoice(t *testing.T) {
	h := newTestHandler(t, dayPrintNow)
	seedIntention(t, h.db, "2026-11-04 18:00", false, "+ Jan Kowalski")
	seedHidden(t, h.db, "2026-11-04 18:00", "+ Anna Nowak")

	saved := func() string {
		t.Helper()
		setting, err := settings.Load(h.db)
		if err != nil {
			t.Fatal(err)
		}
		return setting.PrintHidden
	}

	getPrint(t, h, "/print?week=2026-11-02&day=2026-11-04&hidden=only")
	if got := saved(); got != "only" {
		t.Fatalf("saved = %q, want %q", got, "only")
	}

	body := getPrint(t, h, "/print?week=2026-11-02").Body.String()
	for _, want := range []string{
		`<input type="radio" id="p-hidden-2" name="hidden" value="only" checked data-autosubmit>`,
		`src="/pdf?week=2026-11-02&amp;hidden=only"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("week print after reopening does not contain %q", want)
		}
	}

	body = getPrint(t, h, "/print?week=2026-11-02&day=2026-11-03").Body.String()
	if !strings.Contains(body, `src="/pdf?week=2026-11-02&amp;day=2026-11-03"`) {
		t.Error("a day without hidden intentions should not print an empty page for the remembered Tylko ukryte")
	}
	if got := saved(); got != "only" {
		t.Errorf("saved = %q after opening a day without hidden intentions, want it unchanged", got)
	}

	getPrint(t, h, "/print?week=2026-11-02&hidden=")
	if got := saved(); got != "" {
		t.Errorf("saved = %q after choosing Bez ukrytych, want empty", got)
	}
	getPrint(t, h, "/print?week=2026-11-02&hidden=1")
	if got := saved(); got != "1" {
		t.Errorf("saved = %q after choosing Z ukrytymi, want %q", got, "1")
	}

	if rec := getPDF(t, h, "/pdf?week=2026-11-02&day=2026-11-04"); rec.Header().Get("Content-Disposition") != `inline; filename="intencje-2026-11-04.pdf"` {
		t.Errorf("pdf without hidden used the remembered choice: %q", rec.Header().Get("Content-Disposition"))
	}
}
