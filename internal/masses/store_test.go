package masses

import (
	"errors"
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

	err = Migrate(db)
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func load(t *testing.T, db *gorm.DB) []Schedule {
	t.Helper()

	schedules, err := Load(db)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	return schedules
}

func names(schedules []Schedule) []string {
	var result []string
	for _, s := range schedules {
		result = append(result, s.Name)
	}

	return result
}

func sameMasses(a, b [7][]string) bool {
	for day := range a {
		if !slices.Equal(a[day], b[day]) {
			return false
		}
	}

	return true
}

func countMasses(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	err := db.Model(&massRow{}).Count(&count).Error
	if err != nil {
		t.Fatalf("counting masses: %v", err)
	}

	return count
}

func TestSeedDefaults(t *testing.T) {
	db := newTestDB(t)

	err := SeedDefaults(db)
	if err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	schedules := load(t, db)
	defaults, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if got, want := names(schedules), names(defaults); !slices.Equal(got, want) {
		t.Fatalf("names = %q, want %q (file order)", got, want)
	}
	for i := range defaults {
		if schedules[i].ID == 0 {
			t.Errorf("%s: ID = 0, want a database id", schedules[i].Name)
		}
		if !sameMasses(schedules[i].Masses, defaults[i].Masses) {
			t.Errorf("%s: masses = %q, want %q", schedules[i].Name, schedules[i].Masses, defaults[i].Masses)
		}
	}
}

func TestSeedDefaultsKeepsExistingSchedules(t *testing.T) {
	db := newTestDB(t)
	_, err := Save(db, Schedule{Name: "Własny", Masses: [7][]string{time.Sunday: {"09:00"}}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = SeedDefaults(db)
	if err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	if got := names(load(t, db)); !slices.Equal(got, []string{"Własny"}) {
		t.Errorf("names = %q, want only the existing schedule", got)
	}
}

func TestSaveCreatesAndUpdates(t *testing.T) {
	db := newTestDB(t)

	first, err := Save(db, Schedule{Name: "Zimowy", Masses: [7][]string{time.Monday: {"07:00"}, time.Sunday: {"08:00", "10:00"}}})
	if err != nil {
		t.Fatalf("Save new: %v", err)
	}
	second, err := Save(db, Schedule{Name: "Letni", Masses: [7][]string{time.Sunday: {"09:00"}}})
	if err != nil {
		t.Fatalf("Save second: %v", err)
	}

	_, err = Save(db, Schedule{ID: first, Name: "Zimowy (od 1.11)", Masses: [7][]string{time.Sunday: {"07:30"}}})
	if err != nil {
		t.Fatalf("Save update: %v", err)
	}

	schedules := load(t, db)
	if got := names(schedules); !slices.Equal(got, []string{"Zimowy (od 1.11)", "Letni"}) {
		t.Fatalf("names = %q, want the order of creation", got)
	}
	if schedules[0].ID != first || schedules[1].ID != second {
		t.Errorf("ids = %d, %d, want %d, %d", schedules[0].ID, schedules[1].ID, first, second)
	}
	if want := ([7][]string{time.Sunday: {"07:30"}}); !sameMasses(schedules[0].Masses, want) {
		t.Errorf("updated masses = %q, want %q (old ones replaced)", schedules[0].Masses, want)
	}
	if got := countMasses(t, db); got != 2 {
		t.Errorf("mass rows = %d, want 2 (no leftovers after the update)", got)
	}
}

func TestSaveDuplicateName(t *testing.T) {
	db := newTestDB(t)
	id, err := Save(db, Schedule{Name: "Zimowy"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, err = Save(db, Schedule{Name: "Letni"})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, err = Save(db, Schedule{Name: "Zimowy"})
	if !errors.Is(err, ErrDuplicateName) {
		t.Errorf("new with a taken name: error = %v, want ErrDuplicateName", err)
	}
	_, err = Save(db, Schedule{ID: id, Name: "Letni"})
	if !errors.Is(err, ErrDuplicateName) {
		t.Errorf("rename to a taken name: error = %v, want ErrDuplicateName", err)
	}
	_, err = Save(db, Schedule{ID: id, Name: "Zimowy", Masses: [7][]string{time.Sunday: {"10:00"}}})
	if err != nil {
		t.Errorf("saving under its own name: %v", err)
	}
}

func TestSaveUnknownSchedule(t *testing.T) {
	_, err := Save(newTestDB(t), Schedule{ID: 99, Name: "Zimowy"})

	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("error = %v, want ErrScheduleNotFound", err)
	}
}

func TestFind(t *testing.T) {
	db := newTestDB(t)
	id, err := Save(db, Schedule{Name: "Zimowy", Masses: [7][]string{time.Sunday: {"10:00", "08:00"}}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Find(db, id)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	want := Schedule{ID: id, Name: "Zimowy", Masses: [7][]string{time.Sunday: {"08:00", "10:00"}}}
	if got.ID != want.ID || got.Name != want.Name || !sameMasses(got.Masses, want.Masses) {
		t.Errorf("Find = %+v, want %+v", got, want)
	}

	_, err = Find(db, id+1)
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("Find(missing) error = %v, want ErrScheduleNotFound", err)
	}
}

func TestDelete(t *testing.T) {
	db := newTestDB(t)
	id, err := Save(db, Schedule{Name: "Zimowy", Masses: [7][]string{time.Sunday: {"08:00", "10:00"}}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	_, err = Save(db, Schedule{Name: "Letni", Masses: [7][]string{time.Sunday: {"09:00"}}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = Delete(db, id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if got := names(load(t, db)); !slices.Equal(got, []string{"Letni"}) {
		t.Errorf("names = %q, want [Letni]", got)
	}
	if got := countMasses(t, db); got != 1 {
		t.Errorf("mass rows = %d, want 1 (masses of the deleted schedule removed too)", got)
	}

	err = Delete(db, id)
	if !errors.Is(err, ErrScheduleNotFound) {
		t.Errorf("second Delete error = %v, want ErrScheduleNotFound", err)
	}
}

func TestRestoreDefaults(t *testing.T) {
	db := newTestDB(t)
	_, err := Save(db, Schedule{Name: "Własny", Masses: [7][]string{time.Sunday: {"09:00"}}})
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	err = RestoreDefaults(db)
	if err != nil {
		t.Fatalf("RestoreDefaults: %v", err)
	}

	defaults, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}
	if got, want := names(load(t, db)), names(defaults); !slices.Equal(got, want) {
		t.Errorf("names = %q, want the defaults %q", got, want)
	}

	var wantMasses int64
	for _, s := range defaults {
		for _, times := range s.Masses {
			wantMasses += int64(len(times))
		}
	}
	if got := countMasses(t, db); got != wantMasses {
		t.Errorf("mass rows = %d, want %d (nothing left from the custom schedule)", got, wantMasses)
	}
}

func TestParseDay(t *testing.T) {
	tests := []struct {
		text string
		want []string
	}{
		{text: "6:30, 18:00", want: []string{"06:30", "18:00"}},
		{text: "18:00 6:30", want: []string{"06:30", "18:00"}},
		{text: "7:00;8:30,  10:00", want: []string{"07:00", "08:30", "10:00"}},
		{text: "  ", want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := ParseDay(tt.text)
			if err != nil {
				t.Fatalf("ParseDay: %v", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("ParseDay(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseDayErrors(t *testing.T) {
	tests := []struct {
		text       string
		wantReason error
		wantValue  string
	}{
		{text: "6:30, 6:5", wantReason: ErrInvalidTime, wantValue: "6:5"},
		{text: "osiemnasta", wantReason: ErrInvalidTime, wantValue: "osiemnasta"},
		{text: "6:10", wantReason: ErrNotQuarterHour, wantValue: "6:10"},
		{text: "18:00, 18:00", wantReason: ErrDuplicateTime, wantValue: "18:00"},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			_, err := ParseDay(tt.text)

			var timeErr *TimeError
			if !errors.As(err, &timeErr) {
				t.Fatalf("error = %v, want a *TimeError", err)
			}
			if !errors.Is(err, tt.wantReason) || timeErr.Value != tt.wantValue {
				t.Errorf("error = %v (value %q), want %v for %q", err, timeErr.Value, tt.wantReason, tt.wantValue)
			}
		})
	}
}
