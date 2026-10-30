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
	from := start.Format(intentions.DateLayout)
	to := start.AddDate(0, 0, 6).Format(intentions.DateLayout)

	var days []Day
	err := db.Where("date BETWEEN ? AND ?", from, to).Find(&days).Error
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

func AddMissing(db *gorm.DB, feasts map[string]string) error {
	var days []Day
	for _, date := range slices.Sorted(maps.Keys(feasts)) {
		if feasts[date] != "" {
			days = append(days, Day{Date: date, Feast: feasts[date]})
		}
	}
	if len(days) == 0 {
		return nil
	}

	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&days).Error
}

func SaveFeast(db *gorm.DB, date, feast string) error {
	if feast == "" {
		return db.Delete(&Day{Date: date}).Error
	}

	return db.Save(&Day{Date: date, Feast: feast}).Error
}
