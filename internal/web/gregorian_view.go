package web

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

const (
	seriesActive = "active"
	seriesClosed = "done"

	closedSeriesLimit = 50
)

var errInvalidGregorian = errors.New("invalid gregorian query")

type gregorianQuery struct {
	tab  string
	open int
	stop bool
}

func parseGregorianQuery(query url.Values) (gregorianQuery, error) {
	q := gregorianQuery{tab: query.Get("gregorian"), stop: query.Get("stop") == "1"}
	if q.tab != seriesActive && q.tab != seriesClosed {
		return gregorianQuery{}, errInvalidGregorian
	}
	if param := query.Get("series"); param != "" {
		id, err := strconv.Atoi(param)
		if err != nil || id <= 0 {
			return gregorianQuery{}, errInvalidGregorian
		}
		q.open = id
	}
	if q.stop && q.open == 0 {
		return gregorianQuery{}, errInvalidGregorian
	}

	return q, nil
}

func seriesIDs(list []intentions.Intention) []int {
	var ids []int
	for _, intention := range list {
		if intention.SeriesID > 0 && !slices.Contains(ids, intention.SeriesID) {
			ids = append(ids, intention.SeriesID)
		}
	}

	return ids
}

func setSeriesSteps(days []dayView, steps map[int]int) {
	forEachIntention(days, func(intention *intentionView) {
		if intention.SeriesID > 0 {
			intention.SeriesStep = steps[intention.ID]
		}
	})
}

func shortDate(day time.Time) string {
	return day.Format("2.1")
}

func gregorianURL(week, tab string, open int) string {
	target := "/?week=" + week + "&gregorian=" + tab
	if open > 0 {
		target += "&series=" + strconv.Itoa(open)
	}

	return target
}

func seriesLink(week string, id int) string {
	if id == 0 {
		return ""
	}

	return gregorianURL(week, seriesActive, id) + "#s-" + strconv.Itoa(id)
}

func seriesIntentionURL(week, day, massTime string, id int) string {
	return newIntentionURL(week, day, massTime) + "&series=" + strconv.Itoa(id)
}

type seriesDue struct {
	Name string
	Step int
	URL  string
}

func markSeriesDue(days []dayView, active []gregorian.Summary, week string) {
	for _, s := range active {
		if s.NextDay.IsZero() {
			continue
		}
		date := s.NextDay.Format(intentions.DateLayout)
		for i := range days {
			if days[i].Date != date {
				continue
			}
			days[i].SeriesDue = append(days[i].SeriesDue, seriesDue{
				Name: s.Series.Content,
				Step: s.Count() + 1,
				URL:  seriesIntentionURL(week, date, s.NextTime(), s.Series.ID),
			})
		}
	}
}

type seriesNotice struct {
	Name string
	Text string
	URL  string
}

func seriesNotices(active []gregorian.Summary, start time.Time) []seriesNotice {
	week := start.Format(intentions.DateLayout)
	end := start.AddDate(0, 0, 7)

	var notices []seriesNotice
	for _, s := range active {
		if len(s.Gaps) == 0 || !s.First().Before(end) || (!s.End.IsZero() && s.End.Before(start)) {
			continue
		}

		var gaps []string
		for _, gap := range s.Gaps {
			gaps = append(gaps, shortDate(gap))
		}
		text := "ma przerwę " + gaps[0] + " (po " + strconv.Itoa(s.GapsAfter[0]) + "/30)"
		if len(gaps) > 1 {
			text = "ma przerwy: " + strings.Join(gaps, ", ")
		}
		if !s.End.IsZero() {
			text += " – zakończy się ok. " + shortDate(s.End)
		}

		notices = append(notices, seriesNotice{
			Name: s.Series.Content,
			Text: text + ".",
			URL:  gregorianURL(week, seriesActive, s.Series.ID),
		})
	}

	return notices
}

type seriesOption struct {
	ID       int
	Name     string
	Step     int
	Meta     string
	MetaKind string
	Disabled bool
	Hidden   bool
	Checked  bool

	Content string
	Time    string
	Outside bool
	Paid    string
	Days    string
	Next    string
	First   string
}

