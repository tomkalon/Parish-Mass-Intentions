package web

import (
	"maps"
	"net/url"
	"slices"
	"testing"
	"time"

	"go-parafia/internal/masses"
)

func TestScheduleSummary(t *testing.T) {
	tests := []struct {
		name   string
		masses [7][]string
		want   string
	}{
		{
			name: "weekdays grouped",
			masses: [7][]string{
				time.Monday: {"06:30", "18:00"}, time.Tuesday: {"06:30", "18:00"}, time.Wednesday: {"06:30", "18:00"},
				time.Thursday: {"06:30", "18:00"}, time.Friday: {"06:30", "18:00"}, time.Saturday: {"06:30", "18:00"},
				time.Sunday: {"07:00", "10:00"},
			},
			want: "Pn–So: 6:30, 18:00 · Nd: 7:00, 10:00",
		},
		{
			name:   "single days and gaps",
			masses: [7][]string{time.Monday: {"07:00"}, time.Wednesday: {"07:00"}, time.Sunday: {"09:00"}},
			want:   "Pn: 7:00 · Śr: 7:00 · Nd: 9:00",
		},
		{
			name: "no masses",
			want: "bez mszy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := scheduleSummary(masses.Schedule{Masses: tt.masses}); got != tt.want {
				t.Errorf("scheduleSummary = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScheduleFromForm(t *testing.T) {
	form := url.Values{"name": {"  Zimowy  "}, "day-1": {"18:00, 6:30"}, "day-0": {"7:00; 10:00"}, "day-3": {"  "}}

	got, errs := scheduleFromForm(form)

	if len(errs) != 0 {
		t.Fatalf("errors = %q, want none", errs)
	}
	if got.Name != "Zimowy" {
		t.Errorf("Name = %q, want %q", got.Name, "Zimowy")
	}
	if !slices.Equal(got.Masses[time.Monday], []string{"06:30", "18:00"}) || !slices.Equal(got.Masses[time.Sunday], []string{"07:00", "10:00"}) {
		t.Errorf("Masses = %q", got.Masses)
	}
	if len(got.Masses[time.Wednesday]) != 0 {
		t.Errorf("Wednesday = %q, want no masses", got.Masses[time.Wednesday])
	}
}

func TestScheduleFromFormErrors(t *testing.T) {
	form := url.Values{"name": {""}, "day-1": {"6:5"}, "day-3": {"6:10"}, "day-0": {"18:00, 18:00"}}

	_, errs := scheduleFromForm(form)

	want := map[string]string{
		"name":  "Pole jest wymagane.",
		"day-1": "Nieprawidłowa godzina „6:5” – wpisz np. 6:30.",
		"day-3": "Godzina „6:10” nie jest pełnym kwadransem (:00, :15, :30, :45).",
		"day-0": "Godzina „18:00” jest wpisana dwa razy.",
	}
	if !maps.Equal(errs, want) {
		t.Errorf("errors = %q, want %q", errs, want)
	}
}
