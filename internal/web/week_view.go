package web

import (
	"fmt"
	"time"

	"go-parafia/internal/intentions"
)

type weekView struct {
	Start time.Time
	Range string
	Prev  string
	Next  string
	Days  []dayView

	IsUpcoming bool
}

func newWeekView(start time.Time, intentionsList []intentions.Intention) weekView {
	end := start.AddDate(0, 0, 6)

	byDate := make(map[string][]intentions.Intention)
	for _, intention := range intentionsList {
		date := intention.MassAt.In(intentions.TimeZone).Format(intentions.DateLayout)
		byDate[date] = append(byDate[date], intention)
	}

	days := make([]dayView, 0, 7)
	for i := range 7 {
		day := start.AddDate(0, 0, i)
		days = append(days, newDayView(day, byDate[day.Format(intentions.DateLayout)]))
	}

	return weekView{
		Start: start,
		Range: fmt.Sprintf("%s r. – %s r.", start.Format("2.1.2006"), end.Format("2.1.2006")),
		Prev:  start.AddDate(0, 0, -7).Format(intentions.DateLayout),
		Next:  start.AddDate(0, 0, 7).Format(intentions.DateLayout),
		Days:  days,
	}
}

func (v weekView) IsEmpty() bool {
	for _, day := range v.Days {
		if len(day.Masses) > 0 {
			return false
		}
	}

	return true
}
