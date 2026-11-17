package web

import (
	"net/url"
	"strconv"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

const maxBlankLines = 3

type printOptions struct {
	ShowHidden bool
	Blanks     bool
	Lines      int
	Extra      bool
	Planned    bool
	ScheduleID int
}

func parsePrintOptions(query url.Values, selectedSchedule int, schedules []masses.Schedule) printOptions {
	opts := printOptions{
		ShowHidden: query.Get("hidden") == "1",
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

	return opts
}

func (o printOptions) query(week string) string {
	query := "week=" + url.QueryEscape(week)
	if o.ShowHidden {
		query += "&hidden=1"
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
	view := printView{weekView: newWeekView(start, nil), Options: opts}
	for _, intention := range list {
		if intention.IsHidden {
			view.HiddenCount++
		}
	}

	for n := 1; n <= maxBlankLines; n++ {
		view.Lines = append(view.Lines, searchOption{Value: strconv.Itoa(n), Label: strconv.Itoa(n), Selected: n == opts.Lines})
	}
	for _, s := range schedules {
		view.Schedules = append(view.Schedules, searchOption{Value: strconv.Itoa(s.ID), Label: s.Name, Selected: s.ID == opts.ScheduleID})
	}

	return view
}

func (v printView) ShowHidden() bool {
	return v.Options.ShowHidden
}

func (v printView) PDFURL() string {
	return "/pdf?" + v.Options.query(v.Week())
}
