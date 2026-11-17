package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

var searchNow = time.Date(2026, 11, 17, 10, 0, 0, 0, intentions.TimeZone)

var searchSchedules = []masses.Schedule{{ID: 1, Name: "Zwykły"}, {ID: 2, Name: "Wakacje"}}

func getSearch(t *testing.T, h *handler, target string) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	h.search(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want %d", target, rec.Code, http.StatusOK)
	}

	return rec.Body.String()
}

func seedSearchData(t *testing.T, h *handler) {
	t.Helper()

	err := masses.MarkLoaded(h.db, mustDay(t, "2026-11-23"), 1)
	if err != nil {
		t.Fatal(err)
	}
	seedSlot(t, h, "2026-11-23 06:30")
	seedIntention(t, h.db, "2026-11-23 18:00", false, "+ Jan Kowalski")
	seedIntention(t, h.db, "2026-11-23 18:00", true, "+ Anna Nowak")
	seedIntention(t, h.db, "2026-11-16 18:00", false, "przeszła")
}

func seedSlot(t *testing.T, h *handler, at string) {
	t.Helper()

	massAt, err := time.ParseInLocation(intentions.MassAtLayout, at, intentions.TimeZone)
	if err != nil {
		t.Fatal(err)
	}
	err = h.db.Create(&intentions.Intention{MassAt: massAt, IsPlaceholder: true}).Error
	if err != nil {
		t.Fatal(err)
	}
}

func mustDay(t *testing.T, text string) time.Time {
	t.Helper()

	day, err := time.ParseInLocation(intentions.DateLayout, text, intentions.TimeZone)
	if err != nil {
		t.Fatal(err)
	}

	return day
}

func TestParseSearchQueryDefaults(t *testing.T) {
	for _, raw := range []string{"", "week=2026-12-02"} {
		query, _ := url.ParseQuery(raw)
		q := parseSearchQuery(query, searchNow, 2, searchSchedules)

		want := searchQuery{
			Week:          "2026-11-23",
			Period:        "m3",
			Counts:        []int{0},
			SkipGregorian: true,
			Planned:       true,
			ScheduleID:    2,
			Show:          searchPageDays,
		}
		if raw != "" {
			want.Week = "2026-11-30"
		}
		if !reflect.DeepEqual(q, want) {
			t.Errorf("parse(%q) = %+v, want %+v", raw, q, want)
		}
	}
}

func TestParseSearchQueryExplicit(t *testing.T) {
	query, _ := url.ParseQuery("period=y1&n=3&n=1&n=9&n=1&dow=0&dow=7&time=18:00&time=6:30&time=zła&schedule=99&show=5000")

	q := parseSearchQuery(query, searchNow, 0, searchSchedules)

	want := searchQuery{
		Week:       "2026-11-23",
		Period:     "y1",
		Counts:     []int{1, 3},
		Weekdays:   []int{0},
		Times:      []string{"06:30", "18:00"},
		ScheduleID: 1,
		Show:       maxSearchDays,
	}
	if !reflect.DeepEqual(q, want) {
		t.Errorf("parse = %+v, want %+v", q, want)
	}

	again := parseSearchQuery(q.values(), searchNow, 0, searchSchedules)
	if !reflect.DeepEqual(again, q) {
		t.Errorf("values() does not round-trip: %+v", again)
	}
}

func TestParseSearchQueryUnknownPeriodAndNoSchedules(t *testing.T) {
	query, _ := url.ParseQuery("period=forever&planned=1")

	q := parseSearchQuery(query, searchNow, 0, nil)

	if q.Period != defaultSearchPeriod || q.ScheduleID != 0 || !q.Planned || q.SkipGregorian || len(q.Counts) != 0 {
		t.Errorf("parse = %+v", q)
	}
}

