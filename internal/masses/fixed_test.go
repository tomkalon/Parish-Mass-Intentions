package masses

import (
	"errors"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"
)

var weekdaySchedule = Schedule{
	Name: "Dni powszednie",
	Masses: [7][]string{
		time.Wednesday: {"06:30", "18:00"},
		time.Friday:    {"06:30", "18:00"},
		time.Sunday:    {"07:00", "10:00"},
	},
}

func saveSchedule(t *testing.T, db *gorm.DB, s Schedule) int {
	t.Helper()

	id, err := Save(db, s)
	if err != nil {
		t.Fatalf("Save %q: %v", s.Name, err)
	}

	return id
}

func saveFixed(t *testing.T, db *gorm.DB, f FixedIntention) FixedIntention {
	t.Helper()

	err := SaveFixed(db, &f)
	if err != nil {
		t.Fatalf("SaveFixed %+v: %v", f, err)
	}

	return f
}

func find(t *testing.T, db *gorm.DB, id int) Schedule {
	t.Helper()

	s, err := Find(db, id)
	if err != nil {
		t.Fatalf("Find %d: %v", id, err)
	}

	return s
}

func contents(list []FixedIntention) []string {
	var result []string
	for _, f := range list {
		result = append(result, f.Content)
	}

	return result
}

func TestSaveFixedCreatesAndUpdates(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)

	novena := saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Friday, Time: "18:00", Content: "Intencje zbiorowe za zmarłych", IsDeceased: true, IsHidden: true})
	if novena.ID == 0 {
		t.Fatal("ID = 0 after create")
	}

	novena.Content = "Nowenna do MB Nieustającej Pomocy"
	novena.Time = "06:30"
	novena.IsOutsideParish = true
	saveFixed(t, db, novena)

	got := find(t, db, id).Fixed
	if len(got) != 2 {
		t.Fatalf("fixed = %+v, want 2", got)
	}
	want := FixedIntention{ID: novena.ID, ScheduleID: id, Weekday: time.Wednesday, Time: "06:30", Content: "Nowenna do MB Nieustającej Pomocy", IsOutsideParish: true}
	if got[0] != want {
		t.Errorf("updated = %+v, want %+v", got[0], want)
	}
	if !got[1].IsDeceased || !got[1].IsHidden || got[1].IsOutsideParish {
		t.Errorf("friday flags = %+v", got[1])
	}

	got[1].IsDeceased, got[1].IsHidden = false, false
	saveFixed(t, db, got[1])
	if friday := find(t, db, id).Fixed[1]; friday.IsDeceased || friday.IsHidden {
		t.Errorf("flags not cleared: %+v", friday)
	}
}

func TestSaveFixedErrors(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	other := saveSchedule(t, db, Schedule{Name: "Inny", Masses: weekdaySchedule.Masses})
	novena := saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"})

	tests := []struct {
		name  string
		fixed FixedIntention
		want  error
	}{
		{"mass not in the schedule", FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "19:00", Content: "x"}, ErrNoSuchMass},
		{"day without masses", FixedIntention{ScheduleID: id, Weekday: time.Monday, Time: "18:00", Content: "x"}, ErrNoSuchMass},
		{"weekday out of range", FixedIntention{ScheduleID: id, Weekday: 9, Time: "18:00", Content: "x"}, ErrNoSuchMass},
		{"unknown schedule", FixedIntention{ScheduleID: 99, Weekday: time.Wednesday, Time: "18:00", Content: "x"}, ErrScheduleNotFound},
		{"unknown id", FixedIntention{ID: 99, ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "x"}, ErrFixedNotFound},
		{"id of another schedule", FixedIntention{ID: novena.ID, ScheduleID: other, Weekday: time.Wednesday, Time: "18:00", Content: "x"}, ErrFixedNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := SaveFixed(db, &tt.fixed)
			if !errors.Is(err, tt.want) {
				t.Errorf("err = %v, want %v", err, tt.want)
			}
		})
	}

	if got := find(t, db, id).Fixed; len(got) != 1 || got[0].Content != "Intencje nowennowe" {
		t.Errorf("fixed = %+v, want only the untouched novena", got)
	}
	if got := find(t, db, other).Fixed; got != nil {
		t.Errorf("other schedule fixed = %+v, want none", got)
	}
}

