package web

import (
	"maps"
	"slices"
	"strconv"
	"time"

	"go-parafia/internal/intentions"
)

type hiddenField struct {
	Name  string
	Value string
}

type textHitSection struct {
	Title string
	Count string
	Past  bool
	Hits  []textHit
}

type textHit struct {
	Date  string
	Time  string
	URL   string
	Parts []textPart

	IsOutside   bool
	IsGregorian bool
	IsDeceased  bool
	IsHidden    bool
	SeriesStep  int
	SeriesURL   string
}

type textPart struct {
	Text string
	Mark bool
}

func textFields(q searchQuery) []hiddenField {
	values := q.values()
	var fields []hiddenField
	for _, name := range slices.Sorted(maps.Keys(values)) {
		if name == "q" || name == "show" || name == "day" {
			continue
		}
		for _, value := range values[name] {
			fields = append(fields, hiddenField{Name: name, Value: value})
		}
	}

	return fields
}

func splitTextHits(hits []intentions.Hit, now time.Time) (upcoming, past []intentions.Hit) {
	split := len(hits)
	for i, hit := range hits {
		if !hit.MassAt.Before(now) {
			split = i
			break
		}
	}
	past = slices.Clone(hits[:split])
	slices.Reverse(past)

	return hits[split:], past
}

func shownTextHits(show int, upcoming, past []intentions.Hit) []int {
	ids := make([]int, 0, min(show, len(upcoming)+len(past)))
	for _, part := range [][]intentions.Hit{upcoming, past} {
		for _, hit := range part {
			if len(ids) == show {
				return ids
			}
			ids = append(ids, hit.ID)
		}
	}

	return ids
}

func addTextSearch(view *searchView, q searchQuery, upcoming, past []intentions.Hit, loaded map[int]intentions.Intention) {
	total := len(upcoming) + len(past)
	view.TextMode = true
	view.Empty = total == 0
	view.Total = "Znaleziono " + countOf(total, "intencję", "intencje", "intencji")
	view.Subtotal = "z tekstem „" + q.Text + "” · w całej bazie, także w Mszach, które już były"

	left := q.Show
	add := func(title string, part []intentions.Hit, isPast bool) {
		if len(part) == 0 || left == 0 {
			return
		}
		section := textHitSection{Title: title, Count: countOf(len(part), "intencja", "intencje", "intencji"), Past: isPast}
		shown := part[:min(len(part), left)]
		left -= len(shown)
		for _, hit := range shown {
			if intention, ok := loaded[hit.ID]; ok {
				section.Hits = append(section.Hits, newTextHit(intention, q.Text))
			}
		}
		if len(section.Hits) > 0 {
			view.HitSections = append(view.HitSections, section)
		}
	}
	add("Nadchodzące", upcoming, false)
	add("Wcześniejsze – Msze, które już były", past, true)

	if rest := total - q.Show; rest > 0 {
		view.MoreURL = q.url(func(next *searchQuery) {
			next.Show = min(q.Show+searchPageDays, maxSearchDays)
		})
		view.More = "Pokaż kolejne intencje (" + strconv.Itoa(rest) + ")"
		if q.Show >= maxSearchDays {
			view.MoreURL = ""
			view.More = "Pokazano pierwsze " + strconv.Itoa(maxSearchDays) + " intencji – wpisz dłuższy fragment, aby zawęzić wyniki (" + strconv.Itoa(rest) + ")."
		}
	}
}

func newTextHit(intention intentions.Intention, query string) textHit {
	at := intention.MassAt.In(intentions.TimeZone)
	week := intentions.WeekStart(at).Format(intentions.DateLayout)

	hit := textHit{
		Date:        weekdayShortNames[at.Weekday()] + " " + at.Format("2.01.2006"),
		Time:        clockLabel(at),
		URL:         editURL(week, intention.ID),
		IsOutside:   intention.IsOutsideParish,
		IsGregorian: intention.IsGregorian,
		IsDeceased:  intention.IsDeceased,
		IsHidden:    intention.IsHidden,
	}

	content := intention.Content
	last := 0
	for _, r := range intentions.Find(content, query) {
		if r[0] > last {
			hit.Parts = append(hit.Parts, textPart{Text: content[last:r[0]]})
		}
		hit.Parts = append(hit.Parts, textPart{Text: content[r[0]:r[1]], Mark: true})
		last = r[1]
	}
	if last < len(content) {
		hit.Parts = append(hit.Parts, textPart{Text: content[last:]})
	}

	return hit
}
