package web

import (
	"bytes"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

func describeMasses(days []printedDay) []string {
	var out []string
	for _, day := range days {
		for _, mass := range day.masses {
			var parts []string
			for _, intention := range mass.intentions {
				switch {
				case intention.blank:
					parts = append(parts, "___")
				case intention.outside:
					parts = append(parts, "poza:"+intention.content)
				default:
					parts = append(parts, intention.content)
				}
			}
			out = append(out, fmt.Sprintf("%s %s %s", day.date.Format("02.01"), mass.time, strings.Join(parts, " | ")))
		}
	}

	return out
}

func blankTestWeek(t *testing.T) weekView {
	t.Helper()

	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	slot := newIntention(t, 1, "2026-11-02 06:30", false, "")
	slot.IsPlaceholder = true
	hidden := newIntention(t, 5, "2026-11-04 06:30", false, "ukryta")
	hidden.IsHidden = true

	return newWeekView(start, []intentions.Intention{
		slot,
		newIntention(t, 2, "2026-11-02 18:00", false, "+ Jan"),
		newIntention(t, 3, "2026-11-03 18:00", true, "+ Anna"),
		hidden,
		newIntention(t, 6, "2026-11-08 10:00", false, "Za parafian"),
	})
}

func TestPrintableDaysWithoutBlanksSkipEmptyMasses(t *testing.T) {
	got := describeMasses(printableDays(blankTestWeek(t), pdfOptions{}))

	want := []string{
		"02.11 18:00 + Jan",
		"03.11 18:00 poza:+ Anna",
		"08.11 10:00 Za parafian",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("masses:\n got %q\nwant %q", got, want)
	}
}

func TestPrintableDaysWithBlanks(t *testing.T) {
	got := describeMasses(printableDays(blankTestWeek(t), pdfOptions{blanks: 2}))

	want := []string{
		"02.11 6:30 ___ | ___",
		"02.11 18:00 + Jan",
		"03.11 18:00 ___ | ___ | poza:+ Anna",
		"05.11  ___ | ___",
		"06.11  ___ | ___",
		"07.11  ___ | ___",
		"08.11 10:00 Za parafian",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("masses:\n got %q\nwant %q", got, want)
	}
}

func TestPrintableDaysHiddenIntentionIsNotABlank(t *testing.T) {
	days := printableDays(blankTestWeek(t), pdfOptions{blanks: 1, extra: true})

	got := describeMasses(days)
	if !slices.Contains(got, "04.11 6:30 ___") {
		t.Errorf("a Mass with a hidden intention should only get the extra line: %q", got)
	}

	shown := describeMasses(printableDays(blankTestWeek(t), pdfOptions{blanks: 1, showHidden: true}))
	if !slices.Contains(shown, "04.11 6:30 ukryta") {
		t.Errorf("hidden intention not printed with showHidden: %q", shown)
	}
}

func TestPrintableDaysExtraLine(t *testing.T) {
	got := describeMasses(printableDays(blankTestWeek(t), pdfOptions{blanks: 1, extra: true}))

	for _, want := range []string{"02.11 18:00 + Jan | ___", "08.11 10:00 Za parafian | ___", "02.11 6:30 ___"} {
		if !slices.Contains(got, want) {
			t.Errorf("masses do not contain %q: %q", want, got)
		}
	}
}

func TestPrintableDaysAddsPlannedMasses(t *testing.T) {
	at := func(text string) time.Time {
		v, err := time.ParseInLocation(intentions.MassAtLayout, text, intentions.TimeZone)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	opts := pdfOptions{blanks: 1, planned: map[string][]time.Time{
		"2026-11-02": {at("2026-11-02 07:00"), at("2026-11-02 18:00")},
		"2026-11-05": {at("2026-11-05 18:00")},
	}}

	got := describeMasses(printableDays(blankTestWeek(t), opts))

	for _, want := range []string{
		"02.11 6:30 ___",
		"02.11 7:00 ___",
		"02.11 18:00 + Jan",
		"05.11 18:00 ___",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("masses do not contain %q: %q", want, got)
		}
	}
	if i, j := slices.Index(got, "02.11 6:30 ___"), slices.Index(got, "02.11 7:00 ___"); i > j {
		t.Errorf("planned Masses are not sorted by time: %q", got)
	}
	if slices.Contains(got, "05.11  ___") {
		t.Errorf("a day with planned Masses also got an empty row: %q", got)
	}
}

func TestLayoutWeekDrawsDottedLinesAlignedWithTime(t *testing.T) {
	pages := layoutWeek(blankTestWeek(t), pdfOptions{blanks: 1}, fakeMeasurer{})

	var dotted []pdfRule
	for _, page := range pages {
		for _, rule := range page.Rules {
			if rule.Dotted {
				dotted = append(dotted, rule)
			}
		}
	}
	if len(dotted) != 8 {
		t.Fatalf("dotted rules = %d, want 8 (2.11 6:30, 3.11 18:00, three empty days with a line and a time field)", len(dotted))
	}

	var timeRun pdfRun
	for _, run := range pages[0].Runs {
		if run.Text == "6:30" {
			timeRun = run
			break
		}
	}
	if timeRun.Text == "" {
		t.Fatal("6:30 is not printed")
	}
	aligned := slices.ContainsFunc(dotted, func(r pdfRule) bool { return r.Y == timeRun.Baseline })
	if !aligned {
		t.Errorf("no dotted line on the baseline of 6:30 (%v)", timeRun.Baseline)
	}
}

func TestWriteWeekPDFWithBlanks(t *testing.T) {
	var buf bytes.Buffer
	err := writeWeekPDF(&buf, blankTestWeek(t), pdfOptions{blanks: 3, extra: true})
	if err != nil {
		t.Fatalf("writeWeekPDF: %v", err)
	}
	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Error("output is not a PDF")
	}
}

