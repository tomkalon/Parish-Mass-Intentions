package web

import (
	"fmt"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

type weekView struct {
	Start     time.Time
	Range     string
	Days      []dayView
	Schedules []masses.Schedule

	SelectedSchedule string

	IsUpcoming bool
	Form       *formView
	Delete     *deleteView
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
		Days:  days,
	}
}

func (v weekView) Week() string {
	return v.Start.Format(intentions.DateLayout)
}

func (v weekView) PrevURL() string {
	return v.urlFor(v.Start.AddDate(0, 0, -7))
}

func (v weekView) NextURL() string {
	return v.urlFor(v.Start.AddDate(0, 0, 7))
}

func (v weekView) NewURL() string {
	return newIntentionURL(v.Start.Format(intentions.DateLayout), "", "")
}

func (v weekView) PDFURL() string {
	return "/pdf?week=" + v.Start.Format(intentions.DateLayout)
}

func (v weekView) PrintURL() string {
	return "/print?week=" + v.Start.Format(intentions.DateLayout)
}

func (v weekView) WeekURL() string {
	return "/?week=" + v.Start.Format(intentions.DateLayout)
}

func (v weekView) urlFor(start time.Time) string {
	week := start.Format(intentions.DateLayout)
	if v.Form == nil {
		return "/?week=" + week
	}

	day := ""
	if v.Form.dayIndex >= 0 {
		day = start.AddDate(0, 0, v.Form.dayIndex).Format(intentions.DateLayout)
	}

	return formURL(week, day, v.Form.Time, v.Form.EditID)
}

func (v weekView) IsEmpty() bool {
	for _, day := range v.Days {
		if len(day.Masses) > 0 {
			return false
		}
	}

	return true
}

func markSaved(days []dayView, id int) {
	forEachIntention(days, func(intention *intentionView) {
		intention.IsSaved = intention.ID == id
	})
}

func markEditing(days []dayView, id int) {
	forEachIntention(days, func(intention *intentionView) {
		intention.IsEditing = intention.ID == id
	})
}

func forEachIntention(days []dayView, fn func(intention *intentionView)) {
	for i := range days {
		for j := range days[i].Masses {
			mass := &days[i].Masses[j]
			for k := range mass.Parish {
				fn(&mass.Parish[k])
			}
			for k := range mass.Outside {
				fn(&mass.Outside[k])
			}
		}
	}
}
