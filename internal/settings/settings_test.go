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

func TestLoadDefaultParishName(t *testing.T) {
	setting, err := Load(newTestDB(t))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if setting.ParishName != DefaultParishName {
		t.Errorf("ParishName = %q, want the default %q", setting.ParishName, DefaultParishName)
	}
}

func TestSaveScheduleKeepsParish(t *testing.T) {
	db := newTestDB(t)
	parish := Setting{
		ParishName:  "Parafia św. Jana",
		Address:     "ul. Kościelna 1, 32-765 Rzezawa",
		FeastImport: FeastImport{SkipOtherDays: true},
	}

	err := SaveParish(db, parish)
	if err != nil {
		t.Fatalf("SaveParish: %v", err)
	}
	err = SaveSchedule(db, "Wakacje")
	if err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := parish
	want.ID = settingID
	want.SelectedSchedule = "Wakacje"
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveParishKeepsSchedule(t *testing.T) {
	db := newTestDB(t)

	err := SaveSchedule(db, "Wakacje")
	if err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}
	err = SaveParish(db, Setting{ParishName: "Parafia św. Jana", FeastImport: FeastImport{SkipFeasts: true, SkipMemorials: true}})
	if err != nil {
		t.Fatalf("SaveParish: %v", err)
	}
	err = SaveParish(db, Setting{ParishName: "Parafia św. Jana", FeastImport: FeastImport{SkipMemorials: true}})
	if err != nil {
		t.Fatalf("SaveParish: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got.SelectedSchedule != "Wakacje" {
		t.Errorf("SelectedSchedule = %q, want %q (SaveParish must not touch it)", got.SelectedSchedule, "Wakacje")
	}
	if want := (FeastImport{SkipMemorials: true}); got.FeastImport != want {
		t.Errorf("FeastImport = %+v, want %+v (unchecked again → false saved)", got.FeastImport, want)
	}
}
