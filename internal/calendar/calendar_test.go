package calendar

import (
	"maps"
	"path/filepath"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&Day{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func saveFeast(t *testing.T, db *gorm.DB, date, feast string) {
	t.Helper()

	err := SaveFeast(db, date, feast)
	if err != nil {
		t.Fatalf("SaveFeast(%q, %q): %v", date, feast, err)
	}
}

func countDays(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	err := db.Model(&Day{}).Count(&count).Error
	if err != nil {
		t.Fatalf("counting days: %v", err)
	}

	return count
}

func TestFeastsForWeek(t *testing.T) {
	db := newTestDB(t)
	saveFeast(t, db, "2026-11-01", "Uroczystość Wszystkich Świętych")
	saveFeast(t, db, "2026-11-02", "Wspomnienie wszystkich wiernych zmarłych")
	saveFeast(t, db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	saveFeast(t, db, "2026-11-08", "XXXII Niedziela zwykła")
	saveFeast(t, db, "2026-11-09", "święto rocznicy poświęcenia Bazyliki Laterańskiej")

	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	want := map[string]string{
		"2026-11-02": "Wspomnienie wszystkich wiernych zmarłych",
		"2026-11-04": "wspomnienie św. Karola Boromeusza",
		"2026-11-08": "XXXII Niedziela zwykła",
	}

	tests := []struct {
		name  string
		start time.Time
	}{
		{name: "start in Polish time", start: start},
		{name: "start in UTC", start: start.UTC()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FeastsForWeek(db, tt.start)
			if err != nil {
				t.Fatalf("FeastsForWeek: %v", err)
			}

			if !maps.Equal(got, want) {
				t.Errorf("FeastsForWeek = %q, want %q", got, want)
			}
		})
	}
}

func TestFeastsForWeekWithoutEntries(t *testing.T) {
	got, err := FeastsForWeek(newTestDB(t), time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err != nil {
		t.Fatalf("FeastsForWeek: %v", err)
	}

	if len(got) != 0 {
		t.Errorf("FeastsForWeek = %q, want empty", got)
	}
	if feast := got["2026-11-03"]; feast != "" {
		t.Errorf("missing day = %q, want empty", feast)
	}
}

func TestSaveFeastReplacesText(t *testing.T) {
	db := newTestDB(t)
	saveFeast(t, db, "2026-11-04", "wspomnienie św. Karola")
	saveFeast(t, db, "2026-11-04", "wspomnienie św. Karola Boromeusza")

	if got := countDays(t, db); got != 1 {
		t.Errorf("rows = %d, want 1", got)
	}

	feasts, err := FeastsForWeek(db, time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err != nil {
		t.Fatalf("FeastsForWeek: %v", err)
	}
	if got, want := feasts["2026-11-04"], "wspomnienie św. Karola Boromeusza"; got != want {
		t.Errorf("feast = %q, want %q", got, want)
	}
}

func TestSaveEmptyFeastDeletesEntry(t *testing.T) {
	db := newTestDB(t)
	saveFeast(t, db, "2026-11-04", "wspomnienie św. Karola Boromeusza")
	saveFeast(t, db, "2026-11-05", "inny dzień")

	saveFeast(t, db, "2026-11-04", "")
	saveFeast(t, db, "2026-11-06", "")

	if got := countDays(t, db); got != 1 {
		t.Errorf("rows = %d, want 1 (only 2026-11-05 left)", got)
	}
}

func TestFeastsForWeekDatabaseError(t *testing.T) {
	db := newTestDB(t)

	err := db.Migrator().DropTable(&Day{})
	if err != nil {
		t.Fatalf("dropping days table: %v", err)
	}

	_, err = FeastsForWeek(db, time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone))
	if err == nil {
		t.Error("FeastsForWeek error = nil, want an error without the days table")
	}
}

func TestFeast(t *testing.T) {
	db := newTestDB(t)
	saveFeast(t, db, "2026-11-04", "wspomnienie św. Karola Boromeusza")

	tests := []struct {
		date string
		want string
	}{
		{date: "2026-11-04", want: "wspomnienie św. Karola Boromeusza"},
		{date: "2026-11-05", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.date, func(t *testing.T) {
			got, err := Feast(db, tt.date)
			if err != nil {
				t.Fatalf("Feast: %v", err)
			}
			if got != tt.want {
				t.Errorf("Feast(%q) = %q, want %q", tt.date, got, tt.want)
			}
		})
	}
}
