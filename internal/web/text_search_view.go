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

func addTextSearch(view *searchView, q searchQuery, list []intentions.Intention, now time.Time) {
	view.TextMode = true
	view.Empty = len(list) == 0
	view.Total = "Znaleziono " + countOf(len(list), "intencję", "intencje", "intencji")
	view.Subtotal = "z tekstem „" + q.Text + "” · w całej bazie, także w Mszach, które już były"

	split := len(list)
	for i, intention := range list {
		if !intention.MassAt.Before(now) {
			split = i
			break
		}
	}
	past := slices.Clone(list[:split])
	slices.Reverse(past)
	upcoming := list[split:]

	left := q.Show
	add := func(title string, part []intentions.Intention, isPast bool) {
		if len(part) == 0 {
			return
		}
		section := textHitSection{Title: title, Count: countOf(len(part), "intencja", "intencje", "intencji"), Past: isPast}
		for _, intention := range part[:min(len(part), left)] {
			section.Hits = append(section.Hits, newTextHit(intention, q.Text))
		}
		left -= len(section.Hits)
		if len(section.Hits) > 0 {
			view.HitSections = append(view.HitSections, section)
		}
	}
	add("Nadchodzące", upcoming, false)
	add("Wcześniejsze – Msze, które już były", past, true)

	if rest := len(list) - q.Show; rest > 0 {
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
