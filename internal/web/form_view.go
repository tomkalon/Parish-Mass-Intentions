package web

import (
	"fmt"
	"net/url"
	"slices"
	"time"

	"go-parafia/internal/intentions"
)

var weekdayShortNames = [...]string{"Nd", "Pn", "Wt", "Śr", "Cz", "Pt", "So"}

type formView struct {
	Week     string
	Legend   string
	CloseURL string
	Days     []dayOption
	Times    []timeOption

	Day             string
	Time            string
	Content         string
	IsOutsideParish bool
	Errors          map[string]string

	dayIndex int
}

type dayOption struct {
	Value   string
	Short   string
	Date    string
	Checked bool
}

type timeOption struct {
	Value    string
	Label    string
	Selected bool
}

func newFormView(start time.Time, day, massTime string) formView {
	week := start.Format(intentions.DateLayout)
	form := formView{
		Week:     week,
		Legend:   start.Format("2.1") + " – " + start.AddDate(0, 0, 6).Format("2.1.2006"),
		CloseURL: "/?week=" + week,
		dayIndex: -1,
	}

	for i := range 7 {
		date := start.AddDate(0, 0, i)
		option := dayOption{
			Value: date.Format(intentions.DateLayout),
			Short: weekdayShortNames[date.Weekday()],
			Date:  date.Format("2.1"),
		}
		if option.Value == day {
			option.Checked = true
			form.Day = day
			form.dayIndex = i
		}
		form.Days = append(form.Days, option)
	}

	if t, err := time.Parse("15:04", massTime); err == nil {
		form.Time = t.Format("15:04")
	}
	form.Times = timeOptions(form.Time)

	return form
}

func timeOptions(selected string) []timeOption {
	var options []timeOption
	found := false
	for minutes := 6 * 60; minutes <= 21*60; minutes += 15 {
		value := fmt.Sprintf("%02d:%02d", minutes/60, minutes%60)
		options = append(options, timeOption{
			Value:    value,
			Label:    fmt.Sprintf("%d:%02d", minutes/60, minutes%60),
			Selected: value == selected,
		})
		if value == selected {
			found = true
		}
	}

	if selected != "" && !found {
		options = slices.Insert(options, 0, timeOption{Value: selected, Label: selected, Selected: true})
	}

	return options
}

func markSelected(days []dayView, date, massTime string) {
	for i := range days {
		if days[i].Date != date {
			continue
		}

		days[i].IsSelected = true
		for j := range days[i].Masses {
			days[i].Masses[j].IsSelected = days[i].Masses[j].TimeValue == massTime
		}
	}
}

func newIntentionURL(week, day, massTime string) string {
	query := url.Values{}
	query.Set("week", week)
	query.Set("new", "1")
	if massTime != "" {
		query.Set("time", massTime)
	}
	if day == "" {
		return "/?" + query.Encode()
	}

	query.Set("day", day)

	return "/?" + query.Encode() + "#d-" + day
}
