package intentions

import (
	"time"

	"gorm.io/gorm"
)

func ForWeek(db *gorm.DB, start time.Time) ([]Intention, error) {
	var list []Intention
	err := inWeek(db, start).
		Order("mass_at, id").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	return list, nil
}

func DeleteWeek(db *gorm.DB, start time.Time) error {
	return inWeek(db, start).Delete(&Intention{}).Error
}

func inWeek(db *gorm.DB, start time.Time) *gorm.DB {
	start = start.In(TimeZone)
	end := start.AddDate(0, 0, 7)

	return db.Where("mass_at >= ? AND mass_at < ?", start, end)
}
