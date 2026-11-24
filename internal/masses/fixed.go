package masses

import (
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
)

var (
	ErrFixedNotFound = errors.New("fixed intention not found")
	ErrNoSuchMass    = errors.New("schedule has no such mass")
)

type FixedIntention struct {
	ID              int
	ScheduleID      int          `gorm:"not null;index"`
	Weekday         time.Weekday `gorm:"not null"`
	Time            string       `gorm:"not null"`
	Content         string       `gorm:"not null"`
	IsOutsideParish bool         `gorm:"not null;default:false"`
	IsDeceased      bool         `gorm:"not null;default:false"`
	IsHidden        bool         `gorm:"not null;default:false"`
}

func (FixedIntention) TableName() string {
	return "fixed_intentions"
}

func (s Schedule) HasMass(day time.Weekday, clock string) bool {
	return hasMass(s.Masses, day, clock)
}

func hasMass(masses [7][]string, day time.Weekday, clock string) bool {
	if day < time.Sunday || day > time.Saturday {
		return false
	}

	return slices.Contains(masses[day], clock)
}

func (s Schedule) FixedAt(day time.Weekday, clock string) []FixedIntention {
	var list []FixedIntention
	for _, f := range s.Fixed {
		if f.Weekday == day && f.Time == clock {
			list = append(list, f)
		}
	}

	return list
}

func Orphans(fixed []FixedIntention, masses [7][]string) []FixedIntention {
	var orphans []FixedIntention
	for _, f := range fixed {
		if !hasMass(masses, f.Weekday, f.Time) {
			orphans = append(orphans, f)
		}
	}

	return orphans
}

func FindFixed(db *gorm.DB, scheduleID, id int) (FixedIntention, error) {
	var list []FixedIntention
	err := db.Where("id = ? AND schedule_id = ?", id, scheduleID).Limit(1).Find(&list).Error
	if err != nil {
		return FixedIntention{}, err
	}
	if len(list) == 0 {
		return FixedIntention{}, ErrFixedNotFound
	}

	return list[0], nil
}

func SaveFixed(db *gorm.DB, f *FixedIntention) error {
	return db.Transaction(func(tx *gorm.DB) error {
		schedule, err := Find(tx, f.ScheduleID)
		if err != nil {
			return err
		}
		if !schedule.HasMass(f.Weekday, f.Time) {
			return ErrNoSuchMass
		}

		if f.ID == 0 {
			return tx.Create(f).Error
		}

		result := tx.Model(&FixedIntention{}).
			Where("id = ? AND schedule_id = ?", f.ID, f.ScheduleID).
			Select("weekday", "time", "content", "is_outside_parish", "is_deceased", "is_hidden").
			Updates(f)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrFixedNotFound
		}

		return nil
	})
}

func DeleteFixed(db *gorm.DB, scheduleID, id int) error {
	result := db.Where("id = ? AND schedule_id = ?", id, scheduleID).Delete(&FixedIntention{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrFixedNotFound
	}

	return nil
}

func deleteOrphans(tx *gorm.DB, scheduleID int, masses [7][]string) error {
	var fixed []FixedIntention
	err := tx.Where("schedule_id = ?", scheduleID).Find(&fixed).Error
	if err != nil {
		return err
	}

	orphans := Orphans(fixed, masses)
	if len(orphans) == 0 {
		return nil
	}

	ids := make([]int, 0, len(orphans))
	for _, f := range orphans {
		ids = append(ids, f.ID)
	}

	return tx.Delete(&FixedIntention{}, ids).Error
}
