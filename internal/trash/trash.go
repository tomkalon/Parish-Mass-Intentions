package trash

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

const RetentionDays = 30

var ErrBatchNotFound = errors.New("trash batch not found")

type Entry struct {
	ID         int                  `gorm:"primaryKey"`
	Batch      int64                `gorm:"index;not null"`
	TrashedAt  time.Time            `gorm:"index;not null"`
	Week       string               `gorm:"not null;default:''"`
	ScheduleID int                  `gorm:"not null;default:0"`
	Intention  intentions.Intention `gorm:"serializer:json;not null"`
	Series     *gregorian.Series    `gorm:"serializer:json"`
}

func (Entry) TableName() string {
	return "trash"
}

type Batch struct {
	ID         int64
	TrashedAt  time.Time
	Week       string
	Intentions []intentions.Intention
	Empty      int
}

func Move(tx *gorm.DB, list []intentions.Intention, week time.Time, now time.Time) (int64, error) {
	if len(list) == 0 {
		return 0, nil
	}

	err := Purge(tx, now)
	if err != nil {
		return 0, err
	}

	var last int64
	err = tx.Model(&Entry{}).Select("COALESCE(MAX(batch), 0)").Scan(&last).Error
	if err != nil {
		return 0, err
	}
	batch := max(now.UnixMilli(), last+1)

	weekKey := ""
	scheduleID := 0
	if !week.IsZero() {
		weekKey = week.In(intentions.TimeZone).Format(intentions.DateLayout)
		scheduleID, err = masses.LoadedSchedule(tx, week)
		if err != nil {
			return 0, err
		}
	}

	series := make(map[int]*gregorian.Series)
	entries := make([]Entry, 0, len(list))
	for _, intention := range list {
		entry := Entry{Batch: batch, TrashedAt: now, Week: weekKey, ScheduleID: scheduleID, Intention: intention}
		if id := intention.SeriesID; id > 0 {
			snapshot, ok := series[id]
			if !ok {
				found, err := gregorian.Find(tx, id)
				switch {
				case errors.Is(err, gregorian.ErrSeriesNotFound):
				case err != nil:
					return 0, err
				default:
					snapshot = &found
				}
				series[id] = snapshot
			}
			entry.Series = snapshot
		}
		entries = append(entries, entry)
	}

	err = tx.CreateInBatches(&entries, 100).Error
	if err != nil {
		return 0, fmt.Errorf("moving intentions to the trash: %w", err)
	}

	return batch, nil
}

func Load(db *gorm.DB, batch int64) (Batch, error) {
	var entries []Entry
	err := db.Where("batch = ?", batch).Order("id").Find(&entries).Error
	if err != nil {
		return Batch{}, err
	}
	if len(entries) == 0 {
		return Batch{}, ErrBatchNotFound
	}

	return newBatch(entries), nil
}

func List(db *gorm.DB, now time.Time) ([]Batch, error) {
	err := Purge(db, now)
	if err != nil {
		return nil, err
	}

	var entries []Entry
	err = db.Order("batch DESC, id").Find(&entries).Error
	if err != nil {
		return nil, err
	}

	var batches []Batch
	for start := 0; start < len(entries); {
		end := start + 1
		for end < len(entries) && entries[end].Batch == entries[start].Batch {
			end++
		}
		batches = append(batches, newBatch(entries[start:end]))
		start = end
	}

	return batches, nil
}

func newBatch(entries []Entry) Batch {
	b := Batch{ID: entries[0].Batch, TrashedAt: entries[0].TrashedAt, Week: entries[0].Week}
	for _, entry := range entries {
		if entry.Intention.IsPlaceholder {
			b.Empty++
			continue
		}
		b.Intentions = append(b.Intentions, entry.Intention)
	}
	slices.SortStableFunc(b.Intentions, func(x, y intentions.Intention) int {
		return x.MassAt.Compare(y.MassAt)
	})

	return b
}

func Purge(db *gorm.DB, now time.Time) error {
	return db.Where("trashed_at < ?", now.AddDate(0, 0, -RetentionDays)).Delete(&Entry{}).Error
}

func Restore(db *gorm.DB, batch int64) ([]intentions.Intention, error) {
	var restored []intentions.Intention

	err := db.Transaction(func(tx *gorm.DB) error {
		var entries []Entry
		err := tx.Where("batch = ?", batch).Order("id").Find(&entries).Error
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			return ErrBatchNotFound
		}

		existed, err := restoreSeries(tx, entries)
		if err != nil {
			return err
		}

		slices.SortStableFunc(entries, func(a, b Entry) int {
			switch {
			case a.Intention.IsPlaceholder == b.Intention.IsPlaceholder:
				return 0
			case a.Intention.IsPlaceholder:
				return 1
			default:
				return -1
			}
		})

		var touched []int
		for _, entry := range entries {
			intention := entry.Intention
			if intention.IsPlaceholder {
				taken, err := massTaken(tx, intention.MassAt)
				if err != nil {
					return err
				}
				if taken {
					continue
				}
			}

			if id := intention.SeriesID; id > 0 {
				ok, known := existed[id]
				switch {
				case !known:
					intention.SeriesID = 0
				case ok:
					intention.Amount = 0
				}
				if intention.SeriesID > 0 && !slices.Contains(touched, id) {
					touched = append(touched, id)
				}
			}

			var count int64
			err = tx.Model(&intentions.Intention{}).Where("id = ?", intention.ID).Count(&count).Error
			if err != nil {
				return err
			}
			if count > 0 {
				intention.ID = 0
			}

			err = intentions.Save(tx, &intention)
			if err != nil {
				return err
			}
			restored = append(restored, intention)
		}

		for _, id := range touched {
			err = gregorian.Normalize(tx, id)
			if err != nil {
				return err
			}
		}

		if week := entries[0].Week; week != "" && entries[0].ScheduleID > 0 {
			start, err := time.ParseInLocation(intentions.DateLayout, week, intentions.TimeZone)
			if err != nil {
				return err
			}
			err = masses.MarkLoaded(tx, start, entries[0].ScheduleID)
			if err != nil {
				return err
			}
		}

		return tx.Where("batch = ?", batch).Delete(&Entry{}).Error
	})
	if err != nil {
		return nil, err
	}

	slices.SortStableFunc(restored, func(x, y intentions.Intention) int {
		return x.MassAt.Compare(y.MassAt)
	})

	return restored, nil
}

func restoreSeries(tx *gorm.DB, entries []Entry) (map[int]bool, error) {
	existed := make(map[int]bool)
	for _, entry := range entries {
		id := entry.Intention.SeriesID
		if id == 0 {
			continue
		}
		if _, done := existed[id]; done {
			continue
		}

		_, err := gregorian.Find(tx, id)
		switch {
		case err == nil:
			existed[id] = true
		case !errors.Is(err, gregorian.ErrSeriesNotFound):
			return nil, err
		case entry.Series != nil:
			snapshot := *entry.Series
			err = tx.Create(&snapshot).Error
			if err != nil {
				return nil, fmt.Errorf("restoring gregorian series %d: %w", id, err)
			}
			existed[id] = false
		}
	}

	return existed, nil
}

func massTaken(tx *gorm.DB, at time.Time) (bool, error) {
	var count int64
	err := tx.Model(&intentions.Intention{}).
		Where("mass_at = ? AND is_outside_parish = ?", at.In(intentions.TimeZone), false).
		Count(&count).Error

	return count > 0, err
}
