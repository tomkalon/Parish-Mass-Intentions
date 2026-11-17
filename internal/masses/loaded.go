package masses

import (
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-parafia/internal/intentions"
)

type loadedWeek struct {
	WeekStart  string `gorm:"primaryKey"`
	ScheduleID int    `gorm:"not null"`
}

func (loadedWeek) TableName() string {
	return "loaded_weeks"
}

func MarkLoaded(db *gorm.DB, start time.Time, scheduleID int) error {
	row := loadedWeek{WeekStart: weekKey(start), ScheduleID: scheduleID}

	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func Unmark(db *gorm.DB, start time.Time) error {
	return db.Where("week_start = ?", weekKey(start)).Delete(&loadedWeek{}).Error
}

func loadedWeeks(db *gorm.DB, from, to time.Time) (map[string]bool, error) {
	var keys []string
	err := db.Model(&loadedWeek{}).
		Where("week_start >= ? AND week_start < ?", weekKey(from), to.In(intentions.TimeZone).Format(intentions.DateLayout)).
		Pluck("week_start", &keys).Error
	if err != nil {
		return nil, err
	}

	loaded := make(map[string]bool, len(keys))
	for _, key := range keys {
		loaded[key] = true
	}

	return loaded, nil
}

func weekKey(day time.Time) string {
	return intentions.WeekStart(day).Format(intentions.DateLayout)
}

type Mass struct {
	At        time.Time
	Parish    int
	Outside   int
	Gregorian bool
	Planned   bool
}

func (m Mass) Free() bool {
	return m.Parish == 0
}

// InRange lists the parish Masses between from and to. A Mass is every time
// with an intention or an empty slot; weeks whose hours were never loaded
// also get the hours of the planned schedule (nil: none), marked Planned.
func InRange(db *gorm.DB, from, to time.Time, planned *Schedule) ([]Mass, error) {
	from = from.In(intentions.TimeZone)
	to = to.In(intentions.TimeZone)

	var list []intentions.Intention
	err := db.Select("mass_at", "is_placeholder", "is_outside_parish", "is_gregorian").
		Where("mass_at >= ? AND mass_at < ?", from, to).
		Order("mass_at").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	var result []Mass
	index := make(map[int64]int)
	withSlots := make(map[string]bool)
	for _, intention := range list {
		at := intention.MassAt.In(intentions.TimeZone)
		i, ok := index[at.Unix()]
		if !ok {
			result = append(result, Mass{At: at})
			i = len(result) - 1
			index[at.Unix()] = i
		}

		mass := &result[i]
		switch {
		case intention.IsPlaceholder:
			withSlots[weekKey(at)] = true
		case intention.IsOutsideParish:
			mass.Outside++
		default:
			mass.Parish++
			mass.Gregorian = mass.Gregorian || intention.IsGregorian
		}
	}

	if planned == nil {
		return result, nil
	}

	loaded, err := loadedWeeks(db, from, to)
	if err != nil {
		return nil, err
	}

	added := false
	for day := time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, intentions.TimeZone); day.Before(to); day = day.AddDate(0, 0, 1) {
		key := weekKey(day)
		if loaded[key] || withSlots[key] {
			continue
		}

		for _, clock := range planned.Masses[day.Weekday()] {
			at, err := time.ParseInLocation(intentions.MassAtLayout, day.Format(intentions.DateLayout)+" "+clock, intentions.TimeZone)
			if err != nil || at.Before(from) || !at.Before(to) {
				continue
			}
			if _, ok := index[at.Unix()]; ok {
				continue
			}

			result = append(result, Mass{At: at, Planned: true})
			index[at.Unix()] = len(result) - 1
			added = true
		}
	}

	if added {
		slices.SortFunc(result, func(a, b Mass) int {
			return a.At.Compare(b.At)
		})
	}

	return result, nil
}
