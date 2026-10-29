package intentions

import (
	"time"

	"gorm.io/gorm"
)

func ForWeek(db *gorm.DB, start time.Time) ([]Intention, error) {
	start = start.In(TimeZone)
	end := start.AddDate(0, 0, 7)

	var list []Intention
	err := db.
		Where("mass_at >= ? AND mass_at < ?", start, end).
		Order("mass_at, id").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	return list, nil
}
