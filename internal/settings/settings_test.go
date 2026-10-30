package settings

import (
	"path/filepath"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&Setting{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func TestLoadFromEmptyDatabase(t *testing.T) {
	setting, err := Load(newTestDB(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if setting.SelectedSchedule != "" {
		t.Errorf("SelectedSchedule = %q, want empty", setting.SelectedSchedule)
	}
}

func TestSaveScheduleAndLoad(t *testing.T) {
	db := newTestDB(t)

	err := SaveSchedule(db, "Wakacje")
	if err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}

	setting, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if setting.SelectedSchedule != "Wakacje" {
		t.Errorf("SelectedSchedule = %q, want %q", setting.SelectedSchedule, "Wakacje")
	}
}

func TestSaveScheduleTwiceKeepsOneRow(t *testing.T) {
	db := newTestDB(t)

	for _, name := range []string{"Wakacje", "Porządek zwykły"} {
		err := SaveSchedule(db, name)
		if err != nil {
			t.Fatalf("SaveSchedule(%q): %v", name, err)
		}
	}

	var count int64
	err := db.Model(&Setting{}).Count(&count).Error
	if err != nil {
		t.Fatalf("counting settings: %v", err)
	}
	if count != 1 {
		t.Errorf("rows = %d, want 1", count)
	}

	setting, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if setting.SelectedSchedule != "Porządek zwykły" {
		t.Errorf("SelectedSchedule = %q, want the last saved %q", setting.SelectedSchedule, "Porządek zwykły")
	}
}

func TestLoadDatabaseError(t *testing.T) {
	db := newTestDB(t)

	err := db.Migrator().DropTable(&Setting{})
	if err != nil {
		t.Fatalf("dropping settings table: %v", err)
	}

	_, err = Load(db)
	if err == nil {
		t.Error("Load error = nil, want an error without the settings table")
	}
}
