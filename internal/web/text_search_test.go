package web

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestParseSearchQueryUntil(t *testing.T) {
	tests := []struct {
		query  string
		period string
		until  string
	}{
		{query: "period=date&until=2027-01-10", period: "date", until: "2027-01-10"},
		{query: "period=date&until=2026-11-01", period: "date", until: "2026-11-17"},
		{query: "period=date&until=2030-01-01", period: "date", until: "2028-11-16"},
		{query: "period=m1&until=2027-01-10", period: "m1", until: ""},
		{query: "period=date&until=zła", period: "m3", until: ""},
		{query: "period=date", period: "m3", until: ""},
	}

	for _, test := range tests {
		query, _ := url.ParseQuery(test.query)
		q := parseSearchQuery(query, searchNow, 0, searchSchedules)
		if q.Period != test.period || q.Until != test.until {
			t.Errorf("%s: period %q until %q, want %q %q", test.query, q.Period, q.Until, test.period, test.until)
		}
	}

	query, _ := url.ParseQuery("period=date&until=2027-01-10")
	q := parseSearchQuery(query, searchNow, 0, searchSchedules)
	today := startOfDay(searchNow)
	if got, want := q.end(today), time.Date(2027, 1, 11, 0, 0, 0, 0, intentions.TimeZone); !got.Equal(want) {
		t.Errorf("end = %v, want %v", got, want)
	}
	if values := q.values(); values.Get("period") != "date" || values.Get("until") != "2027-01-10" {
		t.Errorf("values = %v, want the chosen end date", values)
	}

	query, _ = url.ParseQuery("period=y1")
	if got, want := parseSearchQuery(query, searchNow, 0, searchSchedules).end(today), today.AddDate(1, 0, 0); !got.Equal(want) {
		t.Errorf("end of y1 = %v, want %v", got, want)
	}
}

func TestParseSearchQueryText(t *testing.T) {
	query, _ := url.ParseQuery("q=++Kowal++")
	q := parseSearchQuery(query, searchNow, 0, searchSchedules)
	if q.Text != "Kowal" || q.values().Get("q") != "Kowal" {
		t.Errorf("text = %q, values = %v, want trimmed text", q.Text, q.values())
	}

	query = url.Values{"q": {strings.Repeat("ż", 150)}}
	if q := parseSearchQuery(query, searchNow, 0, searchSchedules); q.Text != strings.Repeat("ż", maxSearchText) {
		t.Errorf("long text kept %d runes, want %d", len([]rune(q.Text)), maxSearchText)
	}
}

func TestSearchCustomPeriod(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=date&until=2026-12-10&view=calendar")

	checkBody(t, body,
		`<input type="radio" id="f-period-date" name="period" value="date" checked>`,
		`id="f-until" name="until" value="2026-12-10" min="2026-11-17" max="2028-11-16" aria-label="Koniec okresu" data-date-picker="10.12.2026" data-today="2026-11-17" data-range-start="2026-11-17" data-picker-radio="f-period-date"`,
		`id="f-from" name="from" value="2026-11-17" min="2000-01-01" max="2028-11-16" aria-label="Początek okresu" data-date-picker="dziś, 17.11.2026" data-today="2026-11-17" data-range-end="2026-12-10"`,
		`od 17.11.2026 do 10.12.2026`,
		`<h2>Listopad 2026</h2>`,
		`<h2>Grudzień 2026</h2>`,
		`<a href="/search?period=date&amp;schedule=1&amp;until=2026-12-10&amp;week=2026-11-23">`,
	)
	if strings.Contains(body, `<h2>Styczeń 2027</h2>`) {
		t.Error("calendar goes past the chosen end date")
	}

	body = getSearch(t, h, "/search?period=m1")
	checkBody(t, body,
		`<input type="radio" id="f-period-date" name="period" value="date">`,
		`value="2026-12-16" min="2026-11-17" max="2028-11-16" aria-label="Koniec okresu" data-date-picker=""`,
	)
}