func TestSearchDefaultsShowFreeMasses(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search")

	checkBody(t, body,
		`<title>Szukaj Mszy – Go Parafia</title>`,
		`<a class="button" href="/?week=2026-11-23" data-close>← Wróć do tygodnia</a>`,
		`<p class="search-total">Znaleziono 49 Mszy</p>`,
		`<p class="search-subtotal">w 25 dniach · kliknij Mszę, aby dodać intencję</p>`,
		`<p class="facet-hint">od 17.11.2026 do 16.2.2027</p>`,
		`name="n" value="0" checked data-autosubmit><span class="facet-swatch is-free" aria-hidden="true"></span><span class="facet-label">Wolna – bez intencji</span><span class="facet-count">49</span>`,
		`name="n" value="1" data-autosubmit><span class="facet-swatch is-one" aria-hidden="true"></span><span class="facet-label">1 intencja</span><span class="facet-count">1</span>`,
		`<label class="facet-row is-empty"><input type="checkbox" id="f-n-2"`,
		`id="f-skipgreg" name="skipgreg" value="1" checked`,
		`id="f-planned" name="planned" value="1" checked`,
		`<option value="1" selected>Porządek zwykły</option>`,
		`<a class="search-filter-chip" href="/search?period=m3&amp;planned=1&amp;schedule=1&amp;skipgreg=1&amp;week=2026-11-23" aria-label="Usuń filtr: Wolne">Wolne`,
		`<h2>Listopad 2026</h2>`,
		`<a href="/?week=2026-11-16#d-2026-11-22">Nd 22.11</a>`,
		`<a class="mass-chip is-planned" href="/?day=2026-11-22&amp;new=1&amp;time=07%3A00&amp;week=2026-11-16" title="Dodaj intencję: niedziela 22.11, godz. 7:00"><b>7:00</b><span>planowana</span></a>`,
		`<span class="search-day-planned">godziny wg porządku „Porządek zwykły”</span>`,
		`<a class="mass-chip is-free" href="/?day=2026-11-23&amp;new=1&amp;time=06%3A30&amp;week=2026-11-23" title="Dodaj intencję: poniedziałek 23.11, godz. 6:30"><b>6:30</b><span>wolna</span></a>`,
		`<span>2 Msze</span><span aria-hidden="true">·</span><span>1 intencja</span><span aria-hidden="true">·</span>
	<span class="day-free">1 wolna</span>
	<i class="day-outside">(&#43;1 poza parafią)</i>`,
	)
	if strings.Contains(body, "<b>18:00</b><span>1 intencja</span>") {
		t.Error("a Mass with an intention matched the default free filter")
	}
	if strings.Contains(body, "16.11") {
		t.Error("past Masses are listed")
	}
}

func TestSearchFiltersByCountWeekdayAndTime(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m1&n=1&planned=1&skipgreg=1&schedule=1")
	checkBody(t, body,
		`<p class="search-total">Znaleziono 1 Mszę</p>`,
		`<p class="search-subtotal">w 1 dniu`,
		`<a class="mass-chip is-one" href="/?day=2026-11-23&amp;new=1&amp;time=18%3A00&amp;week=2026-11-23" title="Dodaj intencję: poniedziałek 23.11, godz. 18:00"><b>18:00</b><span>1 intencja</span><i>(&#43;1 poza parafią)</i></a>`,
	)

	body = getSearch(t, h, "/search?period=m1&n=0&dow=0&time=10:00&planned=1&schedule=1")
	checkBody(t, body,
		`<p class="search-total">Znaleziono 3 Msze</p>`,
		`id="f-dow-0" name="dow" value="0" checked data-autosubmit><span class="facet-label">Niedz.</span><span class="facet-count">3</span>`,
		`id="f-dow-1" name="dow" value="1" data-autosubmit><span class="facet-label">Pon.</span><span class="facet-count">0</span>`,
		`id="f-time-1000" name="time" value="10:00" checked data-autosubmit><span class="facet-label">10:00</span><span class="facet-count">3</span>`,
		`id="f-time-0700" name="time" value="07:00" data-autosubmit><span class="facet-label">7:00</span><span class="facet-count">3</span>`,
		`aria-label="Usuń filtr: Niedziela"`,
		`aria-label="Usuń filtr: godz. 10:00"`,
	)
}

func TestSearchWithoutPlannedMasses(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)

	body := getSearch(t, h, "/search?period=m3&n=0&schedule=1")

	checkBody(t, body, `<p class="search-total">Znaleziono 1 Mszę</p>`)
	if strings.Contains(body, "planowana</span>") {
		t.Error("planned Masses listed although the option is off")
	}
}

func TestSearchPlannedMassesFollowChosenSchedule(t *testing.T) {
	h := newTestHandler(t, searchNow)

	body := getSearch(t, h, "/search?period=m1&planned=1&schedule=2")

	checkBody(t, body,
		`<option value="2" selected>Wakacje</option>`,
		`<b>9:00</b><span>planowana</span>`,
		`godziny wg porządku „Wakacje”`,
	)
	if strings.Contains(body, "<b>6:30</b>") {
		t.Error("hours of another schedule are listed")
	}
}

func TestSearchSkipsGregorianMasses(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSearchData(t, h)
	err := h.db.Model(&intentions.Intention{}).Where("content = ?", "+ Jan Kowalski").Update("is_gregorian", true).Error
	if err != nil {
		t.Fatal(err)
	}

	body := getSearch(t, h, "/search?period=m1&n=1&skipgreg=1")
	checkBody(t, body, `<p class="search-total">Znaleziono 0 Mszy</p>`, `Brak Mszy spełniających warunki`)

	body = getSearch(t, h, "/search?period=m1&n=1")
	checkBody(t, body, `<span>1 intencja · gregorianka</span>`)
}

