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
	Topbar    topbarView

	SelectedScheduleID int
	ParishName         string
	Address            string

	IsUpcoming  bool
	UpcomingURL string
	Form        *formView
	Delete      *deleteView
	Clear       *clearView
	Feast       *feastView

	ImportError string
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
	return weekURL(v.Start)
}

func weekURL(start time.Time) string {
	return "/?week=" + start.Format(intentions.DateLayout)
}

func (v weekView) SettingsURL() string {
	return settingsURL(v.Week(), false)
}

func (v weekView) ClearURL() string {
	return "/?clear=1&week=" + v.Start.Format(intentions.DateLayout)
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

func setFeasts(days []dayView, feasts map[string]string) {
	for i := range days {
		days[i].Feast = feasts[days[i].Date]
	}
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

type emptyMassesNotice struct {
	Text string
	URL  string
}

func (v weekView) EmptyMasses() *emptyMassesNotice {
	count := 0
	first := ""
	for _, day := range v.Days {
		for _, mass := range day.Masses {
			if !withoutIntention(mass) {
				continue
			}
			count++
			if first == "" {
				first = day.Date
			}
		}
	}
	if count == 0 {
		return nil
	}

	return &emptyMassesNotice{Text: emptyMassesText(count), URL: "#d-" + first}
}

func withoutIntention(mass massView) bool {
	if len(mass.Outside) > 0 {
		return false
	}
	for _, intention := range mass.Parish {
		if !intention.IsPlaceholder {
			return false
		}
	}

	return len(mass.Parish) > 0
}

func emptyMassesText(n int) string {
	switch {
	case n == 1:
		return "1 msza nie ma uzupełnionej intencji."
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d msze nie mają uzupełnionej intencji.", n)
	default:
		return fmt.Sprintf("%d mszy nie ma uzupełnionej intencji.", n)
	}
}