func TestParsePrintOptions(t *testing.T) {
	schedules := []masses.Schedule{{ID: 1, Name: "Zwykły"}, {ID: 2, Name: "Wakacje"}}
	tests := []struct {
		raw  string
		want printOptions
	}{
		{"", printOptions{Lines: 1, Planned: true, ScheduleID: 2}},
		{"blanks=1", printOptions{Blanks: true, Lines: 1, Planned: true, ScheduleID: 2}},
		{"blanks=1&set=1&lines=3&extra=1&schedule=1", printOptions{Blanks: true, Lines: 3, Extra: true, ScheduleID: 1}},
		{"blanks=1&set=1&planned=1&lines=9&hidden=1&schedule=77", printOptions{ShowHidden: true, Blanks: true, Lines: 1, Planned: true, ScheduleID: 2}},
	}

	for _, tt := range tests {
		query, _ := url.ParseQuery(tt.raw)
		got := parsePrintOptions(query, 2, schedules)
		if got != tt.want {
			t.Errorf("parse(%q) = %+v, want %+v", tt.raw, got, tt.want)
		}

		again, _ := url.ParseQuery(got.query("2026-11-02"))
		if back := parsePrintOptions(again, 2, schedules); back != got {
			t.Errorf("query(%+v) does not round-trip: %+v", got, back)
		}
	}
}

func TestPrintPreviewBlankOptions(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	body := getPrint(t, h, "/print?week=2026-11-02").Body.String()
	checkBody(t, body, `<input type="checkbox" id="p-blanks" name="blanks" value="1" data-autosubmit> Miejsca do wpisania</label>`)
	if strings.Contains(body, `id="p-lines"`) {
		t.Error("blank options shown before the mode is switched on")
	}

	body = getPrint(t, h, "/print?week=2026-11-02&blanks=1").Body.String()
	checkBody(t, body,
		`id="p-blanks" name="blanks" value="1" checked data-autosubmit>`,
		`<input type="hidden" name="set" value="1">`,
		`<option value="1" selected>1</option>`,
		`id="p-planned" name="planned" value="1" checked data-autosubmit>`,
		`<option value="1" selected>Porządek zwykły</option>`,
		`<iframe class="print-preview" src="/pdf?week=2026-11-02&amp;blanks=1&amp;lines=1&amp;set=1&amp;planned=1&amp;schedule=1" title=`,
	)

	body = getPrint(t, h, "/print?week=2026-11-02&blanks=1&set=1&lines=2&extra=1").Body.String()
	checkBody(t, body,
		`<option value="2" selected>2</option>`,
		`id="p-extra" name="extra" value="1" checked data-autosubmit>`,
		`id="p-planned" name="planned" value="1" data-autosubmit>`,
		`data-autosubmit disabled>`,
		`src="/pdf?week=2026-11-02&amp;blanks=1&amp;lines=2&amp;set=1&amp;extra=1&amp;schedule=1"`,
	)
}

func TestPDFWithBlanks(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")

	rec := getPDF(t, h, "/pdf?week=2026-11-02&blanks=1&lines=2&set=1&planned=1&schedule=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Error("response is not a PDF")
	}
	if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="intencje-2026-11-02-do-wpisania.pdf"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}

	planned, err := h.plannedForPrint(time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone), printOptions{Blanks: true, Planned: true, ScheduleID: 1}, testSchedules)
	if err != nil {
		t.Fatal(err)
	}
	if len(planned) != 0 {
		t.Errorf("planned = %v, want none for schedules without IDs in the test list", planned)
	}
}

func TestPlannedForPrintUsesScheduleForUnloadedWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	schedules, err := masses.Load(h.db)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	planned, err := h.plannedForPrint(start, printOptions{Blanks: true, Planned: true, ScheduleID: schedules[0].ID}, schedules)
	if err != nil {
		t.Fatal(err)
	}

	var got []string
	for date, times := range planned {
		for _, at := range times {
			got = append(got, date+" "+at.Format("15:04"))
		}
	}
	slices.Sort(got)
	want := []string{"2026-11-02 06:30", "2026-11-02 18:00", "2026-11-08 07:00", "2026-11-08 10:00"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("planned = %q, want %q", got, want)
	}

	none, err := h.plannedForPrint(start, printOptions{Blanks: true, Planned: false, ScheduleID: schedules[0].ID}, schedules)
	if err != nil || len(none) != 0 {
		t.Errorf("planned with the option off = %v, %v", none, err)
	}
}