func TestSearchPagesByDays(t *testing.T) {
	h := newTestHandler(t, searchNow)

	body := getSearch(t, h, "/search?period=y1&planned=1&schedule=1")
	checkBody(t, body,
		`<a class="button search-more" href="/search?period=y1&amp;planned=1&amp;schedule=1&amp;show=60&amp;week=2026-11-23">Pokaż kolejne dni (`,
	)
	if got := strings.Count(body, `<div class="search-day-date">`); got != searchPageDays {
		t.Errorf("listed %d days, want %d", got, searchPageDays)
	}

	body = getSearch(t, h, "/search?period=y1&planned=1&schedule=1&show=60")
	if got := strings.Count(body, `<div class="search-day-date">`); got != 2*searchPageDays {
		t.Errorf("listed %d days with show=60, want %d", got, 2*searchPageDays)
	}
}

func TestSearchWithoutSchedules(t *testing.T) {
	h := newTestHandler(t, searchNow)
	err := h.db.Exec("DELETE FROM schedule_masses").Error
	if err == nil {
		err = h.db.Exec("DELETE FROM schedules").Error
	}
	if err != nil {
		t.Fatal(err)
	}

	body := getSearch(t, h, "/search")

	checkBody(t, body, `<p class="search-total">Znaleziono 0 Mszy</p>`, `<p class="facet-hint">Brak Mszy w tym okresie.</p>`)
	if strings.Contains(body, `id="f-schedule"`) {
		t.Error("schedule select shown without schedules")
	}
}

func TestSearchDatabaseError(t *testing.T) {
	h := newTestHandler(t, searchNow)
	err := h.db.Exec("DROP TABLE intentions").Error
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/search", nil)
	rec := httptest.NewRecorder()
	h.search(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestWeekLinksToSearch(t *testing.T) {
	h := newTestHandler(t, searchNow)

	rec := getWeek(t, h, "/?week=2026-11-30")

	checkBody(t, rec.Body.String(), `<a class="button toolbar-search" href="/search?week=2026-11-30" title="Szukaj Mszy">`)
}

func TestWeekDayHeaderShowsDayStats(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSlot(t, h, "2026-12-01 06:30")
	seedIntention(t, h.db, "2026-12-01 18:00", false, "+ Jan Kowalski")
	seedIntention(t, h.db, "2026-12-01 18:00", false, "Za parafian")
	seedIntention(t, h.db, "2026-12-01 18:00", true, "+ Anna Nowak")
	seedIntention(t, h.db, "2026-12-02 18:00", false, "+ Piotr")

	body := getWeek(t, h, "/?week=2026-11-30").Body.String()

	checkBody(t, body,
		`<span>2 Msze</span><span aria-hidden="true">·</span><span>2 intencje</span><span aria-hidden="true">·</span>
	<span class="day-free">1 wolna</span>
	<i class="day-outside">(&#43;1 poza parafią)</i>`,
		`<span>1 Msza</span><span aria-hidden="true">·</span><span>1 intencja</span><span aria-hidden="true">·</span>
	<span class="day-full">wszystkie zajęte</span>`,
	)
	if got := strings.Count(body, `<span class="day-stats">`); got != 2 {
		t.Errorf("day summaries = %d, want 2 (days without Masses have none)", got)
	}
}

func TestCreateSlotsMarksWeekAndClearUnmarks(t *testing.T) {
	h := newTestHandler(t, searchNow)
	countLoaded := func() int64 {
		var n int64
		err := h.db.Table("loaded_weeks").Where("week_start = ?", "2026-11-30").Count(&n).Error
		if err != nil {
			t.Fatal(err)
		}
		return n
	}

	postSlots(t, h, url.Values{"week": {"2026-12-02"}, "schedule": {"1"}})
	if countLoaded() != 1 {
		t.Fatal("loading the hours did not mark the week")
	}

	body := getSearch(t, h, "/search?period=m1&n=0&schedule=1&planned=1")
	if strings.Contains(body, `<a href="/?week=2026-11-30#d-2026-11-30">`) && strings.Contains(body, "Pn 30.11</a>") {
		checkBody(t, body, `href="/?day=2026-11-30&amp;new=1&amp;time=06%3A30&amp;week=2026-11-30" title="Dodaj intencję: poniedziałek 30.11, godz. 6:30"><b>6:30</b><span>wolna</span>`)
	} else {
		t.Error("loaded empty slot on 30.11 not listed as free")
	}

	postClear(t, h, url.Values{"week": {"2026-11-30"}})
	if countLoaded() != 0 {
		t.Error("clearing the week did not remove the mark")
	}
}