func TestFindAndDeleteFixed(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	other := saveSchedule(t, db, Schedule{Name: "Inny", Masses: weekdaySchedule.Masses})
	novena := saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"})

	got, err := FindFixed(db, id, novena.ID)
	if err != nil || got != novena {
		t.Errorf("FindFixed = %+v, %v, want %+v", got, err, novena)
	}
	if _, err := FindFixed(db, other, novena.ID); !errors.Is(err, ErrFixedNotFound) {
		t.Errorf("FindFixed in another schedule: err = %v, want ErrFixedNotFound", err)
	}

	if err := DeleteFixed(db, other, novena.ID); !errors.Is(err, ErrFixedNotFound) {
		t.Errorf("DeleteFixed in another schedule: err = %v, want ErrFixedNotFound", err)
	}
	if err := DeleteFixed(db, id, novena.ID); err != nil {
		t.Fatalf("DeleteFixed: %v", err)
	}
	if err := DeleteFixed(db, id, novena.ID); !errors.Is(err, ErrFixedNotFound) {
		t.Errorf("second DeleteFixed: err = %v, want ErrFixedNotFound", err)
	}
	if got := find(t, db, id).Fixed; got != nil {
		t.Errorf("fixed = %+v, want none", got)
	}
}

func TestLoadReturnsFixedInOrder(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	empty := saveSchedule(t, db, Schedule{Name: "Bez stałych", Masses: weekdaySchedule.Masses})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Friday, Time: "18:00", Content: "piątek"})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "środa 18 – pierwsza"})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "06:30", Content: "środa rano"})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "środa 18 – druga"})

	schedules := load(t, db)
	want := []string{"środa rano", "środa 18 – pierwsza", "środa 18 – druga", "piątek"}
	if got := contents(schedules[0].Fixed); !slices.Equal(got, want) {
		t.Errorf("fixed = %q, want %q", got, want)
	}
	if schedules[1].ID != empty || schedules[1].Fixed != nil {
		t.Errorf("schedule without fixed intentions = %+v", schedules[1])
	}
	if got := contents(schedules[0].FixedAt(time.Wednesday, "18:00")); !slices.Equal(got, want[1:3]) {
		t.Errorf("FixedAt = %q, want %q", got, want[1:3])
	}
}

func TestSaveScheduleDeletesOrphanedFixed(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Friday, Time: "18:00", Content: "Intencje zbiorowe za zmarłych"})
	other := saveSchedule(t, db, Schedule{Name: "Inny", Masses: weekdaySchedule.Masses})
	saveFixed(t, db, FixedIntention{ScheduleID: other, Weekday: time.Wednesday, Time: "18:00", Content: "Inna nowenna"})

	changed := weekdaySchedule
	changed.ID = id
	changed.Masses[time.Wednesday] = []string{"06:30", "18:30"}
	saveSchedule(t, db, changed)

	if got := contents(find(t, db, id).Fixed); !slices.Equal(got, []string{"Intencje zbiorowe za zmarłych"}) {
		t.Errorf("fixed = %q, want only the Friday one", got)
	}
	if got := contents(find(t, db, other).Fixed); !slices.Equal(got, []string{"Inna nowenna"}) {
		t.Errorf("other schedule fixed = %q, want it untouched", got)
	}
}

func TestDeleteScheduleDeletesFixed(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	other := saveSchedule(t, db, Schedule{Name: "Inny", Masses: weekdaySchedule.Masses})
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"})
	saveFixed(t, db, FixedIntention{ScheduleID: other, Weekday: time.Wednesday, Time: "18:00", Content: "Inna nowenna"})

	err := Delete(db, id)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}

	var left []FixedIntention
	err = db.Find(&left).Error
	if err != nil {
		t.Fatal(err)
	}
	if got := contents(left); !slices.Equal(got, []string{"Inna nowenna"}) {
		t.Errorf("fixed left = %q, want only the other schedule's", got)
	}
}

func TestRestoreDefaultsReplacesFixed(t *testing.T) {
	db := newTestDB(t)
	id := saveSchedule(t, db, weekdaySchedule)
	saveFixed(t, db, FixedIntention{ScheduleID: id, Weekday: time.Wednesday, Time: "06:30", Content: "Własna"})

	err := RestoreDefaults(db)
	if err != nil {
		t.Fatalf("RestoreDefaults: %v", err)
	}

	for _, s := range load(t, db) {
		want := []string{"Intencje nowennowe", "Intencje zbiorowe za zmarłych"}
		if got := contents(s.Fixed); !slices.Equal(got, want) {
			t.Errorf("%s: fixed = %q, want the defaults %q", s.Name, got, want)
		}
	}
}