func newSeriesOptions(active []gregorian.Summary, day string, selected int) []seriesOption {
	options := make([]seriesOption, 0, len(active))
	for _, s := range active {
		option := seriesOption{
			ID:      s.Series.ID,
			Name:    s.Series.Content,
			Content: s.Series.Content,
			Time:    s.NextTime(),
			Outside: s.Series.IsOutsideParish,
			Paid:    paidLabel(s),
			Days:    strings.Join(s.Dates(), ","),
			First:   s.First().Format(intentions.DateLayout),
		}
		if !s.NextDay.IsZero() {
			option.Next = s.NextDay.Format(intentions.DateLayout)
		}
		option.Step, option.Meta, option.MetaKind, option.Disabled, option.Hidden = seriesOptionState(s, day)
		option.Checked = selected == s.Series.ID && !option.Disabled && !option.Hidden
		options = append(options, option)
	}

	return options
}

func seriesOptionState(s gregorian.Summary, day string) (step int, meta, kind string, disabled, hidden bool) {
	next := ""
	if !s.NextDay.IsZero() {
		next = s.NextDay.Format(intentions.DateLayout)
	}
	first := s.First().Format(intentions.DateLayout)
	step = s.StepOn(day)

	switch {
	case day == "":
		if next == "" {
			return s.Count(), "ma już 30 Mszy", "", true, false
		}
		return s.Count() + 1, "następna: " + shortDate(s.NextDay) + " · " + clockText(s.NextTime()), "", false, false
	case s.HasDay(day):
		return step, "już dodana na ten dzień", "", true, false
	case day < first:
		return 0, "", "", true, true
	case next == "":
		return s.Count(), "ma już 30 Mszy", "", true, false
	case day == next:
		return step, "na ten dzień · " + clockText(s.NextTime()), "due", false, false
	case day > next:
		return step, "zaległa od " + shortDate(s.NextDay), "late", false, false
	default:
		return step, "uzupełnia przerwę w serii", "gap", false, false
	}
}

func clockText(value string) string {
	t, err := time.Parse("15:04", value)
	if err != nil {
		return value
	}

	return clockLabel(t)
}

func paidLabel(s gregorian.Summary) string {
	if amount := s.Amount(); amount > 0 {
		return amount.String()
	}

	return ""
}

type gregorianView struct {
	Tab         string
	ActiveURL   string
	ClosedURL   string
	CloseURL    string
	ActiveCount int
	ClosedCount int64
	Rows        []seriesRow
	Stop        *seriesStop
	Week        string
}

type seriesStop struct {
	ID       int
	Name     string
	Done     int
	Count    int
	CloseURL string
}

type seriesRow struct {
	ID        int
	Name      string
	Sub       string
	Done      int
	Count     int
	Segments  []string
	Plan      string
	Next      string
	Gap       string
	AddURL    string
	AddLabel  string
	Status    string
	IsStopped bool
	IsOpen    bool
	ToggleURL string
	Days      []seriesDay
	Paid      string
	EditURL   string
	StopURL   string
}

type seriesDay struct {
	Step  int
	Date  string
	Class string
	Title string
	URL   string
}

func (r seriesRow) BarLabel() string {
	return fmt.Sprintf("%d odprawionych, %d zaplanowanych, %d do dodania", r.Done, r.Count-r.Done, gregorian.Length-r.Count)
}

func newGregorianView(q gregorianQuery, current, now time.Time, active, closed []gregorian.Summary, closedCount int64) gregorianView {
	week := current.Format(intentions.DateLayout)
	view := gregorianView{
		Tab:         q.tab,
		ActiveURL:   gregorianURL(week, seriesActive, 0),
		ClosedURL:   gregorianURL(week, seriesClosed, 0),
		CloseURL:    weekURL(current),
		ActiveCount: len(active),
		ClosedCount: closedCount,
		Week:        week,
	}

	if q.open > 0 && q.tab == seriesActive && !containsSeries(active, q.open) && containsSeries(closed, q.open) {
		q.tab = seriesClosed
		view.Tab = seriesClosed
	}

	list := active
	if q.tab == seriesClosed {
		list = closed
	}
	for _, s := range list {
		view.Rows = append(view.Rows, newSeriesRow(s, q, week, now))
	}

	if q.stop {
		for _, s := range active {
			if s.Series.ID == q.open {
				view.Stop = &seriesStop{
					ID:       s.Series.ID,
					Name:     s.Series.Content,
					Done:     s.Done,
					Count:    s.Count(),
					CloseURL: gregorianURL(week, q.tab, q.open),
				}
			}
		}
	}

	return view
}

