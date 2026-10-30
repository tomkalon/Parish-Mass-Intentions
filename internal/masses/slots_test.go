package masses

import (
	"slices"
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

var testSchedule = Schedule{
	Name: "Test",
	Masses: [7][]string{
		time.Monday: {"06:30", "18:00"},
		time.Sunday: {"07:00", "10:00"},
	},
}

func massAt(t *testing.T, value string) time.Time {
	t.Helper()

	at, err := time.ParseInLocation(intentions.MassAtLayout, value, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", value, err)
	}

	return at
}

func slotTimes(slots []intentions.Intention) []string {
	times := make([]string, 0, len(slots))
	for _, slot := range slots {
		times = append(times, slot.MassAt.In(intentions.TimeZone).Format(intentions.MassAtLayout))
	}

	return times
}

func TestSlots(t *testing.T) {
	weekStart := massAt(t, "2026-11-02 00:00")

	tests := []struct {
		name     string
		existing []intentions.Intention
		want     []string
	}{
		{
			name: "empty week",
			want: []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 07:00", "2026-11-08 10:00"},
		},
		{
			name:     "mass with an intention is skipped",
			existing: []intentions.Intention{{MassAt: massAt(t, "2026-11-02 18:00"), Content: "+ Jan Kowalski"}},
			want:     []string{"2026-11-02 06:30", "2026-11-08 07:00", "2026-11-08 10:00"},
		},
		{
			name:     "mass with an outside parish intention is skipped",
			existing: []intentions.Intention{{MassAt: massAt(t, "2026-11-08 10:00"), Content: "+ Anna Nowak", IsOutsideParish: true}},
			want:     []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 07:00"},
		},
		{
			name:     "existing slot is not duplicated",
			existing: []intentions.Intention{{MassAt: massAt(t, "2026-11-08 07:00"), IsPlaceholder: true}},
			want:     []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 10:00"},
		},
		{
			name:     "intention outside the schedule changes nothing",
			existing: []intentions.Intention{{MassAt: massAt(t, "2026-11-02 15:00"), Content: "+ Jan Kowalski"}},
			want:     []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 07:00", "2026-11-08 10:00"},
		},
		{
			name:     "same moment in UTC is recognized",
			existing: []intentions.Intention{{MassAt: massAt(t, "2026-11-02 18:00").UTC(), Content: "+ Jan Kowalski"}},
			want:     []string{"2026-11-02 06:30", "2026-11-08 07:00", "2026-11-08 10:00"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slotTimes(testSchedule.Slots(weekStart, tt.existing))

			if !slices.Equal(got, tt.want) {
				t.Errorf("slots = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSlotsArePlaceholders(t *testing.T) {
	slots := testSchedule.Slots(massAt(t, "2026-11-02 00:00"), nil)

	if len(slots) == 0 {
		t.Fatal("no slots")
	}
	for _, slot := range slots {
		if !slot.IsPlaceholder || slot.Content != "" || slot.ID != 0 {
			t.Errorf("slot = %+v, want a new placeholder without content", slot)
		}
	}
}

func TestSlotsAcrossDSTChange(t *testing.T) {
	got := slotTimes(testSchedule.Slots(massAt(t, "2026-10-19 00:00"), nil))

	want := []string{"2026-10-19 06:30", "2026-10-19 18:00", "2026-10-25 07:00", "2026-10-25 10:00"}
	if !slices.Equal(got, want) {
		t.Errorf("slots = %q, want %q", got, want)
	}
}

func TestSlotsWeekStartInUTC(t *testing.T) {
	got := slotTimes(testSchedule.Slots(massAt(t, "2026-11-02 00:00").UTC(), nil))

	want := []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 07:00", "2026-11-08 10:00"}
	if !slices.Equal(got, want) {
		t.Errorf("slots = %q, want %q", got, want)
	}
}
