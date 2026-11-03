package database

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go-parafia/internal/calendar"
	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

var ErrInvalidFile = errors.New("to nie jest plik z danymi Go Parafia")

func Open(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("error opening database %s: %w", path, err)
	}

	err = Migrate(db)
	if err != nil {
		return nil, fmt.Errorf("error migrating database %s: %w", path, err)
	}

	return db, nil
}

func Migrate(db *gorm.DB) error {
	err := db.AutoMigrate(&intentions.Intention{}, &settings.Setting{}, &calendar.Day{}, &gregorian.Series{})
	if err != nil {
		return err
	}

	err = masses.Migrate(db)
	if err != nil {
		return fmt.Errorf("mass schedules: %w", err)
	}

	err = masses.SeedDefaults(db)
	if err != nil {
		return fmt.Errorf("default mass schedules: %w", err)
	}

	return nil
}

func Export(db *gorm.DB, path string) error {
	tmp := path + ".tmp"
	err := os.Remove(tmp)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	err = db.Exec("VACUUM INTO ?", tmp).Error
	if err != nil {
		return fmt.Errorf("copying database: %w", err)
	}

	err = os.Rename(tmp, path)
	if err != nil {
		return errors.Join(err, os.Remove(tmp))
	}

	return nil
}

type Summary struct {
	Intentions int64
	First      time.Time
	Last       time.Time
	ParishName string
}

func PrepareImport(path string) (summary Summary, err error) {
	_, err = os.Stat(path)
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}

	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{Logger: logger.Discard})
	if err != nil {
		return Summary{}, fmt.Errorf("%w: %v", ErrInvalidFile, err)
	}
	defer func() {
		err = errors.Join(err, closeDB(db))
	}()

	var check string
	err = db.Raw("PRAGMA quick_check").Scan(&check).Error
	if err != nil || check != "ok" {
		return Summary{}, fmt.Errorf("%w: integrity check %q: %v", ErrInvalidFile, check, err)
	}
	if !db.Migrator().HasTable(&intentions.Intention{}) {
		return Summary{}, fmt.Errorf("%w: no intentions table", ErrInvalidFile)
	}

	err = Migrate(db)
	if err != nil {
		return Summary{}, fmt.Errorf("%w: migrating: %v", ErrInvalidFile, err)
	}

	return summarize(db)
}

func summarize(db *gorm.DB) (Summary, error) {
	var summary Summary

	err := db.Model(&intentions.Intention{}).Where("is_placeholder = ?", false).Count(&summary.Intentions).Error
	if err != nil {
		return Summary{}, err
	}

	if summary.Intentions > 0 {
		var first, last intentions.Intention
		err = db.Where("is_placeholder = ?", false).Order("mass_at").Limit(1).Find(&first).Error
		if err != nil {
			return Summary{}, err
		}
		err = db.Where("is_placeholder = ?", false).Order("mass_at DESC").Limit(1).Find(&last).Error
		if err != nil {
			return Summary{}, err
		}
		summary.First = first.MassAt.In(intentions.TimeZone)
		summary.Last = last.MassAt.In(intentions.TimeZone)
	}

	setting, err := settings.Load(db)
	if err != nil {
		return Summary{}, err
	}
	summary.ParishName = setting.ParishName

	return summary, nil
}

func Restore(db *gorm.DB, path, backupPath string) error {
	_, err := PrepareImport(path)
	if err != nil {
		return err
	}

	err = Export(db, backupPath)
	if err != nil {
		return fmt.Errorf("backing up current data: %w", err)
	}

	return db.Connection(func(conn *gorm.DB) error {
		err := conn.Exec("ATTACH DATABASE ? AS source", path).Error
		if err != nil {
			return fmt.Errorf("attaching %s: %w", path, err)
		}

		err = conn.Transaction(copyTables)
		detachErr := conn.Exec("DETACH DATABASE source").Error

		return errors.Join(err, detachErr)
	})
}

func copyTables(tx *gorm.DB) error {
	err := tx.Exec("PRAGMA defer_foreign_keys = ON").Error
	if err != nil {
		return err
	}

	var tables []string
	err = tx.Raw("SELECT name FROM main.sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name").
		Scan(&tables).Error
	if err != nil {
		return err
	}

	for _, table := range tables {
		var columns []string
		err = tx.Raw("SELECT name FROM pragma_table_info(?, 'main')", table).Scan(&columns).Error
		if err != nil {
			return err
		}

		list := quoteAll(columns)
		err = tx.Exec(fmt.Sprintf("DELETE FROM main.%s", quote(table))).Error
		if err != nil {
			return fmt.Errorf("clearing %s: %w", table, err)
		}
		err = tx.Exec(fmt.Sprintf("INSERT INTO main.%s (%s) SELECT %s FROM source.%s", quote(table), list, list, quote(table))).Error
		if err != nil {
			return fmt.Errorf("copying %s: %w", table, err)
		}
	}

	return nil
}

func quote(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func quoteAll(names []string) string {
	quoted := make([]string, 0, len(names))
	for _, name := range names {
		quoted = append(quoted, quote(name))
	}

	return strings.Join(quoted, ", ")
}

func closeDB(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}
