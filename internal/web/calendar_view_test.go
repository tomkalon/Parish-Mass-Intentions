package web

import (
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func findCalendarWeek(t *testing.T, month calendarMonth, url string) calendarWeek {
	t.Helper()

	for _, week := range month.Weeks {
		if week.URL == url {
			return week
		}
	}
	t.Fatalf("%s has no week %s", month.Name, url)

	return calendarWeek{}
}

func TestNewCalendarView(t *testing.T) {
	current := time.Date(2026, 11, 9, 0, 0, 0, 0, intentions.TimeZone)
	today := time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone)
	counts := map[string]intentions.DayCount{
		"2025-12-29": {Filled: 2},
		"2026-01-01": {Filled: 1},
		"2026-11-02": {Filled: 3},
		"2026-11-09": {Filled: 7, Placeholders: 2},
		"2026-11-10": {Placeholders: 1},
	}

	view := newCalendarView(2026, current, today, counts)

	if view.Year != 2026 || !view.HasCurrent {
		t.Errorf("Year = %d, HasCurrent = %t, want 2026, true", view.Year, view.HasCurrent)
	}
	if want := "/?week=2026-11-09&calendar=2025"; view.PrevURL != want {
		t.Errorf("PrevURL = %q, want %q", view.PrevURL, want)
	}
	if want := "/?week=2026-11-09&calendar=2027"; view.NextURL != want {
		t.Errorf("NextURL = %q, want %q", view.NextURL, want)
	}
	if want := "/?week=2026-11-09"; view.CloseURL != want {
		t.Errorf("CloseURL = %q, want %q", view.CloseURL, want)
	}
	if len(view.Months) != 12 || view.Months[0].Name != "Styczeń" || view.Months[11].Name != "Grudzień" {
		t.Fatalf("months = %d (%s … %s), want 12 from Styczeń to Grudzień", len(view.Months), view.Months[0].Name, view.Months[len(view.Months)-1].Name)
	}

	january := view.Months[0].Weeks[0]
	if want := "/?week=2025-12-29"; january.URL != want {
		t.Errorf("first January week URL = %q, want %q", january.URL, want)
	}
	if want := "Tydzień 29.12 – 4.1.2026: 3 intencje"; january.Label != want {
		t.Errorf("first January week label = %q, want %q", january.Label, want)
	}
	for i, day := range january.Days[:3] {
		if day != (calendarDay{}) {
			t.Errorf("January day %d = %+v, want an empty cell from December", i, day)
		}
	}
	if want := (calendarDay{Day: 1, Count: 1, Level: 1, InMonth: true}); january.Days[3] != want {
		t.Errorf("1 January = %+v, want %+v", january.Days[3], want)
	}

	november := view.Months[10]
	opened := findCalendarWeek(t, november, "/?week=2026-11-09")
	if !opened.IsCurrent {
		t.Error("week 9.11 is not marked as current")
	}
	if want := "Tydzień 9.11 – 15.11.2026: 7 intencji, są msze do uzupełnienia"; opened.Label != want {
		t.Errorf("current week label = %q, want %q", opened.Label, want)
	}
	if want := (calendarDay{Day: 9, Count: 7, Level: maxCalendarLevel, InMonth: true, HasEmpty: true}); opened.Days[0] != want {
		t.Errorf("9 November = %+v, want %+v", opened.Days[0], want)
	}
	if want := (calendarDay{Day: 10, InMonth: true, HasEmpty: true}); opened.Days[1] != want {
		t.Errorf("10 November = %+v, want %+v", opened.Days[1], want)
	}

	thisWeek := findCalendarWeek(t, november, "/?week=2026-11-02")
	if thisWeek.IsCurrent {
		t.Error("week 2.11 is marked as current")
	}
	if want := (calendarDay{Day: 2, Count: 3, Level: 3, InMonth: true, IsToday: true}); thisWeek.Days[0] != want {
		t.Errorf("2 November = %+v, want %+v", thisWeek.Days[0], want)
	}
}

func TestNewCalendarViewWeeksAcrossDSTChange(t *testing.T) {
	current := time.Date(2026, 11, 9, 0, 0, 0, 0, intentions.TimeZone)

	march := newCalendarView(2026, current, current, nil).Months[2]

	want := []string{
		"/?week=2026-02-23",
		"/?week=2026-03-02",
		"/?week=2026-03-09",
		"/?week=2026-03-16",
		"/?week=2026-03-23",
		"/?week=2026-03-30",
	}
	if len(march.Weeks) != len(want) {
		t.Fatalf("March has %d weeks, want %d", len(march.Weeks), len(want))
	}
	for i, week := range march.Weeks {
		if week.URL != want[i] {
			t.Errorf("March week %d URL = %q, want %q", i, week.URL, want[i])
		}
	}
	if last := march.Weeks[5].Days; last[1].Day != 31 || last[2].InMonth {
		t.Errorf("last March week = %+v, want 31 March on Tuesday and April left empty", last)
	}
}

func TestNewCalendarViewOtherYearHasNoCurrentWeek(t *testing.T) {
	current := time.Date(2026, 11, 9, 0, 0, 0, 0, intentions.TimeZone)

	view := newCalendarView(2027, current, current, nil)

	if view.HasCurrent {
		t.Error("2027 has the current week of 2026")
	}
	if want := "/?week=2026-11-09"; view.CloseURL != want {
		t.Errorf("CloseURL = %q, want %q", view.CloseURL, want)
	}
}

func TestCalendarRange(t *testing.T) {
	from, to := calendarRange(2026)

	if want := time.Date(2025, 12, 29, 0, 0, 0, 0, intentions.TimeZone); !from.Equal(want) {
		t.Errorf("from = %v, want %v", from, want)
	}
	if want := time.Date(2027, 1, 4, 0, 0, 0, 0, intentions.TimeZone); !to.Equal(want) {
		t.Errorf("to = %v, want %v", to, want)
	}
}

func TestCalendarDayTitle(t *testing.T) {
	tests := []struct {
		day  calendarDay
		want string
	}{
		{day: calendarDay{}, want: "0 intencji"},
		{day: calendarDay{Count: 1}, want: "1 intencja"},
		{day: calendarDay{Count: 2, HasEmpty: true}, want: "2 intencje · msze do uzupełnienia"},
	}

	for _, tt := range tests {
		if got := tt.day.Title(); got != tt.want {
			t.Errorf("%+v.Title() = %q, want %q", tt.day, got, tt.want)
		}
	}
}
