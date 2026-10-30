package masses

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"
)

const otherDays = `"poniedziałek": ["6:30", "18:00"], "wtorek": [], "środa": [], "czwartek": [], "piątek": [], "sobota": []`

func scheduleJSON(name, sunday string) string {
	return fmt.Sprintf(`{"name": %q, "masses": {%s, "niedziela": %s}}`, name, otherDays, sunday)
}

func TestParseSchedules(t *testing.T) {
	data := "[" + scheduleJSON("Porządek zwykły", `["18:00", "7:00", "10:15"]`) + ", " + scheduleJSON("Wakacje", `[]`) + "]"

	got, err := ParseSchedules([]byte(data))
	if err != nil {
		t.Fatalf("ParseSchedules: %v", err)
	}

	if len(got) != 2 {
		t.Fatalf("schedules = %d, want 2", len(got))
	}
	if got[0].Name != "Porządek zwykły" || got[1].Name != "Wakacje" {
		t.Errorf("names = %q, %q, want file order", got[0].Name, got[1].Name)
	}

	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{name: "sunday sorted and normalized", got: got[0].Masses[time.Sunday], want: []string{"07:00", "10:15", "18:00"}},
		{name: "monday", got: got[0].Masses[time.Monday], want: []string{"06:30", "18:00"}},
		{name: "day without masses", got: got[0].Masses[time.Tuesday], want: []string{}},
		{name: "empty sunday", got: got[1].Masses[time.Sunday], want: []string{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !slices.Equal(tt.got, tt.want) {
				t.Errorf("masses = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestParseSchedulesErrors(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{name: "invalid JSON", data: `[`, want: "decoding schedules"},
		{name: "object instead of list", data: `{}`, want: "decoding schedules"},
		{name: "no schedules", data: `[]`, want: "no schedules defined"},
		{name: "unknown field", data: `[{"name": "A", "mases": {}}]`, want: `unknown field "mases"`},
		{name: "times as text", data: "[" + scheduleJSON("A", `"18:00"`) + "]", want: "decoding schedules"},
		{name: "empty name", data: "[" + scheduleJSON("", `[]`) + "]", want: "schedule without a name"},
		{name: "duplicate name", data: "[" + scheduleJSON("A", `[]`) + ", " + scheduleJSON("A", `[]`) + "]", want: `duplicate schedule "A"`},
		{name: "unknown day", data: `[{"name": "A", "masses": {"sroda": []}}]`, want: `schedule "A": unknown day "sroda"`},
		{name: "missing day", data: `[{"name": "A", "masses": {` + otherDays + `}}]`, want: `schedule "A": missing day "niedziela"`},
		{name: "invalid time", data: "[" + scheduleJSON("A", `["6:5"]`) + "]", want: `schedule "A": niedziela: invalid time "6:5"`},
		{name: "hour out of range", data: "[" + scheduleJSON("A", `["25:00"]`) + "]", want: `invalid time "25:00"`},
		{name: "not a time", data: "[" + scheduleJSON("A", `["osiemnasta"]`) + "]", want: `invalid time "osiemnasta"`},
		{name: "not a quarter hour", data: "[" + scheduleJSON("A", `["6:10"]`) + "]", want: `not a full quarter hour "6:10"`},
		{name: "duplicate time", data: "[" + scheduleJSON("A", `["18:00", "18:00"]`) + "]", want: `duplicate time "18:00"`},
		{name: "duplicate after normalization", data: "[" + scheduleJSON("A", `["6:30", "06:30"]`) + "]", want: `duplicate time "06:30"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseSchedules([]byte(tt.data))
			if err == nil {
				t.Fatalf("ParseSchedules(%s) error = nil, want %q", tt.data, tt.want)
			}

			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("error = %q, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestDefault(t *testing.T) {
	schedules, err := Default()
	if err != nil {
		t.Fatalf("Default: %v", err)
	}

	if len(schedules) == 0 {
		t.Error("Default returned no schedules")
	}
}
