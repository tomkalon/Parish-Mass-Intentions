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

type DayCount struct {
	Filled       int
	Placeholders int
}

func CountByDay(db *gorm.DB, from, to time.Time) (map[string]DayCount, error) {
	var list []Intention
	err := inRange(db, from, to).
		Select("mass_at", "is_placeholder").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	counts := make(map[string]DayCount)
	for _, intention := range list {
		date := intention.MassAt.In(TimeZone).Format(DateLayout)
		count := counts[date]
		if intention.IsPlaceholder {
			count.Placeholders++
		} else {
			count.Filled++
		}
		counts[date] = count
	}

	return counts, nil
}

func Save(db *gorm.DB, intention *Intention) error {
	intention.MassAt = intention.MassAt.In(TimeZone)

	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Save(intention).Error
		if err != nil {
			return err
		}

		return tx.Where("mass_at = ? AND is_placeholder = ? AND id <> ?", intention.MassAt, true, intention.ID).
			Delete(&Intention{}).Error
	})
}

func DeleteWeek(db *gorm.DB, start time.Time) error {
	return inWeek(db, start).Delete(&Intention{}).Error
}

func inWeek(db *gorm.DB, start time.Time) *gorm.DB {
	start = start.In(TimeZone)

	return inRange(db, start, start.AddDate(0, 0, 7))
}

func inRange(db *gorm.DB, from, to time.Time) *gorm.DB {
	return db.Where("mass_at >= ? AND mass_at < ?", from.In(TimeZone), to.In(TimeZone))
}
