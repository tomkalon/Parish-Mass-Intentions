package web

import (
	"strings"
	"testing"
	"time"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

func summaryOf(t *testing.T, first string, days int, skip ...string) gregorian.Summary {
	t.Helper()

	start := date(t, first).Add(18 * time.Hour)
	var masses []gregorian.Mass
	for day := start; len(masses) < days; day = day.AddDate(0, 0, 1) {
		if contains(skip, day.Format(intentions.DateLayout)) {
			continue
		}
		masses = append(masses, gregorian.Mass{ID: len(masses) + 1, MassAt: day})
	}

	s := gregorian.Summary{Series: gregorian.Series{ID: 7, Content: "+ Jan"}, Masses: masses}
	if len(masses) < gregorian.Length {
		s.NextDay = gregorian.NextMassDay(date(t, masses[len(masses)-1].Date()))
	}

	return s
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}

	return false
}

func TestSeriesOptionState(t *testing.T) {
	s := summaryOf(t, "2026-11-01", 6, "2026-11-03")

	tests := []struct {
		day      string
		step     int
		meta     string
		kind     string
		disabled bool
		hidden   bool
	}{
		{day: "", step: 7, meta: "następna: 8.11 · 18:00"},
		{day: "2026-11-08", step: 7, meta: "na ten dzień · 18:00", kind: "due"},
		{day: "2026-11-12", step: 7, meta: "zaległa od 8.11", kind: "late"},
		{day: "2026-11-03", step: 3, meta: "uzupełnia przerwę w serii", kind: "gap"},
		{day: "2026-11-04", step: 3, meta: "już dodana na ten dzień", disabled: true},
		{day: "2026-10-31", disabled: true, hidden: true},
	}

	for _, tt := range tests {
		step, meta, kind, disabled, hidden := seriesOptionState(s, tt.day)
		if step != tt.step || meta != tt.meta || kind != tt.kind || disabled != tt.disabled || hidden != tt.hidden {
			t.Errorf("seriesOptionState(%q) = %d %q %q %v %v, want %d %q %q %v %v", tt.day, step, meta, kind, disabled, hidden, tt.step, tt.meta, tt.kind, tt.disabled, tt.hidden)
		}
	}

	full := summaryOf(t, "2026-10-01", 30)
	if _, meta, _, disabled, _ := seriesOptionState(full, "2026-11-08"); !disabled || meta != "ma już 30 Mszy" {
		t.Errorf("full series: %q, disabled %v; want ma już 30 Mszy, disabled", meta, disabled)
	}
}

func TestSeriesNoticesListGaps(t *testing.T) {
	s := summaryOf(t, "2026-11-01", 6, "2026-11-03", "2026-11-05")
	s.Gaps = []time.Time{date(t, "2026-11-03"), date(t, "2026-11-05")}
	s.GapsAfter = []int{2, 3}
	s.End = date(t, "2026-12-02")

	notices := seriesNotices([]gregorian.Summary{s}, date(t, "2026-11-02"))
	if len(notices) != 1 || notices[0].Text != "ma przerwy: 3.11, 5.11 – zakończy się ok. 2.12." {
		t.Errorf("notices = %+v", notices)
	}
	if !strings.HasSuffix(notices[0].URL, "gregorian=active&series=7") {
		t.Errorf("URL = %q", notices[0].URL)
	}

	if got := seriesNotices([]gregorian.Summary{s}, date(t, "2026-10-19")); len(got) != 0 {
		t.Errorf("notices before the series starts = %+v, want none", got)
	}
}

func TestMarkSeriesDue(t *testing.T) {
	s := summaryOf(t, "2026-11-01", 9)
	view := newWeekView(date(t, "2026-11-09"), nil)

	markSeriesDue(view.Days, []gregorian.Summary{s}, "2026-11-09")

	if len(view.Days[1].SeriesDue) != 1 {
		t.Fatalf("Tuesday reminders = %+v, want one", view.Days[1].SeriesDue)
	}
	due := view.Days[1].SeriesDue[0]
	if due.Step != 10 || due.Name != "+ Jan" || due.URL != "/?day=2026-11-10&new=1&time=18%3A00&week=2026-11-09&series=7" {
		t.Errorf("reminder = %+v", due)
	}
	for i, day := range view.Days {
		if i != 1 && len(day.SeriesDue) > 0 {
			t.Errorf("%s has a reminder, want only Tuesday", day.Date)
		}
	}
}
