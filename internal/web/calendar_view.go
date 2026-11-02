package web

import (
	"strconv"
	"time"

	"go-parafia/internal/intentions"
)

const maxCalendarLevel = 5

var monthNames = [...]string{
	"Styczeń",
	"Luty",
	"Marzec",
	"Kwiecień",
	"Maj",
	"Czerwiec",
	"Lipiec",
	"Sierpień",
	"Wrzesień",
	"Październik",
	"Listopad",
	"Grudzień",
}

type calendarView struct {
	Year     int
	PrevURL  string
	NextURL  string
	CloseURL string
	Months   []calendarMonth

	HasCurrent bool
}

type calendarMonth struct {
	Name  string
	Weeks []calendarWeek
}

type calendarWeek struct {
	URL   string
	Label string
	Days  []calendarDay

	IsCurrent bool
}

type calendarDay struct {
	Day   int
	Count int
	Level int

	InMonth  bool
	HasEmpty bool
	IsToday  bool
}

func newCalendarView(year int, current, today time.Time, counts map[string]intentions.DayCount) calendarView {
	week := current.Format(intentions.DateLayout)
	todayDate := today.In(intentions.TimeZone).Format(intentions.DateLayout)

	view := calendarView{
		Year:     year,
		PrevURL:  calendarURL(week, year-1),
		NextURL:  calendarURL(week, year+1),
		CloseURL: weekURL(current),
		Months:   make([]calendarMonth, 0, 12),
	}

	for month := time.January; month <= time.December; month++ {
		first := time.Date(year, month, 1, 0, 0, 0, 0, intentions.TimeZone)
		next := first.AddDate(0, 1, 0)

		var weeks []calendarWeek
		for start := intentions.WeekStart(first); start.Before(next); start = start.AddDate(0, 0, 7) {
			w := newCalendarWeek(start, month, todayDate, counts)
			w.IsCurrent = start.Equal(current)
			view.HasCurrent = view.HasCurrent || w.IsCurrent
			weeks = append(weeks, w)
		}

		view.Months = append(view.Months, calendarMonth{Name: monthNames[month-1], Weeks: weeks})
	}

	return view
}

func newCalendarWeek(start time.Time, month time.Month, today string, counts map[string]intentions.DayCount) calendarWeek {
	end := start.AddDate(0, 0, 6)
	total := 0
	hasEmpty := false

	days := make([]calendarDay, 0, 7)
	for i := range 7 {
		day := start.AddDate(0, 0, i)
		date := day.Format(intentions.DateLayout)
		count := counts[date]
		total += count.Filled
		hasEmpty = hasEmpty || count.Placeholders > 0

		if day.Month() != month {
			days = append(days, calendarDay{})
			continue
		}

		days = append(days, calendarDay{
			Day:      day.Day(),
			Count:    count.Filled,
			Level:    min(count.Filled, maxCalendarLevel),
			InMonth:  true,
			HasEmpty: count.Placeholders > 0,
			IsToday:  date == today,
		})
	}

	label := "Tydzień " + start.Format("2.1") + " – " + end.Format("2.1.2006") + ": " + intentionsCount(total)
	if hasEmpty {
		label += ", są msze do uzupełnienia"
	}

	return calendarWeek{URL: weekURL(start), Label: label, Days: days}
}

func (d calendarDay) Title() string {
	title := intentionsCount(d.Count)
	if d.HasEmpty {
		title += " · msze do uzupełnienia"
	}

	return title
}

func calendarRange(year int) (time.Time, time.Time) {
	first := time.Date(year, time.January, 1, 0, 0, 0, 0, intentions.TimeZone)
	last := time.Date(year, time.December, 31, 0, 0, 0, 0, intentions.TimeZone)

	return intentions.WeekStart(first), intentions.WeekStart(last).AddDate(0, 0, 7)
}

func calendarURL(week string, year int) string {
	return "/?week=" + week + "&calendar=" + strconv.Itoa(year)
}
