package calendar

import (
	"maps"
	"slices"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-parafia/internal/intentions"
)

type Day struct {
	Date  string `gorm:"primaryKey"`
	Feast string
}

func FeastsForWeek(db *gorm.DB, start time.Time) (map[string]string, error) {
	start = start.In(intentions.TimeZone)

	return FeastsBetween(db, start, start.AddDate(0, 0, 7))
}

func FeastsBetween(db *gorm.DB, from, to time.Time) (map[string]string, error) {
	first := from.In(intentions.TimeZone).Format(intentions.DateLayout)
	end := to.In(intentions.TimeZone).Format(intentions.DateLayout)

	var days []Day
	err := db.Where("date >= ? AND date < ?", first, end).Find(&days).Error
	if err != nil {
		return nil, err
	}

	feasts := make(map[string]string, len(days))
	for _, day := range days {
		feasts[day.Date] = day.Feast
	}

	return feasts, nil
}

func Feast(db *gorm.DB, date string) (string, error) {
	var day Day
	err := db.Limit(1).Find(&day, "date = ?", date).Error

	return day.Feast, err
}

func AddMissing(db *gorm.DB, feasts map[string]string) (int, error) {
	var days []Day
	for _, date := range slices.Sorted(maps.Keys(feasts)) {
		if feasts[date] != "" {
			days = append(days, Day{Date: date, Feast: feasts[date]})
		}
	}
	if len(days) == 0 {
		return 0, nil
	}

	result := db.Clauses(clause.OnConflict{DoNothing: true}).Create(&days)

	return int(result.RowsAffected), result.Error
}

func SaveFeast(db *gorm.DB, date, feast string) error {
	if feast == "" {
		return db.Delete(&Day{Date: date}).Error
	}

	return db.Save(&Day{Date: date, Feast: feast}).Error
}

type FeastImport struct {
	WeekStart  string    `gorm:"primaryKey"`
	ImportedAt time.Time `gorm:"not null"`
	Added      int       `gorm:"not null;default:0"`
}

func MarkImported(db *gorm.DB, start time.Time, added int, now time.Time) error {
	row := FeastImport{WeekStart: start.In(intentions.TimeZone).Format(intentions.DateLayout), ImportedAt: now.In(intentions.TimeZone), Added: added}

	return db.Clauses(clause.OnConflict{UpdateAll: true}).Create(&row).Error
}

func LastImport(db *gorm.DB, start time.Time) (FeastImport, bool, error) {
	var rows []FeastImport
	err := db.Where("week_start = ?", start.In(intentions.TimeZone).Format(intentions.DateLayout)).Limit(1).Find(&rows).Error
	if err != nil || len(rows) == 0 {
		return FeastImport{}, false, err
	}

	return rows[0], true, nil
}
