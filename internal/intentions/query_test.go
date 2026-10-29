package intentions

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&Intention{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func createIntention(t *testing.T, db *gorm.DB, massAt, content string) {
	t.Helper()

	at, err := time.ParseInLocation(MassAtLayout, massAt, TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", massAt, err)
	}

	err = db.Create(&Intention{MassAt: at, Content: content}).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}
}

func contents(list []Intention) []string {
	result := make([]string, 0, len(list))
	for _, intention := range list {
		result = append(result, intention.Content)
	}

	return result
}

func TestForWeek(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-01 23:59", "sunday before")
	createIntention(t, db, "2026-11-02 18:00", "monday evening")
	createIntention(t, db, "2026-11-08 23:45", "sunday night")
	createIntention(t, db, "2026-11-02 00:00", "monday midnight")
	createIntention(t, db, "2026-11-09 00:00", "next monday")
	createIntention(t, db, "2026-11-02 18:00", "monday evening, added later")

	start := time.Date(2026, 11, 2, 0, 0, 0, 0, TimeZone)
	want := []string{"monday midnight", "monday evening", "monday evening, added later", "sunday night"}

	tests := []struct {
		name  string
		start time.Time
	}{
		{name: "start in Polish time", start: start},
		{name: "start in UTC", start: start.UTC()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := ForWeek(db, tt.start)
			if err != nil {
				t.Fatalf("ForWeek: %v", err)
			}

			if got := contents(list); !slices.Equal(got, want) {
				t.Errorf("ForWeek = %q, want %q", got, want)
			}
		})
	}
}

func TestForWeekAcrossDSTChange(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-10-19 00:00", "monday")
	createIntention(t, db, "2026-10-25 23:30", "sunday after the change")
	createIntention(t, db, "2026-10-26 00:00", "next monday")

	list, err := ForWeek(db, time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone))
	if err != nil {
		t.Fatalf("ForWeek: %v", err)
	}

	want := []string{"monday", "sunday after the change"}
	if got := contents(list); !slices.Equal(got, want) {
		t.Errorf("ForWeek = %q, want %q", got, want)
	}
}