func TestDefaultFixedIntentions(t *testing.T) {
	defaults, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	want := []FixedIntention{
		{Weekday: time.Wednesday, Time: "18:00", Content: "Intencje nowennowe"},
		{Weekday: time.Friday, Time: "18:00", Content: "Intencje zbiorowe za zmarłych", IsDeceased: true},
	}
	for _, s := range defaults {
		if !slices.Equal(s.Fixed, want) {
			t.Errorf("%s: fixed = %+v, want %+v", s.Name, s.Fixed, want)
		}
	}
}

func TestSeedDefaultsAddsFixed(t *testing.T) {
	db := newTestDB(t)

	err := SeedDefaults(db)
	if err != nil {
		t.Fatalf("SeedDefaults: %v", err)
	}

	for _, s := range load(t, db) {
		if len(s.Fixed) != 2 || s.Fixed[0].ScheduleID != s.ID || s.Fixed[0].ID == 0 {
			t.Errorf("%s: fixed = %+v, want the two defaults stored for this schedule", s.Name, s.Fixed)
		}
	}
}

func TestMigrateAddsDefaultFixedOnce(t *testing.T) {
	db := newTestDB(t)
	err := db.Migrator().DropTable(&FixedIntention{})
	if err != nil {
		t.Fatal(err)
	}
	oldSchedule := func(position int, s Schedule) int {
		id, err := insertSchedule(db, s.Name, position)
		if err != nil {
			t.Fatal(err)
		}
		err = insertMasses(db, id, s.Masses)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	ordinary := oldSchedule(1, Schedule{Name: "Porządek zwykły", Masses: [7][]string{time.Wednesday: {"18:00"}, time.Friday: {"18:00"}}})
	summer := oldSchedule(2, Schedule{Name: "Wakacje", Masses: [7][]string{time.Wednesday: {"19:00"}, time.Friday: {"18:00"}}})
	own := oldSchedule(3, Schedule{Name: "Własny", Masses: [7][]string{time.Wednesday: {"18:00"}, time.Friday: {"18:00"}}})

	err = Migrate(db)
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if got := contents(find(t, db, ordinary).Fixed); !slices.Equal(got, []string{"Intencje nowennowe", "Intencje zbiorowe za zmarłych"}) {
		t.Errorf("Porządek zwykły: fixed = %q, want both defaults", got)
	}
	if got := contents(find(t, db, summer).Fixed); !slices.Equal(got, []string{"Intencje zbiorowe za zmarłych"}) {
		t.Errorf("Wakacje: fixed = %q, want only Friday (no Wednesday 18:00)", got)
	}
	if got := find(t, db, own).Fixed; got != nil {
		t.Errorf("own schedule: fixed = %+v, want none (not a default schedule)", got)
	}

	first := find(t, db, ordinary).Fixed[0]
	err = DeleteFixed(db, ordinary, first.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = Migrate(db)
	if err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	if got := contents(find(t, db, ordinary).Fixed); !slices.Equal(got, []string{"Intencje zbiorowe za zmarłych"}) {
		t.Errorf("after deleting and migrating again: fixed = %q, want the deleted one to stay deleted", got)
	}
}

func TestOrphans(t *testing.T) {
	fixed := []FixedIntention{
		{ID: 1, Weekday: time.Wednesday, Time: "18:00"},
		{ID: 2, Weekday: time.Friday, Time: "18:00"},
		{ID: 3, Weekday: time.Sunday, Time: "10:00"},
	}
	masses := [7][]string{time.Wednesday: {"18:30"}, time.Friday: {"06:30", "18:00"}}

	got := Orphans(fixed, masses)

	var ids []int
	for _, f := range got {
		ids = append(ids, f.ID)
	}
	if !slices.Equal(ids, []int{1, 3}) {
		t.Errorf("orphans = %v, want [1 3]", ids)
	}
	if Orphans(fixed[1:2], masses) != nil {
		t.Error("a fixed intention with its mass reported as orphan")
	}
}