func seedTextSearchData(t *testing.T, h *handler) map[string]int {
	t.Helper()

	ids := map[string]int{}
	add := func(at string, outside, hidden bool, content string) {
		intention := newIntention(t, 0, at, outside, content)
		intention.IsHidden = hidden
		if err := h.db.Create(&intention).Error; err != nil {
			t.Fatal(err)
		}
		ids[content] = intention.ID
	}

	add("2026-11-30 06:30", true, false, "W intencji Łukasza Kowalczyka")
	add("2026-11-23 18:00", false, false, "Za + Jana Kowalskiego")
	add("2025-03-01 18:00", false, true, "Za + Annę Kowalską")
	add("2026-10-05 18:00", false, false, "Za + Józefa KOWALSKIEGO")
	add("2026-11-24 06:30", false, false, "Za + Stefana")
	seedSlot(t, h, "2026-11-25 06:30")

	return ids
}

func TestTextSearchPage(t *testing.T) {
	h := newTestHandler(t, searchNow)
	ids := seedTextSearchData(t, h)

	body := getSearch(t, h, "/search?q=kowal&n=0&period=m1&view=calendar")

	checkBody(t, body,
		`<input class="input search-text-input" type="search" id="f-q" name="q" value="kowal"`,
		`<input type="hidden" name="n" value="0">`,
		`<input type="hidden" name="period" value="m1">`,
		`<input type="hidden" name="view" value="calendar">`,
		`<a class="search-text-clear" href="/search?n=0&amp;period=m1&amp;schedule=1&amp;view=calendar&amp;week=2026-11-23" aria-label="Wyczyść tekst"`,
		`<p class="search-total">Znaleziono 4 intencje</p>`,
		`<h2>Nadchodzące</h2>`,
		`<h2>Wcześniejsze – Msze, które już były</h2>`,
		`<a class="text-hit" href="/?edit=`+strconv.Itoa(ids["Za + Jana Kowalskiego"])+`&amp;week=2026-11-23">`,
		`<span class="text-hit-when"><b>Pn 23.11.2026</b><span>18:00</span></span>`,
		`<span class="text-hit-content">Za &#43; Jana <mark>Kowal</mark>skiego</span>`,
		`<span class="text-hit-content is-outside">(W intencji Łukasza <mark>Kowal</mark>czyka)</span> <span class="badge badge-outside">poza parafią</span>`,
		`<span class="text-hit-content">Za &#43; Józefa <mark>KOWAL</mark>SKIEGO</span>`,
		`<span class="text-hit-content">Za &#43; Annę <mark>Kowal</mark>ską</span>`,
		`ukryta na wydruku`,
	)
	for _, unwanted := range []string{`class="search-filters`, `Stefana`, `class="search-calendar"`} {
		if strings.Contains(body, unwanted) {
			t.Errorf("text search page contains %q", unwanted)
		}
	}

	order := []string{"Jana <mark>", "Łukasza <mark>", "Józefa <mark>", "Annę <mark>"}
	for i := 1; i < len(order); i++ {
		if strings.Index(body, order[i-1]) > strings.Index(body, order[i]) {
			t.Errorf("%q is shown after %q, want upcoming first, then the latest past", order[i-1], order[i])
		}
	}
}

func TestTextSearchFoldsPolishLetters(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedTextSearchData(t, h)

	body := getSearch(t, h, "/search?q=LUKASZ")
	checkBody(t, body, `<p class="search-total">Znaleziono 1 intencję</p>`, `W intencji <mark>Łukasz</mark>a Kowalczyka`)

	body = getSearch(t, h, "/search?q=józef")
	checkBody(t, body, `<p class="search-total">Znaleziono 1 intencję</p>`, `Za &#43; <mark>Józef</mark>a KOWALSKIEGO`)
}

