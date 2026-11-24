package masses

import (
	"errors"
	"slices"

	"gorm.io/gorm"
)

var (
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrDuplicateName    = errors.New("schedule name already exists")
)

type scheduleRow struct {
	ID       int
	Name     string           `gorm:"not null;uniqueIndex"`
	Position int              `gorm:"not null"`
	Masses   []massRow        `gorm:"foreignKey:ScheduleID"`
	Fixed    []FixedIntention `gorm:"foreignKey:ScheduleID"`
}

func (scheduleRow) TableName() string {
	return "schedules"
}

type massRow struct {
	ID         int
	ScheduleID int    `gorm:"not null;index"`
	Weekday    int    `gorm:"not null"`
	Time       string `gorm:"not null"`
}

func (massRow) TableName() string {
	return "schedule_masses"
}

func Migrate(db *gorm.DB) error {
	firstFixed := !db.Migrator().HasTable(&FixedIntention{})

	err := db.AutoMigrate(&scheduleRow{}, &massRow{}, &FixedIntention{}, &loadedWeek{})
	if err != nil || !firstFixed {
		return err
	}

	return addDefaultFixed(db)
}

func Load(db *gorm.DB) ([]Schedule, error) {
	var rows []scheduleRow
	err := withMasses(db).Order("position, id").Find(&rows).Error
	if err != nil {
		return nil, err
	}

	schedules := make([]Schedule, 0, len(rows))
	for _, row := range rows {
		schedules = append(schedules, row.schedule())
	}

	return schedules, nil
}

func Find(db *gorm.DB, id int) (Schedule, error) {
	var row scheduleRow
	result := withMasses(db).Limit(1).Find(&row, id)
	if result.Error != nil {
		return Schedule{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Schedule{}, ErrScheduleNotFound
	}

	return row.schedule(), nil
}

func Save(db *gorm.DB, s Schedule) (int, error) {
	err := db.Transaction(func(tx *gorm.DB) error {
		taken, err := NameTaken(tx, s.Name, s.ID)
		if err != nil {
			return err
		}
		if taken {
			return ErrDuplicateName
		}

		if s.ID == 0 {
			position, err := nextPosition(tx)
			if err != nil {
				return err
			}

			s.ID, err = insertSchedule(tx, s.Name, position)
			if err != nil {
				return err
			}
		} else {
			result := tx.Model(&scheduleRow{ID: s.ID}).Update("name", s.Name)
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return ErrScheduleNotFound
			}

			err = tx.Where("schedule_id = ?", s.ID).Delete(&massRow{}).Error
			if err != nil {
				return err
			}
		}

		err = insertMasses(tx, s.ID, s.Masses)
		if err != nil {
			return err
		}

		return deleteOrphans(tx, s.ID, s.Masses)
	})

	return s.ID, err
}

func NameTaken(db *gorm.DB, name string, exceptID int) (bool, error) {
	var count int64
	err := db.Model(&scheduleRow{}).Where("name = ? AND id <> ?", name, exceptID).Count(&count).Error

	return count > 0, err
}

func Delete(db *gorm.DB, id int) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("schedule_id = ?", id).Delete(&massRow{}).Error
		if err != nil {
			return err
		}

		err = tx.Where("schedule_id = ?", id).Delete(&FixedIntention{}).Error
		if err != nil {
			return err
		}

		result := tx.Delete(&scheduleRow{}, id)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return ErrScheduleNotFound
		}

		return nil
	})
}

func SeedDefaults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var count int64
		err := tx.Model(&scheduleRow{}).Count(&count).Error
		if err != nil || count > 0 {
			return err
		}

		return insertDefaults(tx)
	})
}

func RestoreDefaults(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Where("1 = 1").Delete(&FixedIntention{}).Error
		if err != nil {
			return err
		}

		err = tx.Where("1 = 1").Delete(&massRow{}).Error
		if err != nil {
			return err
		}

		err = tx.Where("1 = 1").Delete(&scheduleRow{}).Error
		if err != nil {
			return err
		}

		return insertDefaults(tx)
	})
}

func insertDefaults(tx *gorm.DB) error {
	defaults, err := Default()
	if err != nil {
		return err
	}

	for i, s := range defaults {
		id, err := insertSchedule(tx, s.Name, i+1)
		if err != nil {
			return err
		}

		err = insertMasses(tx, id, s.Masses)
		if err != nil {
			return err
		}

		err = insertFixed(tx, id, s.Fixed)
		if err != nil {
			return err
		}
	}

	return nil
}

func insertFixed(tx *gorm.DB, scheduleID int, fixed []FixedIntention) error {
	if len(fixed) == 0 {
		return nil
	}

	rows := slices.Clone(fixed)
	for i := range rows {
		rows[i].ID = 0
		rows[i].ScheduleID = scheduleID
	}

	return tx.Create(&rows).Error
}

func addDefaultFixed(db *gorm.DB) error {
	defaults, err := Default()
	if err != nil {
		return err
	}

	return db.Transaction(func(tx *gorm.DB) error {
		schedules, err := Load(tx)
		if err != nil {
			return err
		}

		for _, s := range schedules {
			i := slices.IndexFunc(defaults, func(d Schedule) bool { return d.Name == s.Name })
			if i < 0 {
				continue
			}

			var fixed []FixedIntention
			for _, f := range defaults[i].Fixed {
				if s.HasMass(f.Weekday, f.Time) {
					fixed = append(fixed, f)
				}
			}

			err = insertFixed(tx, s.ID, fixed)
			if err != nil {
				return err
			}
		}

		return nil
	})
}

func insertSchedule(tx *gorm.DB, name string, position int) (int, error) {
	row := scheduleRow{Name: name, Position: position}
	err := tx.Create(&row).Error

	return row.ID, err
}

func insertMasses(tx *gorm.DB, scheduleID int, masses [7][]string) error {
	var rows []massRow
	for weekday, times := range masses {
		for _, t := range times {
			rows = append(rows, massRow{ScheduleID: scheduleID, Weekday: weekday, Time: t})
		}
	}
	if len(rows) == 0 {
		return nil
	}

	return tx.Create(&rows).Error
}

func nextPosition(tx *gorm.DB) (int, error) {
	var last int
	err := tx.Model(&scheduleRow{}).Select("COALESCE(MAX(position), 0)").Scan(&last).Error

	return last + 1, err
}

func withMasses(db *gorm.DB) *gorm.DB {
	return db.
		Preload("Masses", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("weekday, time")
		}).
		Preload("Fixed", func(tx *gorm.DB) *gorm.DB {
			return tx.Order("weekday, time, id")
		})
}

func (row scheduleRow) schedule() Schedule {
	s := Schedule{ID: row.ID, Name: row.Name}
	for _, mass := range row.Masses {
		s.Masses[mass.Weekday] = append(s.Masses[mass.Weekday], mass.Time)
	}
	if len(row.Fixed) > 0 {
		s.Fixed = row.Fixed
	}

	return s
}
