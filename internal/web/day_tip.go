package web

import (
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

type dayTip struct {
	Label   string
	Summary string
	Outside string
	Masses  []searchMass
}

func newDayTip(day time.Time, list []masses.Mass) *dayTip {
	if len(list) == 0 {
		return nil
	}

	stats := massStats(list)
	tip := &dayTip{
		Label:   weekdayNames[day.Weekday()] + " " + day.Format("2.1."),
		Summary: stats.Summary(),
		Outside: stats.OutsideLabel(),
	}

	week := intentions.WeekStart(day).Format(intentions.DateLayout)
	date := day.Format(intentions.DateLayout)
	for _, m := range list {
		tip.Masses = append(tip.Masses, newSearchMass(m, week, date))
	}

	return tip
}

func massStats(list []masses.Mass) dayStats {
	stats := dayStats{Masses: len(list)}
	for _, m := range list {
		stats.Intentions += m.Parish
		stats.Outside += m.Outside
		if m.Free() {
			stats.Free++
		}
	}

	return stats
}

func massesByDay(list []masses.Mass) map[string][]masses.Mass {
	byDay := make(map[string][]masses.Mass)
	for _, m := range list {
		date := m.At.Format(intentions.DateLayout)
		byDay[date] = append(byDay[date], m)
	}

	return byDay
}
