package intentions

import (
	"slices"
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
	Unpaid       int
	Amount       Grosze
}

func CountByDay(db *gorm.DB, from, to time.Time) (map[string]DayCount, error) {
	var list []Intention
	err := inRange(db, from, to).
		Select("id", "mass_at", "is_placeholder", "amount", "series_id").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	paidInSeries, err := paidInSeries(db, list)
	if err != nil {
		return nil, err
	}

	counts := make(map[string]DayCount)
	for _, intention := range list {
		date := intention.MassAt.In(TimeZone).Format(DateLayout)
		count := counts[date]
		switch {
		case intention.IsPlaceholder:
			count.Placeholders++
		case intention.Amount > 0:
			count.Filled++
			count.Amount += intention.Amount
		case paidInSeries[intention.ID]:
			count.Filled++
		default:
			count.Filled++
			count.Unpaid++
		}
		counts[date] = count
	}

	return counts, nil
}

func paidInSeries(db *gorm.DB, list []Intention) (map[int]bool, error) {
	var ids []int
	for _, intention := range list {
		if intention.SeriesID > 0 && intention.Amount == 0 && !slices.Contains(ids, intention.SeriesID) {
			ids = append(ids, intention.SeriesID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var masses []Intention
	err := db.Select("id", "series_id", "mass_at", "amount").
		Where("series_id IN ?", ids).
		Order("mass_at, id").
		Find(&masses).Error
	if err != nil {
		return nil, err
	}

	paid := make(map[int]bool)
	seen := make(map[int]bool)
	total := make(map[int]Grosze)
	for _, mass := range masses {
		total[mass.SeriesID] += mass.Amount
	}
	for _, mass := range masses {
		first := !seen[mass.SeriesID]
		seen[mass.SeriesID] = true
		paid[mass.ID] = !first || total[mass.SeriesID] > 0
	}

	return paid, nil
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