func TestTextSearchShortAndMissing(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedTextSearchData(t, h)

	body := getSearch(t, h, "/search?q=ko")
	checkBody(t, body,
		`<p class="search-text-hint" id="f-q-hint">Wpisz co najmniej 3 znaki, aby szukać w treści intencji.</p>`,
		`value="ko"`,
		`class="search-filters`,
	)

	body = getSearch(t, h, "/search?q=xyz")
	checkBody(t, body,
		`<p class="search-total">Znaleziono 0 intencji</p>`,
		`<p class="search-empty-title">Brak intencji z tekstem „xyz”</p>`,
	)
}

func TestTextSearchShowsMore(t *testing.T) {
	h := newTestHandler(t, searchNow)
	for i := range searchPageDays + 5 {
		seedIntention(t, h.db, time.Date(2026, 12, 1, 6, 30, 0, 0, intentions.TimeZone).AddDate(0, 0, i).Format(intentions.MassAtLayout), false, "Za + Kowalskiego nr "+strconv.Itoa(i))
	}

	body := getSearch(t, h, "/search?q=kowal")
	checkBody(t, body, `Pokaż kolejne intencje (5)</a>`, `show=60`)
	if got := strings.Count(body, `class="text-hit"`); got != searchPageDays {
		t.Errorf("shown %d hits, want %d", got, searchPageDays)
	}

	body = getSearch(t, h, "/search?q=kowal&show=60")
	if got := strings.Count(body, `class="text-hit"`); got != searchPageDays+5 || strings.Contains(body, "Pokaż kolejne") {
		t.Errorf("second page shows %d hits, want all %d without a more link", got, searchPageDays+5)
	}
}

func TestParseSearchQueryFrom(t *testing.T) {
	tests := []struct {
		query string
		from  string
		until string
		end   string
	}{
		{query: "from=2026-10-01&period=m1", from: "2026-10-01", end: "2026-11-01"},
		{query: "from=2026-11-17", from: "", end: "2027-02-17"},
		{query: "from=1990-05-05&period=m1", from: "2000-01-01", end: "2000-02-01"},
		{query: "from=2031-01-01&period=m1", from: "2028-11-16", end: "2028-12-16"},
		{query: "from=zła&period=m1", from: "", end: "2026-12-17"},
		{query: "from=2026-10-01&period=date&until=2026-09-01", from: "2026-10-01", until: "2026-10-01", end: "2026-10-02"},
		{query: "from=2026-10-01&period=date&until=2029-01-01", from: "2026-10-01", until: "2028-09-30", end: "2028-10-01"},
	}

	today := startOfDay(searchNow)
	for _, test := range tests {
		query, _ := url.ParseQuery(test.query)
		q := parseSearchQuery(query, searchNow, 0, searchSchedules)
		end := q.end(today).Format(intentions.DateLayout)
		if q.From != test.from || q.Until != test.until || end != test.end {
			t.Errorf("%s: from %q until %q end %s, want %q %q %s", test.query, q.From, q.Until, end, test.from, test.until, test.end)
		}
	}
}

func TestSearchFromPastDay(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?from=2026-11-09&period=m1&planned=1&schedule=1")

	checkBody(t, body,
		`od 9.11.2026 do 8.12.2026`,
		`data-date-picker="9.11.2026" data-today="2026-11-17"`,
		`<a class="link-action period-today" href="/search?period=m1&amp;planned=1&amp;schedule=1&amp;week=2026-11-23" title="Zacznij okres od dzisiaj">dziś</a>`,
		`>Pn 16.11</a>`,
		`>Pn 23.11</a>`,
	)
	if strings.Contains(body, `>Wt 10.11</a>`) {
		t.Error("planned Masses are shown for days that already passed")
	}

	body = getSearch(t, h, "/search?from=2026-11-01&period=m1&view=calendar")
	checkBody(t, body,
		`<span class="scal-cell is-none is-today"><span class="scal-num">17</span></span>`,
		`aria-label="Poniedziałek 16.11: 1 Msza, pasuje 1"`,
	)
	if strings.Contains(body, `<span class="scal-cell is-none is-today"><span class="scal-num">1</span>`) {
		t.Error("the first day of the period is marked as today")
	}
}
