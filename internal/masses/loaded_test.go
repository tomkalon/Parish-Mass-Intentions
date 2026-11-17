package masses

import (
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func newRangeDB(t *testing.T) *gorm.DB {
	t.Helper()

	db := newTestDB(t)
	err := db.AutoMigrate(&intentions.Intention{})
	if err != nil {
		t.Fatal(err)
	}

	return db
}

func day(text string) time.Time {
	at, err := time.ParseInLocation(intentions.DateLayout, text, intentions.TimeZone)
	if err != nil {
		panic(err)
	}

	return at
}

func addAt(t *testing.T, db *gorm.DB, intention intentions.Intention, at string) {
	t.Helper()

	var err error
	intention.MassAt, err = time.ParseInLocation(intentions.MassAtLayout, at, intentions.TimeZone)
	if err != nil {
		t.Fatal(err)
	}
	err = db.Create(&intention).Error
	if err != nil {
		t.Fatal(err)
	}
}

func describe(masses []Mass) []string {
	var out []string
	for _, m := range masses {
		text := fmt.Sprintf("%s p%d o%d", m.At.Format("02.01 15:04"), m.Parish, m.Outside)
		if m.Gregorian {
			text += " greg"
		}
		if m.Planned {
			text += " planned"
		}
		out = append(out, text)
	}

	return out
}

func checkMasses(t *testing.T, got []Mass, want ...string) {
	t.Helper()

	lines := describe(got)
	if fmt.Sprint(lines) != fmt.Sprint(want) {
		t.Errorf("masses:\n got %q\nwant %q", lines, want)
	}
}

var testPlan = &Schedule{ID: 1, Name: "Zwykły", Masses: [7][]string{
	time.Monday:  {"06:30", "18:00"},
	time.Tuesday: {"18:00"},
	time.Sunday:  {"07:00", "10:00"},
}}

func TestMarkLoadedAndUnmark(t *testing.T) {
	db := newRangeDB(t)

	for _, id := range []int{1, 2} {
		err := MarkLoaded(db, day("2026-12-02"), id)
		if err != nil {
			t.Fatalf("MarkLoaded(%d): %v", id, err)
		}
	}

	loaded, err := loadedWeeks(db, day("2026-11-30"), day("2026-12-07"))
	if err != nil {
		t.Fatal(err)
	}
	if !loaded["2026-11-30"] || len(loaded) != 1 {
		t.Fatalf("loaded weeks = %v, want only 2026-11-30", loaded)
	}

	var row loadedWeek
	err = db.First(&row).Error
	if err != nil || row.ScheduleID != 2 {
		t.Errorf("stored row = %+v, %v; want schedule 2", row, err)
	}

	err = Unmark(db, day("2026-12-06"))
	if err != nil {
		t.Fatalf("Unmark: %v", err)
	}
	loaded, err = loadedWeeks(db, day("2026-11-30"), day("2026-12-07"))
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 0 {
		t.Errorf("loaded weeks after Unmark = %v", loaded)
	}
}

func TestInRangeCountsParishAndOutsideIntentions(t *testing.T) {
	db := newRangeDB(t)
	addAt(t, db, intentions.Intention{Content: "a"}, "2026-12-01 06:30")
	addAt(t, db, intentions.Intention{Content: "b", IsHidden: true}, "2026-12-01 06:30")
	addAt(t, db, intentions.Intention{Content: "c", IsOutsideParish: true}, "2026-12-01 06:30")
	addAt(t, db, intentions.Intention{IsPlaceholder: true}, "2026-12-01 18:00")
	addAt(t, db, intentions.Intention{Content: "d", IsOutsideParish: true}, "2026-12-02 18:00")
	addAt(t, db, intentions.Intention{Content: "e", IsGregorian: true, IsDeceased: true}, "2026-12-03 18:00")
	addAt(t, db, intentions.Intention{Content: "f"}, "2026-12-08 06:30")

	got, err := InRange(db, day("2026-11-30"), day("2026-12-07"), nil)
	if err != nil {
		t.Fatal(err)
	}

	checkMasses(t, got,
		"01.12 06:30 p2 o1",
		"01.12 18:00 p0 o0",
		"02.12 18:00 p0 o1",
		"03.12 18:00 p1 o0 greg",
	)
	if !got[1].Free() || !got[2].Free() || got[0].Free() {
		t.Error("Free() should be true exactly for Masses without parish intentions")
	}
}

func TestInRangeAddsPlannedHoursOnlyForUnloadedWeeks(t *testing.T) {
	db := newRangeDB(t)

	err := MarkLoaded(db, day("2026-11-30"), 1)
	if err != nil {
		t.Fatal(err)
	}
	addAt(t, db, intentions.Intention{Content: "loaded"}, "2026-11-30 18:00")

	addAt(t, db, intentions.Intention{IsPlaceholder: true}, "2026-12-08 18:00")

	addAt(t, db, intentions.Intention{Content: "early"}, "2026-12-14 18:00")
	addAt(t, db, intentions.Intention{Content: "odd hour"}, "2026-12-20 12:00")

	got, err := InRange(db, day("2026-11-30"), day("2026-12-21"), testPlan)
	if err != nil {
		t.Fatal(err)
	}

	checkMasses(t, got,
		"30.11 18:00 p1 o0",
		"08.12 18:00 p0 o0",
		"14.12 06:30 p0 o0 planned",
		"14.12 18:00 p1 o0",
		"15.12 18:00 p0 o0 planned",
		"20.12 07:00 p0 o0 planned",
		"20.12 10:00 p0 o0 planned",
		"20.12 12:00 p1 o0",
	)
}

func TestInRangeWithoutPlanAddsNothing(t *testing.T) {
	db := newRangeDB(t)

	got, err := InRange(db, day("2026-11-30"), day("2026-12-21"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("InRange without a plan = %v", describe(got))
	}
}

func TestInRangeStartsAtFromTime(t *testing.T) {
	db := newRangeDB(t)
	from := time.Date(2026, 11, 30, 12, 0, 0, 0, intentions.TimeZone)

	got, err := InRange(db, from, day("2026-12-02"), testPlan)
	if err != nil {
		t.Fatal(err)
	}

	checkMasses(t, got,
		"30.11 18:00 p0 o0 planned",
		"01.12 18:00 p0 o0 planned",
	)
}

func TestInRangeAcrossDaylightSavingChange(t *testing.T) {
	db := newRangeDB(t)

	got, err := InRange(db, day("2027-03-27"), day("2027-03-30"), testPlan)
	if err != nil {
		t.Fatal(err)
	}

	checkMasses(t, got,
		"28.03 07:00 p0 o0 planned",
		"28.03 10:00 p0 o0 planned",
		"29.03 06:30 p0 o0 planned",
		"29.03 18:00 p0 o0 planned",
	)
}
