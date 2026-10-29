package web

import (
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestNewFormView(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	got := newFormView(start, "2026-11-03", "18:00")

	if want := "2026-11-02"; got.Week != want {
		t.Errorf("Week = %q, want %q", got.Week, want)
	}
	if want := "2.11 – 8.11.2026"; got.Legend != want {
		t.Errorf("Legend = %q, want %q", got.Legend, want)
	}
	if want := "/?week=2026-11-02"; got.CloseURL != want {
		t.Errorf("CloseURL = %q, want %q", got.CloseURL, want)
	}

	if len(got.Days) != 7 {
		t.Fatalf("len(Days) = %d, want 7", len(got.Days))
	}
	if want := (dayOption{Value: "2026-11-02", Short: "Pn", Date: "2.11"}); got.Days[0] != want {
		t.Errorf("Days[0] = %+v, want %+v", got.Days[0], want)
	}
	if want := (dayOption{Value: "2026-11-03", Short: "Wt", Date: "3.11", Checked: true}); got.Days[1] != want {
		t.Errorf("Days[1] = %+v, want %+v", got.Days[1], want)
	}
	if want := (dayOption{Value: "2026-11-08", Short: "Nd", Date: "8.11"}); got.Days[6] != want {
		t.Errorf("Days[6] = %+v, want %+v", got.Days[6], want)
	}

	if got.Day != "2026-11-03" || got.Time != "18:00" {
		t.Errorf("Day, Time = %q, %q, want %q, %q", got.Day, got.Time, "2026-11-03", "18:00")
	}

	if len(got.Times) != 61 {
		t.Fatalf("len(Times) = %d, want 61 (6:00–21:00 every 15 minutes)", len(got.Times))
	}
	if want := (timeOption{Value: "06:00", Label: "6:00"}); got.Times[0] != want {
		t.Errorf("Times[0] = %+v, want %+v", got.Times[0], want)
	}
	if want := (timeOption{Value: "21:00", Label: "21:00"}); got.Times[60] != want {
		t.Errorf("Times[60] = %+v, want %+v", got.Times[60], want)
	}
	if selected := selectedTimes(got.Times); len(selected) != 1 || selected[0] != "18:00" {
		t.Errorf("selected times = %q, want [18:00]", selected)
	}
}

func TestNewFormViewWithoutSelection(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		name     string
		day      string
		massTime string
	}{
		{name: "nothing given", day: "", massTime: ""},
		{name: "day from another week", day: "2026-11-10", massTime: ""},
		{name: "time that is not HH:MM", day: "", massTime: "bzdura"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newFormView(start, tt.day, tt.massTime)

			if got.Day != "" || got.Time != "" {
				t.Errorf("Day, Time = %q, %q, want empty", got.Day, got.Time)
			}
			for _, day := range got.Days {
				if day.Checked {
					t.Errorf("day %s is checked, want none", day.Value)
				}
			}
			if selected := selectedTimes(got.Times); len(selected) != 0 {
				t.Errorf("selected times = %q, want none", selected)
			}
		})
	}
}

func TestNewFormViewNormalizesTime(t *testing.T) {
	got := newFormView(time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone), "", "6:30")

	if want := "06:30"; got.Time != want {
		t.Errorf("Time = %q, want %q", got.Time, want)
	}
}

func TestNewFormViewKeepsTimeOutsideList(t *testing.T) {
	got := newFormView(time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone), "", "18:10")

	if len(got.Times) != 62 {
		t.Fatalf("len(Times) = %d, want 62", len(got.Times))
	}
	if want := (timeOption{Value: "18:10", Label: "18:10", Selected: true}); got.Times[0] != want {
		t.Errorf("Times[0] = %+v, want %+v", got.Times[0], want)
	}
}

func TestWeekViewURLsWithOpenForm(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		name     string
		day      string
		massTime string
		wantPrev string
		wantNext string
	}{
		{
			name:     "day and time move to the same weekday",
			day:      "2026-11-03",
			massTime: "18:00",
			wantPrev: "/?day=2026-10-27&new=1&time=18%3A00&week=2026-10-26#d-2026-10-27",
			wantNext: "/?day=2026-11-10&new=1&time=18%3A00&week=2026-11-09#d-2026-11-10",
		},
		{
			name:     "no day selected",
			wantPrev: "/?new=1&week=2026-10-26",
			wantNext: "/?new=1&week=2026-11-09",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := newWeekView(start, nil)
			form := newFormView(start, tt.day, tt.massTime)
			view.Form = &form

			if got := view.PrevURL(); got != tt.wantPrev {
				t.Errorf("PrevURL() = %q, want %q", got, tt.wantPrev)
			}
			if got := view.NextURL(); got != tt.wantNext {
				t.Errorf("NextURL() = %q, want %q", got, tt.wantNext)
			}
		})
	}
}

func TestMarkSelected(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-03 06:30", false, "A"),
		newIntention(t, 2, "2026-11-03 18:00", false, "B"),
	})

	markSelected(view.Days, "2026-11-03", "18:00")

	tuesday := view.Days[1]
	if !tuesday.IsSelected {
		t.Error("tuesday is not selected")
	}
	if tuesday.Masses[0].IsSelected {
		t.Error("6:30 mass is selected, want only 18:00")
	}
	if !tuesday.Masses[1].IsSelected {
		t.Error("18:00 mass is not selected")
	}
	if view.Days[0].IsSelected {
		t.Error("monday is selected, want only tuesday")
	}
}

func selectedTimes(options []timeOption) []string {
	var selected []string
	for _, option := range options {
		if option.Selected {
			selected = append(selected, option.Value)
		}
	}

	return selected
}

func TestNewIntentionURL(t *testing.T) {
	tests := []struct {
		name     string
		day      string
		massTime string
		want     string
	}{
		{
			name: "week only",
			want: "/?new=1&week=2026-11-02",
		},
		{
			name: "day",
			day:  "2026-11-03",
			want: "/?day=2026-11-03&new=1&week=2026-11-02#d-2026-11-03",
		},
		{
			name:     "day and time",
			day:      "2026-11-03",
			massTime: "18:00",
			want:     "/?day=2026-11-03&new=1&time=18%3A00&week=2026-11-02#d-2026-11-03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newIntentionURL("2026-11-02", tt.day, tt.massTime); got != tt.want {
				t.Errorf("newIntentionURL = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestWeekViewNewURLs(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-08 06:30", false, "A"),
	})

	if want := "/?new=1&week=2026-11-02"; view.NewURL() != want {
		t.Errorf("week NewURL() = %q, want %q", view.NewURL(), want)
	}

	sunday := view.Days[6]
	if want := "/?day=2026-11-08&new=1&week=2026-11-02#d-2026-11-08"; sunday.NewURL != want {
		t.Errorf("sunday NewURL = %q, want %q", sunday.NewURL, want)
	}

	if len(sunday.Masses) != 1 {
		t.Fatalf("sunday has %d masses, want 1", len(sunday.Masses))
	}
	if want := "/?day=2026-11-08&new=1&time=06%3A30&week=2026-11-02#d-2026-11-08"; sunday.Masses[0].NewURL != want {
		t.Errorf("mass NewURL = %q, want %q", sunday.Masses[0].NewURL, want)
	}
}
