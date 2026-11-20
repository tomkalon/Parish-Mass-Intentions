package web

import (
	"fmt"
	"log"
	"maps"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"go-parafia/internal/intentions"
)

const fromSearch = "search"

type searchMemory struct {
	mu    sync.Mutex
	query url.Values
}

func (m *searchMemory) remember(q searchQuery) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.query = q.values()
}

func (m *searchMemory) url(week string) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.query == nil {
		return "/search?week=" + week, false
	}
	query := maps.Clone(m.query)
	query.Set("week", week)

	return "/search?" + query.Encode(), true
}

func searchMassURL(week, date, massTime string) string {
	return newIntentionURL(week, date, massTime) + "&from=" + fromSearch
}

func (h *handler) openedFromSearch(f *formView) {
	f.FromSearch = true
	f.CloseURL, _ = h.searches.url(f.Week)
}

func (h *handler) backToSearchURL(week string, saved int) string {
	searchURL, _ := h.searches.url(week)

	return searchURL + "&saved=" + strconv.Itoa(saved)
}

type searchSaved struct {
	Label   string
	WeekURL string
}

func (h *handler) searchSaved(param string) *searchSaved {
	id, err := strconv.Atoi(param)
	if err != nil || id <= 0 {
		return nil
	}

	found, err := intentions.ByIDs(h.db, []int{id})
	if err != nil {
		log.Printf("error loading saved intention %d: %v", id, err)
		return nil
	}
	intention, ok := found[id]
	if !ok {
		return nil
	}

	at := intention.MassAt.In(intentions.TimeZone)
	week := intentions.WeekStart(at).Format(intentions.DateLayout)

	return &searchSaved{
		Label:   strings.ToLower(weekdayNames[at.Weekday()]) + " " + at.Format("2.1.2006") + ", godz. " + clockLabel(at),
		WeekURL: fmt.Sprintf("/?week=%s&saved=%d#i-%d", week, intention.ID, intention.ID),
	}
}
