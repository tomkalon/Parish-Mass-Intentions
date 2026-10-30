package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/calendar"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

func main() {
	dir, err := prepareDataDir()
	if err != nil {
		log.Fatalf("error preparing data directory: %v", err)
	}

	db, err := openDatabase(filepath.Join(dir, "data.db"))
	if err != nil {
		log.Fatal(err)
	}

	err = run(newRouter(db), dir)
	if err != nil {
		log.Fatalf("error running application: %v", err)
	}
}

func openDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("error opening database %s: %w", path, err)
	}

	err = db.AutoMigrate(&intentions.Intention{}, &settings.Setting{}, &calendar.Day{})
	if err != nil {
		return nil, fmt.Errorf("error migrating database %s: %w", path, err)
	}

	err = masses.Migrate(db)
	if err != nil {
		return nil, fmt.Errorf("error migrating mass schedules in %s: %w", path, err)
	}

	err = masses.SeedDefaults(db)
	if err != nil {
		return nil, fmt.Errorf("error loading default mass schedules into %s: %w", path, err)
	}

	return db, nil
}