func newSeriesRow(s gregorian.Summary, q gregorianQuery, week string, now time.Time) seriesRow {
	row := seriesRow{
		ID:    s.Series.ID,
		Name:  s.Series.Content,
		Done:  s.Done,
		Count: s.Count(),
		Paid:  paidLabel(s),
	}
	row.IsOpen = q.open == s.Series.ID
	row.ToggleURL = gregorianURL(week, q.tab, s.Series.ID)
	if row.IsOpen {
		row.ToggleURL = gregorianURL(week, q.tab, 0)
	}

	sub := "od " + s.First().Format("2.1.2006")
	if s.Status != gregorian.StatusActive {
		sub = s.First().Format("2.1") + " – " + s.Last().Format("2.1.2006")
	} else if !s.NextDay.IsZero() {
		sub += " · zwykle " + clockText(s.NextTime())
	}
	if s.Series.IsOutsideParish {
		sub += " · poza parafią"
	}
	if row.Paid != "" {
		sub += " · ofiara " + row.Paid
	} else {
		sub += " · ofiara nie wpisana"
	}
	row.Sub = sub

	for i := 1; i <= gregorian.Length; i++ {
		class := "seg todo"
		switch {
		case i <= s.Done:
			class = "seg done"
		case i <= s.Count():
			class = "seg planned"
		}
		if slices.Contains(s.GapsAfter, i-1) {
			class += " gap-before"
		}
		row.Segments = append(row.Segments, class)
	}

	switch s.Status {
	case gregorian.StatusActive:
		row.Plan = fmt.Sprintf("%d zapisanych · koniec ok. %s", s.Count(), shortDate(s.End))
		if !s.NextDay.IsZero() {
			step := s.Count() + 1
			row.Next = fmt.Sprintf("%s %s (%d/30)", weekdayShortNames[s.NextDay.Weekday()], shortDate(s.NextDay), step)
			row.AddURL = seriesIntentionURL(intentions.WeekStart(s.NextDay).Format(intentions.DateLayout), s.NextDay.Format(intentions.DateLayout), s.NextTime(), s.Series.ID)
			row.AddLabel = fmt.Sprintf("Dodaj %d/30", step)
		} else {
			row.Plan = "Wszystkie 30 Mszy zapisane · koniec " + shortDate(s.End)
		}
		if len(s.Gaps) > 0 {
			row.Gap = fmt.Sprintf("Przerwa %s (po %d/30) – seria się wydłuża", shortDate(s.Gaps[0]), s.GapsAfter[0])
			if len(s.Gaps) > 1 {
				row.Gap = fmt.Sprintf("Przerwy w serii: %d – seria się wydłuża", len(s.Gaps))
			}
		}
		row.StopURL = gregorianURL(week, q.tab, s.Series.ID) + "&stop=1"
	case gregorian.StatusFinished:
		row.Status = "zakończona " + shortDate(s.Last())
		row.Plan = "Odprawiono wszystkie 30 Mszy."
	case gregorian.StatusStopped:
		row.Status = fmt.Sprintf("przerwana po %d/30", s.Count())
		row.IsStopped = true
		row.Plan = fmt.Sprintf("Odprawiono %d z 30.", s.Done)
	}

	if first := s.FirstID(); first > 0 {
		row.EditURL = editURL(intentions.WeekStart(s.First()).Format(intentions.DateLayout), first)
	}

	today := now.In(intentions.TimeZone).Format(intentions.DateLayout)
	for i, m := range s.Masses {
		if i >= gregorian.Length {
			break
		}
		class := "series-day planned"
		what := "zaplanowana"
		if m.MassAt.Before(now) {
			class, what = "series-day done", "odprawiona"
		}
		if m.Date() == today {
			class += " is-today"
		}
		row.Days = append(row.Days, seriesDay{
			Step:  i + 1,
			Date:  shortDate(m.MassAt),
			Class: class,
			Title: fmt.Sprintf("%d/30 · %s %s · %s", i+1, shortDate(m.MassAt), clockLabel(m.MassAt), what),
			URL:   weekURL(intentions.WeekStart(m.MassAt)) + "#i-" + strconv.Itoa(m.ID),
		})
	}
	if s.Status == gregorian.StatusActive {
		for i, day := range s.Planned() {
			step := s.Count() + i + 1
			row.Days = append(row.Days, seriesDay{
				Step:  step,
				Date:  shortDate(day),
				Class: "series-day todo",
				Title: fmt.Sprintf("%d/30 · %s · do dodania (przewidywana data)", step, shortDate(day)),
			})
		}
	}

	return row
}

func (v weekView) GregorianURL() string {
	return gregorianURL(v.Week(), seriesActive, 0)
}

func containsSeries(list []gregorian.Summary, id int) bool {
	for _, s := range list {
		if s.Series.ID == id {
			return true
		}
	}

	return false
}
