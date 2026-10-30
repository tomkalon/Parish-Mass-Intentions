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
	ID        int
	Number    int
	Content   string
	EditURL   string
	DeleteURL string

	IsSaved       bool
	IsEditing     bool
	IsPlaceholder bool
}

func newDayView(day time.Time, list []intentions.Intention) dayView {
	date := day.Format(intentions.DateLayout)
	week := intentions.WeekStart(day).Format(intentions.DateLayout)

	var masses []massView
	for _, intention := range list {
		at := intention.MassAt.In(intentions.TimeZone)
		massTime := clockLabel(at)

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
			mass.Outside = appendIntention(mass.Outside, intention, week)
		} else {
			mass.Parish = appendIntention(mass.Parish, intention, week)
		}
	}

	return dayView{
		Date:   date,
		Label:  dayLabel(day),
		NewURL: newIntentionURL(week, date, ""),
		Masses: masses,
	}
}

func appendIntention(list []intentionView, intention intentions.Intention, week string) []intentionView {
	return append(list, intentionView{
		ID:        intention.ID,
		Number:    len(list) + 1,
		Content:   intention.Content,
		EditURL:   editURL(week, intention.ID),
		DeleteURL: deleteURL(week, intention.ID),

		IsPlaceholder: intention.IsPlaceholder,
	})
}

func dayLabel(day time.Time) string {
	return weekdayNames[day.Weekday()] + " " + day.Format("2.1.")
}

func clockLabel(at time.Time) string {
	return fmt.Sprintf("%d:%02d", at.Hour(), at.Minute())
}
