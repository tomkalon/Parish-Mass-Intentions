package web

import (
	"slices"
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestNewWeekView(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	got := newWeekView(start, nil)

	if !got.Start.Equal(start) {
		t.Errorf("Start = %v, want %v", got.Start, start)
	}
	if want := "2.11.2026 r. – 8.11.2026 r."; got.Range != want {
		t.Errorf("Range = %q, want %q", got.Range, want)
	}
	if want := "/?week=2026-10-26"; got.PrevURL() != want {
		t.Errorf("PrevURL() = %q, want %q", got.PrevURL(), want)
	}
	if want := "/?week=2026-11-09"; got.NextURL() != want {
		t.Errorf("NextURL() = %q, want %q", got.NextURL(), want)
	}

	wantDays := []dayView{
		{Date: "2026-11-02", Label: "Poniedziałek 2.11."},
		{Date: "2026-11-03", Label: "Wtorek 3.11."},
		{Date: "2026-11-04", Label: "Środa 4.11."},
		{Date: "2026-11-05", Label: "Czwartek 5.11."},
		{Date: "2026-11-06", Label: "Piątek 6.11."},
		{Date: "2026-11-07", Label: "Sobota 7.11."},
		{Date: "2026-11-08", Label: "Niedziela 8.11."},
	}
	if len(got.Days) != len(wantDays) {
		t.Fatalf("len(Days) = %d, want %d", len(got.Days), len(wantDays))
	}
	for i, want := range wantDays {
		if got.Days[i].Date != want.Date || got.Days[i].Label != want.Label {
			t.Errorf("Days[%d] = {%q, %q}, want {%q, %q}", i, got.Days[i].Date, got.Days[i].Label, want.Date, want.Label)
		}
	}
}

func TestNewWeekViewAcrossDSTChange(t *testing.T) {
	start := time.Date(2026, 10, 19, 0, 0, 0, 0, intentions.TimeZone)

	got := newWeekView(start, nil)

	if len(got.Days) != 7 {
		t.Fatalf("len(Days) = %d, want 7", len(got.Days))
	}
	if want := "2026-10-25"; got.Days[6].Date != want {
		t.Errorf("last day = %q, want %q", got.Days[6].Date, want)
	}
	if want := "/?week=2026-10-26"; got.NextURL() != want {
		t.Errorf("NextURL() = %q, want %q", got.NextURL(), want)
	}
}

func TestNewWeekViewGroupsIntentions(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	list := []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, "A"),
		newIntention(t, 2, "2026-11-02 06:30", false, "B"),
		newIntention(t, 3, "2026-11-02 06:30", true, "C"),
		newIntention(t, 4, "2026-11-02 18:00", true, "D"),
		newIntention(t, 5, "2026-11-04 07:00", false, "E"),
		newIntention(t, 6, "2026-11-09 07:00", false, "next week"),
	}

	got := newWeekView(start, list)

	if len(got.Days) != 7 {
		t.Fatalf("len(Days) = %d, want 7", len(got.Days))
	}

	monday := got.Days[0].Masses
	if len(monday) != 2 {
		t.Fatalf("monday has %d masses, want 2", len(monday))
	}
	checkMass(t, monday[0], massView{
		Time:    "6:30",
		Parish:  []intentionView{{ID: 1, Number: 1, Content: "A"}, {ID: 2, Number: 2, Content: "B"}},
		Outside: []intentionView{{ID: 3, Number: 1, Content: "C"}},
	})
	checkMass(t, monday[1], massView{
		Time:    "18:00",
		Outside: []intentionView{{ID: 4, Number: 1, Content: "D"}},
	})

	if n := len(got.Days[1].Masses); n != 0 {
		t.Errorf("tuesday has %d masses, want 0", n)
	}

	wednesday := got.Days[2].Masses
	if len(wednesday) != 1 {
		t.Fatalf("wednesday has %d masses, want 1", len(wednesday))
	}
	checkMass(t, wednesday[0], massView{
		Time:   "7:00",
		Parish: []intentionView{{ID: 5, Number: 1, Content: "E"}},
	})

	if n := len(got.Days[6].Masses); n != 0 {
		t.Errorf("sunday has %d masses, want 0 (intention from next week must be ignored)", n)
	}
}

func TestNewWeekViewUsesPolishTimeZone(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	list := []intentions.Intention{
		{ID: 1, MassAt: time.Date(2026, 11, 2, 23, 30, 0, 0, time.UTC), Content: "A"},
	}

	got := newWeekView(start, list)

	if n := len(got.Days[0].Masses); n != 0 {
		t.Errorf("monday has %d masses, want 0", n)
	}

	tuesday := got.Days[1].Masses
	if len(tuesday) != 1 {
		t.Fatalf("tuesday has %d masses, want 1", len(tuesday))
	}
	if want := "0:30"; tuesday[0].Time != want {
		t.Errorf("mass time = %q, want %q", tuesday[0].Time, want)
	}
}

func newIntention(t *testing.T, id int, massAt string, outside bool, content string) intentions.Intention {
	t.Helper()

	at, err := time.ParseInLocation(intentions.MassAtLayout, massAt, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", massAt, err)
	}

	return intentions.Intention{ID: id, MassAt: at, Content: content, IsOutsideParish: outside}
}

func checkMass(t *testing.T, got, want massView) {
	t.Helper()

	if got.Time != want.Time {
		t.Errorf("mass time = %q, want %q", got.Time, want.Time)
	}
	if !slices.EqualFunc(got.Parish, want.Parish, sameIntention) {
		t.Errorf("%s parish = %v, want %v", want.Time, got.Parish, want.Parish)
	}
	if !slices.EqualFunc(got.Outside, want.Outside, sameIntention) {
		t.Errorf("%s outside = %v, want %v", want.Time, got.Outside, want.Outside)
	}
}

func sameIntention(a, b intentionView) bool {
	return a.ID == b.ID && a.Number == b.Number && a.Content == b.Content
}

func TestMarkSaved(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, "A"),
		newIntention(t, 2, "2026-11-02 06:30", true, "B"),
	})

	markSaved(view.Days, 2)

	mass := view.Days[0].Masses[0]
	if mass.Parish[0].IsSaved {
		t.Error("intention 1 is saved, want only 2")
	}
	if !mass.Outside[0].IsSaved {
		t.Error("intention 2 is not saved")
	}
}

func TestMarkEditing(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, "A"),
		newIntention(t, 2, "2026-11-02 06:30", true, "B"),
	})

	markEditing(view.Days, 1)

	mass := view.Days[0].Masses[0]
	if !mass.Parish[0].IsEditing {
		t.Error("intention 1 is not marked as edited")
	}
	if mass.Outside[0].IsEditing {
		t.Error("intention 2 is marked as edited, want only 1")
	}
}
