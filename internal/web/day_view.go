package web

import (
	"fmt"
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
	Date   string
	Label  string
	NewURL string
	Masses []massView

	IsSelected bool
}

type massView struct {
	Time      string
	TimeValue string
	NewURL    string
	Parish    []intentionView
	Outside   []intentionView

	IsSelected bool
}

type intentionView struct {
	ID      int
	Number  int
	Content string
}

func newDayView(day time.Time, list []intentions.Intention) dayView {
	date := day.Format(intentions.DateLayout)
	week := intentions.WeekStart(day).Format(intentions.DateLayout)

	var masses []massView
	for _, intention := range list {
		at := intention.MassAt.In(intentions.TimeZone)
		massTime := fmt.Sprintf("%d:%02d", at.Hour(), at.Minute())

		if len(masses) == 0 || masses[len(masses)-1].Time != massTime {
			timeValue := at.Format("15:04")
			masses = append(masses, massView{
				Time:      massTime,
				TimeValue: timeValue,
				NewURL:    newIntentionURL(week, date, timeValue),
			})
		}
		mass := &masses[len(masses)-1]

		if intention.IsOutsideParish {
			mass.Outside = appendIntention(mass.Outside, intention)
		} else {
			mass.Parish = appendIntention(mass.Parish, intention)
		}
	}

	return dayView{
		Date:   date,
		Label:  weekdayNames[day.Weekday()] + " " + day.Format("2.1."),
		NewURL: newIntentionURL(week, date, ""),
		Masses: masses,
	}
}

func appendIntention(list []intentionView, intention intentions.Intention) []intentionView {
	return append(list, intentionView{
		ID:      intention.ID,
		Number:  len(list) + 1,
		Content: intention.Content,
	})
}
