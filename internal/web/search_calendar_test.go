package web

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseSearchQueryCalendar(t *testing.T) {
	query, _ := url.ParseQuery("view=calendar&day=2026-11-23")
	q := parseSearchQuery(query, searchNow, 0, searchSchedules)

	if !q.Calendar || q.Day != "2026-11-23" || len(q.Counts) != 1 || q.Planned || !q.SkipGregorian {
		t.Errorf("parse = %+v, want calendar view with the default filters and the chosen day", q)
	}

	query, _ = url.ParseQuery("day=2026-11-23&period=m1")
	if q := parseSearchQuery(query, searchNow, 0, searchSchedules); q.Day != "" {
		t.Errorf("day kept outside the calendar view: %+v", q)
	}

	query, _ = url.ParseQuery("view=calendar&day=zła")
	if q := parseSearchQuery(query, searchNow, 0, searchSchedules); q.Day != "" {
		t.Errorf("invalid day accepted: %+v", q)
	}
}

func TestSearchCalendarView(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m1&n=0&planned=1&skipgreg=1&schedule=1&view=calendar")

	checkBody(t, body,
		`<input type="hidden" name="view" value="calendar">`,
		`<a href="/search?n=0&amp;period=m1&amp;planned=1&amp;schedule=1&amp;skipgreg=1&amp;week=2026-11-23">`,
		`<a href="/search?n=0&amp;period=m1&amp;planned=1&amp;schedule=1&amp;skipgreg=1&amp;view=calendar&amp;week=2026-11-23" aria-current="true">`,
		`kliknij dzień, aby zobaczyć jego Msze`,
		`<h2>Listopad 2026</h2>`,
		`<h2>Grudzień 2026</h2>`,
		`<section class="search-preview" id="podglad" aria-label="Podgląd dnia">`,
		`<h2>Niedziela 22.11.2026</h2>`,
		`<span class="search-preview-status is-planned">planowana</span>`,
		`<span class="search-preview-note">pasuje do filtrów</span>`,
		`<a class="scal-cell has-tip is-match" href="/search?day=2026-11-23&amp;n=0&amp;period=m1&amp;planned=1&amp;schedule=1&amp;skipgreg=1&amp;view=calendar&amp;week=2026-11-23#podglad" aria-label="Poniedziałek 23.11: 2 Msze, pasuje 1">`,
		`<span class="scal-bars" aria-hidden="true"><span class="scal-bar is-free"></span><span class="scal-bar is-one"></span></span>`,
		`<span class="day-tip-title">Poniedziałek 23.11.</span>`,
		`<span class="day-tip-summary">2 Msze · 1 intencja · 1 wolna <i>(&#43;1 poza parafią)</i></span>`,
		`<span class="scal-cell is-none"><span class="scal-num">18</span></span>`,
	)
	if got := strings.Count(body, `<span class="scal-cell is-out"></span>`); got != 6+1 {
		t.Errorf("leading empty cells = %d, want 7 (November 2026 starts on Sunday, December on Tuesday)", got)
	}
	if strings.Contains(body, `class="search-month"`) {
		t.Error("the list of days is shown in the calendar view")
	}
}

func TestSearchCalendarMarksTodayWithoutMasses(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m1&n=0&view=calendar")
	checkBody(t, body, `<span class="scal-cell is-none is-today"><span class="scal-num">17</span></span>`)
}

func TestSearchCalendarSelectedDay(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m1&n=0&planned=1&skipgreg=1&schedule=1&view=calendar&day=2026-11-23")

	checkBody(t, body,
		`<h2>Poniedziałek 23.11.2026</h2>`,
		`<a class="button-small search-preview-week" href="/?week=2026-11-23#d-2026-11-23">`,
		`Otwórz tydzień<svg`,
		`<li class="search-preview-row">
						<b>6:30</b>
						<span class="search-preview-status is-free">wolna</span>`,
		`<li class="search-preview-row is-dim">
						<b>18:00</b>
						<span class="search-preview-status is-one">1 intencja</span>
						<span class="search-preview-note"><i>(&#43;1 poza parafią)</i> </span>`,
		`href="/?day=2026-11-23&amp;new=1&amp;time=18%3A00&amp;week=2026-11-23&amp;from=search" aria-label="Dodaj intencję na godz. 18:00">+ Dodaj</a>`,
		`<a class="scal-cell has-tip is-match is-selected" href="/search?day=2026-11-23&amp;`,
	)

	body = getSearch(t, h, "/search?period=m1&n=3&view=calendar&day=2026-11-23")
	checkBody(t, body, `<h2>Poniedziałek 23.11.2026</h2>`)

	body = getSearch(t, h, "/search?period=m1&n=3&view=calendar")
	if strings.Contains(body, `id="podglad"`) {
		t.Error("a day preview is shown without matches and without a chosen day")
	}
}

func TestSearchFiltersResetChosenDay(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m1&n=0&planned=1&view=calendar&day=2026-11-23")

	if strings.Contains(body, `aria-label="Usuń filtr: Wolne"`) && strings.Contains(body, `href="/search?day=2026-11-23&amp;period=m1&amp;planned=1&amp;view=calendar`) {
		t.Error("removing a filter keeps the chosen day")
	}
	checkBody(t, body, `<a class="search-filter-chip" href="/search?period=m1&amp;planned=1&amp;schedule=1&amp;view=calendar&amp;week=2026-11-23" aria-label="Usuń filtr: Wolne">`)
}
