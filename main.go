package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
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

	schedules, err := masses.Default()
	if err != nil {
		log.Fatalf("error loading mass schedules: %v", err)
	}

	err = run(newRouter(db, schedules))
	if err != nil {
		log.Fatalf("error running application: %v", err)
	}
}

func openDatabase(path string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("error opening database %s: %w", path, err)
	}

	err = db.AutoMigrate(&intentions.Intention{})
	if err != nil {
		return nil, fmt.Errorf("error migrating database %s: %w", path, err)
	}

	return db, nil
}
