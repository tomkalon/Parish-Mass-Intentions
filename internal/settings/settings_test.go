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

	if setting.SelectedScheduleID != 0 {
		t.Errorf("SelectedScheduleID = %d, want 0", setting.SelectedScheduleID)
	}
}

func TestSaveScheduleAndLoad(t *testing.T) {
	db := newTestDB(t)

	err := SaveSchedule(db, 2)
	if err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}

	setting, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if setting.SelectedScheduleID != 2 {
		t.Errorf("SelectedScheduleID = %d, want 2", setting.SelectedScheduleID)
	}
}

func TestSaveScheduleTwiceKeepsOneRow(t *testing.T) {
	db := newTestDB(t)

	for _, id := range []int{2, 1} {
		err := SaveSchedule(db, id)
		if err != nil {
			t.Fatalf("SaveSchedule(%d): %v", id, err)
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
	if setting.SelectedScheduleID != 1 {
		t.Errorf("SelectedScheduleID = %d, want the last saved 1", setting.SelectedScheduleID)
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
	err = SaveSchedule(db, 2)
	if err != nil {
		t.Fatalf("SaveSchedule: %v", err)
	}

	got, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := parish
	want.ID = settingID
	want.SelectedScheduleID = 2
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveParishKeepsSchedule(t *testing.T) {
	db := newTestDB(t)

	err := SaveSchedule(db, 2)
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
	if got.SelectedScheduleID != 2 {
		t.Errorf("SelectedScheduleID = %d, want 2 (SaveParish must not touch it)", got.SelectedScheduleID)
	}
	if want := (FeastImport{SkipMemorials: true}); got.FeastImport != want {
		t.Errorf("FeastImport = %+v, want %+v (unchecked again → false saved)", got.FeastImport, want)
	}
}
