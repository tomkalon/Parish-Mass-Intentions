package web

import (
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

var searchLinkPattern = regexp.MustCompile(`<a class="button toolbar-search" href="([^"]*)" title="([^"]*)">`)

func weekSearchLink(t *testing.T, h *handler, target string) (url.Values, string) {
	t.Helper()

	body := getWeek(t, h, target).Body.String()
	match := searchLinkPattern.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("GET %s: search link not found", target)
	}
	link, err := url.Parse(html.UnescapeString(match[1]))
	if err != nil {
		t.Fatal(err)
	}
	if link.Path != "/search" {
		t.Fatalf("search link path = %q, want /search", link.Path)
	}

	return link.Query(), match[2]
}

func TestWeekSearchLinkReturnsToLastSearch(t *testing.T) {
	h := newTestHandler(t, searchNow)

	query, title := weekSearchLink(t, h, "/?week=2026-11-30")
	if want := (url.Values{"week": {"2026-11-30"}}); query.Encode() != want.Encode() || title != "Szukaj Mszy" {
		t.Errorf("before any search: link %v, title %q; want %v, %q", query, title, want, "Szukaj Mszy")
	}

	getSearch(t, h, "/search?week=2026-11-30&period=m1&n=1&view=calendar&day=2026-12-01&show=60")

	query, title = weekSearchLink(t, h, "/?week=2026-12-07")
	if title != "Wróć do wyszukiwania" {
		t.Errorf("title = %q, want %q", title, "Wróć do wyszukiwania")
	}
	for key, want := range map[string]string{"week": "2026-12-07", "period": "m1", "n": "1", "view": "calendar", "day": "2026-12-01"} {
		if got := query.Get(key); got != want {
			t.Errorf("remembered %s = %q, want %q (link %v)", key, got, want, query)
		}
	}
}

func TestSearchDefaultsLink(t *testing.T) {
	h := newTestHandler(t, searchNow)

	if body := getSearch(t, h, "/search?week=2026-11-30"); strings.Contains(body, "search-defaults") {
		t.Error("default search shows the reset link")
	}

	body := getSearch(t, h, "/search?week=2026-11-30&period=m1")
	checkBody(t, body, `<a class="button button-primary search-defaults" href="/search?week=2026-11-30"`)
}

func TestSearchMassOpensPanelThatReturnsToSearch(t *testing.T) {
	h := newTestHandler(t, searchNow)
	seedSlot(t, h, "2026-12-01 18:00")

	body := getSearch(t, h, "/search?week=2026-11-30&period=m1")
	checkBody(t, body, `href="/?day=2026-12-01&amp;new=1&amp;time=18%3A00&amp;week=2026-11-30&amp;from=search"`)

	panel := getWeek(t, h, "/?day=2026-12-01&new=1&time=18%3A00&week=2026-11-30&from=search").Body.String()
	back, _ := h.searches.url("2026-11-30")
	checkBody(t, panel,
		`<input type="hidden" name="from" value="search">`,
		`Z wyszukiwania – po zapisaniu wrócisz do wyników.`,
		`<a class="button" href="`+html.EscapeString(back)+`" data-discard>Anuluj</a>`,
		`<div class="form-from-search">
				<p>Z wyszukiwania – po zapisaniu wrócisz do wyników.</p>
				<a class="button button-primary button-return" href="`+html.EscapeString(back)+`" title="Wróć do wyników wyszukiwania bez zapisywania" data-discard>`,
		`<div class="toolbar-return">
			<a class="button button-primary button-return" href="`+html.EscapeString(back)+`" title="Wróć do wyników wyszukiwania bez zapisywania" data-discard>`,
	)
	toolbarEnd := strings.Index(panel, "</nav>")
	if i := strings.Index(panel, "toolbar-return"); i < strings.Index(panel, `aria-label="Ustawienia"`) || i > toolbarEnd {
		t.Error("the way back to search is not on its own line at the end of the toolbar")
	}

	if plain := getWeek(t, h, "/?day=2026-12-01&new=1&time=18%3A00&week=2026-11-30").Body.String(); strings.Contains(plain, `name="from"`) || strings.Contains(plain, "toolbar-return") {
		t.Error("panel opened from the week offers a way back to search")
	}
}

func TestSaveFromSearchReturnsToSearch(t *testing.T) {
	h := newTestHandler(t, searchNow)
	getSearch(t, h, "/search?week=2026-11-23&period=m1&view=calendar&day=2026-12-01")

	form := url.Values{
		"week":    {"2026-11-30"},
		"day":     {"2026-12-01"},
		"time":    {"18:00"},
		"content": {"+ Jan Kowalski"},
		"from":    {"search"},
	}
	rec := postIntention(t, h, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	location, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	query := location.Query()
	if location.Path != "/search" || query.Get("saved") != "1" || query.Get("day") != "2026-12-01" || query.Get("week") != "2026-11-30" {
		t.Fatalf("Location = %q, want the remembered search for week 2026-11-30 with saved=1", location)
	}

	body := getSearch(t, h, location.String())
	checkBody(t, body,
		`<span>Zapisano intencję: wtorek 1.12.2026, godz. 18:00.</span>`,
		`<a href="/?week=2026-11-30&amp;saved=1#i-1">Zobacz w tygodniu</a>`,
	)

	if again, _ := weekSearchLink(t, h, "/?week=2026-11-30"); again.Get("saved") != "" {
		t.Error("the saved notice became part of the remembered search")
	}
}

func TestInvalidFormFromSearchKeepsTheWayBack(t *testing.T) {
	h := newTestHandler(t, searchNow)

	form := url.Values{
		"week": {"2026-11-30"},
		"day":  {"2026-12-01"},
		"time": {"18:00"},
		"from": {"search"},
	}
	rec := postIntention(t, h, form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	checkBody(t, rec.Body.String(), `<input type="hidden" name="from" value="search">`)
}

func TestSaveFromWeekStaysInWeek(t *testing.T) {
	h := newTestHandler(t, searchNow)
	getSearch(t, h, "/search?week=2026-11-30&period=m1")

	form := url.Values{
		"week":    {"2026-11-30"},
		"day":     {"2026-12-01"},
		"time":    {"18:00"},
		"content": {"+ Jan Kowalski"},
	}
	rec := postIntention(t, h, form)

	if got := rec.Header().Get("Location"); got != "/?week=2026-11-30&saved=1#i-1" {
		t.Errorf("Location = %q, want the week", got)
	}
}
