package web

import (
	"slices"
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestNewWeekView(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	got := newWeekView(start)

	if !got.Start.Equal(start) {
		t.Errorf("Start = %v, want %v", got.Start, start)
	}
	if want := "2.11.2026 r. – 8.11.2026 r."; got.Range != want {
		t.Errorf("Range = %q, want %q", got.Range, want)
	}
	if want := "2026-10-26"; got.Prev != want {
		t.Errorf("Prev = %q, want %q", got.Prev, want)
	}
	if want := "2026-11-09"; got.Next != want {
		t.Errorf("Next = %q, want %q", got.Next, want)
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
	if !slices.Equal(got.Days, wantDays) {
		t.Errorf("Days = %v, want %v", got.Days, wantDays)
	}
}

func TestNewWeekViewAcrossDSTChange(t *testing.T) {
	start := time.Date(2026, 10, 19, 0, 0, 0, 0, intentions.TimeZone)

	got := newWeekView(start)

	if len(got.Days) != 7 {
		t.Fatalf("len(Days) = %d, want 7", len(got.Days))
	}
	if want := "2026-10-25"; got.Days[6].Date != want {
		t.Errorf("last day = %q, want %q", got.Days[6].Date, want)
	}
	if want := "2026-10-26"; got.Next != want {
		t.Errorf("Next = %q, want %q", got.Next, want)
	}
}
