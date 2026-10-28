package intentions

import (
	"time"
	_ "time/tzdata"
)

const (
	MassAtLayout = "2006-01-02 15:04"
	DateLayout   = "2006-01-02"
)

var TimeZone = mustLoadLocation("Europe/Warsaw")

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

func WeekStart(day time.Time) time.Time {
	day = day.In(TimeZone)

	daysSinceMonday := (int(day.Weekday()) + 6) % 7

	y, m, d := day.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, TimeZone)

	return midnight.AddDate(0, 0, -daysSinceMonday)
}

func NextWeekStart(day time.Time) time.Time {
	return WeekStart(day).AddDate(0, 0, 7)
}
