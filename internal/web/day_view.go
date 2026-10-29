package web

import (
	"time"

	"go-parafia/internal/intentions"
)

var weekdayNames = [...]string{
	"Niedziela",
	"Poniedziałek",
	"Wtorek",
	"Środa",
	"Czwartek",
	"Piątek",
	"Sobota",
}

type dayView struct {
	Date  string
	Label string
}

func newDayView(day time.Time) dayView {
	return dayView{
		Date:  day.Format(intentions.DateLayout),
		Label: weekdayNames[day.Weekday()] + " " + day.Format("2.1."),
	}
}
