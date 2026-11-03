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
	Date     string
	Label    string
	Feast    string
	FeastURL string
	NewURL   string
	Masses   []massView

	SeriesDue []seriesDue

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

func (m massView) ParishFilled() int {
	count := 0
	for _, intention := range m.Parish {
		if !intention.IsPlaceholder {
			count++
		}
	}

	return count
}

func (m massView) ParishGregorians() int {
	count := 0
	for _, intention := range m.Parish {
		if intention.IsGregorian && !intention.IsPlaceholder {
			count++
		}
	}

	return count
}

func (m massView) SharedWithGregorian() bool {
	return m.ParishGregorians() > 0 && m.ParishFilled() > 1
}

type intentionView struct {
	ID        int
	Number    int
	Content   string
	Amount    intentions.Grosze
	EditURL   string
	DeleteURL string

	SeriesID   int
	SeriesStep int
	SeriesURL  string

	IsUnpaid      bool
	IsSaved       bool
	IsEditing     bool
	IsPlaceholder bool
	IsGregorian   bool
	IsDeceased    bool
	IsHidden      bool
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
		Date:     date,
		Label:    dayLabel(day),
		FeastURL: feastURL(week, date),
		NewURL:   newIntentionURL(week, date, ""),
		Masses:   masses,
	}
}

func appendIntention(list []intentionView, intention intentions.Intention, week string) []intentionView {
	view := intentionView{
		ID:        intention.ID,
		Content:   intention.Content,
		Amount:    intention.Amount,
		SeriesID:  intention.SeriesID,
		SeriesURL: seriesLink(week, intention.SeriesID),
		EditURL:   editURL(week, intention.ID),
		DeleteURL: deleteURL(week, intention.ID),

		IsPlaceholder: intention.IsPlaceholder,
		IsGregorian:   intention.IsGregorian,
		IsDeceased:    intention.IsDeceased,
		IsHidden:      intention.IsHidden,
	}

	if !intention.IsPlaceholder {
		view.Number = 1
		for _, previous := range list {
			if !previous.IsPlaceholder {
				view.Number++
			}
		}
	}

	return append(list, view)
}

func dayLabel(day time.Time) string {
	return weekdayNames[day.Weekday()] + " " + day.Format("2.1.")
}

func clockLabel(at time.Time) string {
	return fmt.Sprintf("%d:%02d", at.Hour(), at.Minute())
}
