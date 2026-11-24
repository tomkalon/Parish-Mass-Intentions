package web

import (
	"net/url"
	"strconv"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

const maxBlankLines = 3

type hiddenMode int

const (
	hiddenSkip hiddenMode = iota
	hiddenInclude
	hiddenOnly
)

func parseHiddenMode(value string) hiddenMode {
	switch value {
	case "1":
		return hiddenInclude
	case "only":
		return hiddenOnly
	default:
		return hiddenSkip
	}
}

func (m hiddenMode) param() string {
	switch m {
	case hiddenInclude:
		return "1"
	case hiddenOnly:
		return "only"
	default:
		return ""
	}
}

func (m hiddenMode) prints(hidden bool) bool {
	switch m {
	case hiddenInclude:
		return true
	case hiddenOnly:
		return hidden
	default:
		return !hidden
	}
}

type printOptions struct {
	Hidden     hiddenMode
	Day        time.Time
	Blanks     bool
	Lines      int
	Extra      bool
	Planned    bool
	ScheduleID int
}

func parsePrintOptions(query url.Values, selectedSchedule int, schedules []masses.Schedule, fallback hiddenMode) printOptions {
	hidden := fallback
	if query.Has("hidden") {
		hidden = parseHiddenMode(query.Get("hidden"))
	}

	opts := printOptions{
		Hidden:     hidden,
		Blanks:     query.Get("blanks") == "1",
		Lines:      1,
		Extra:      query.Get("extra") == "1",
		Planned:    true,
		ScheduleID: chooseSchedule(query.Get("schedule"), selectedSchedule, schedules),
	}
	if n, err := strconv.Atoi(query.Get("lines")); err == nil && n >= 1 && n <= maxBlankLines {
		opts.Lines = n
	}
	if query.Get("set") == "1" {
		opts.Planned = query.Get("planned") == "1"
	}
	if opts.Hidden == hiddenOnly {
		opts.Blanks = false
	}

	return opts
}

func parsePrintDay(value string, start time.Time) (time.Time, bool) {
	if value == "" {
		return time.Time{}, true
	}

	day, err := time.ParseInLocation(intentions.DateLayout, value, intentions.TimeZone)
	if err != nil || day.Before(start) || !day.Before(start.AddDate(0, 0, 7)) {
		return time.Time{}, false
	}

	return day, true
}

func (o printOptions) query(week string) string {
	query := "week=" + url.QueryEscape(week)
	if !o.Day.IsZero() {
		query += "&day=" + o.Day.Format(intentions.DateLayout)
	}
	if param := o.Hidden.param(); param != "" {
		query += "&hidden=" + param
	}
	if !o.Blanks {
		return query
	}

	query += "&blanks=1&lines=" + strconv.Itoa(o.Lines) + "&set=1"
	if o.Extra {
		query += "&extra=1"
	}
	if o.Planned {
		query += "&planned=1"
	}
	if o.ScheduleID > 0 {
		query += "&schedule=" + strconv.Itoa(o.ScheduleID)
	}

	return query
}

type printView struct {
	weekView

	HiddenCount int
	Options     printOptions
	Lines       []searchOption
	Schedules   []searchOption
}

func newPrintView(start time.Time, list []intentions.Intention, opts printOptions, schedules []masses.Schedule) printView {
	view := printView{weekView: newWeekView(start, nil), Options: opts, HiddenCount: countHidden(list, opts.Day)}

	for n := 1; n <= maxBlankLines; n++ {
		view.Lines = append(view.Lines, searchOption{Value: strconv.Itoa(n), Label: strconv.Itoa(n), Selected: n == opts.Lines})
	}
	for _, s := range schedules {
		view.Schedules = append(view.Schedules, searchOption{Value: strconv.Itoa(s.ID), Label: s.Name, Selected: s.ID == opts.ScheduleID})
	}

	return view
}

func countHidden(list []intentions.Intention, day time.Time) int {
	count := 0
	for _, intention := range list {
		if intention.IsHidden && !intention.IsPlaceholder && (day.IsZero() || sameDay(intention.MassAt, day)) {
			count++
		}
	}

	return count
}

func rememberedHidden(saved string, hiddenCount int) hiddenMode {
	mode := parseHiddenMode(saved)
	if mode == hiddenOnly && hiddenCount == 0 {
		return hiddenSkip
	}

	return mode
}

func sameDay(at, day time.Time) bool {
	return at.In(intentions.TimeZone).Format(intentions.DateLayout) == day.Format(intentions.DateLayout)
}

func (v printView) PDFURL() string {
	return "/pdf?" + v.Options.query(v.Week())
}

func (v printView) IsDay() bool {
	return !v.Options.Day.IsZero()
}

func (v printView) Day() string {
	if !v.IsDay() {
		return ""
	}

	return v.Options.Day.Format(intentions.DateLayout)
}

func (v printView) Title() string {
	if !v.IsDay() {
		return v.Range
	}

	return v.Options.Day.Format("2.1.2006") + " r."
}

func (v printView) Note() string {
	if !v.IsDay() {
		return v.Range
	}

	return weekdayNames[v.Options.Day.Weekday()] + ", " + v.Options.Day.Format("2.1.2006") + " r."
}

func (v printView) BackURL() string {
	if !v.IsDay() {
		return v.WeekURL()
	}

	return v.WeekURL() + "#d-" + v.Day()
}

func (v printView) HiddenModes() []searchOption {
	modes := []struct {
		mode  hiddenMode
		label string
	}{
		{hiddenSkip, "Bez ukrytych"},
		{hiddenInclude, "Z ukrytymi"},
		{hiddenOnly, "Tylko ukryte"},
	}

	options := make([]searchOption, 0, len(modes))
	for _, m := range modes {
		options = append(options, searchOption{Value: m.mode.param(), Label: m.label, Selected: m.mode == v.Options.Hidden})
	}

	return options
}

func printFileName(start time.Time, opts printOptions) string {
	name := "intencje-" + start.Format(intentions.DateLayout)
	if !opts.Day.IsZero() {
		name = "intencje-" + opts.Day.Format(intentions.DateLayout)
	}
	if opts.Hidden == hiddenOnly {
		name += "-ukryte"
	}
	if opts.Blanks {
		name += "-do-wpisania"
	}

	return name + ".pdf"
}

func (v printView) OnlyHidden() bool {
	return v.Options.Hidden == hiddenOnly
}
