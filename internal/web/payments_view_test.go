package web

import (
	"net/url"
	"slices"
	"testing"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

func date(t *testing.T, value string) time.Time {
	t.Helper()

	day, err := time.ParseInLocation(intentions.DateLayout, value, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing date %q: %v", value, err)
	}

	return day
}

func TestParsePaymentsQuery(t *testing.T) {
	start := date(t, "2026-11-30")

	tests := []struct {
		query     string
		wantTab   string
		wantMonth string
		wantYear  int
	}{
		{query: "payments=week", wantTab: paymentsWeek, wantMonth: "2026-11", wantYear: 2026},
		{query: "payments=month", wantTab: paymentsMonth, wantMonth: "2026-11", wantYear: 2026},
		{query: "payments=month&month=2025-02", wantTab: paymentsMonth, wantMonth: "2025-02", wantYear: 2026},
		{query: "payments=year", wantTab: paymentsYear, wantMonth: "2026-11", wantYear: 2026},
		{query: "payments=year&year=2024", wantTab: paymentsYear, wantMonth: "2026-11", wantYear: 2024},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			query, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatal(err)
			}

			got, err := parsePaymentsQuery(query, start)
			if err != nil {
				t.Fatalf("parsePaymentsQuery() error = %v", err)
			}
			if got.tab != tt.wantTab || got.month.Format(monthLayout) != tt.wantMonth || got.year != tt.wantYear {
				t.Errorf("parsePaymentsQuery() = %s %s %d, want %s %s %d", got.tab, got.month.Format(monthLayout), got.year, tt.wantTab, tt.wantMonth, tt.wantYear)
			}
			if got.month.Location() != intentions.TimeZone || got.month.Day() != 1 || got.month.Hour() != 0 {
				t.Errorf("month = %v, want midnight of the 1st in Europe/Warsaw", got.month)
			}
		})
	}
}

func TestParsePaymentsQueryInvalid(t *testing.T) {
	for _, raw := range []string{
		"payments=decade",
		"payments=1",
		"payments=month&month=2026-13",
		"payments=month&month=11-2026",
		"payments=month&month=2026-11-01",
		"payments=month&month=0999-01",
		"payments=year&year=abc",
		"payments=year&year=999",
		"payments=year&year=10000",
	} {
		query, err := url.ParseQuery(raw)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parsePaymentsQuery(query, date(t, "2026-11-09")); err == nil {
			t.Errorf("parsePaymentsQuery(%q) error = nil, want an error", raw)
		}
	}
}

func TestPaymentsQuerySpan(t *testing.T) {
	start := date(t, "2026-10-26")

	tests := []struct {
		query    paymentsQuery
		wantFrom string
		wantTo   string
	}{
		{query: paymentsQuery{tab: paymentsWeek}, wantFrom: "2026-10-26", wantTo: "2026-11-02"},
		{query: paymentsQuery{tab: paymentsMonth, month: date(t, "2026-10-01")}, wantFrom: "2026-10-01", wantTo: "2026-11-01"},
		{query: paymentsQuery{tab: paymentsMonth, month: date(t, "2026-12-01")}, wantFrom: "2026-12-01", wantTo: "2027-01-01"},
		{query: paymentsQuery{tab: paymentsYear, year: 2026}, wantFrom: "2026-01-01", wantTo: "2027-01-01"},
	}

	for _, tt := range tests {
		from, to := tt.query.span(start)
		got := from.Format(intentions.DateLayout) + " – " + to.Format(intentions.DateLayout)
		if want := tt.wantFrom + " – " + tt.wantTo; got != want {
			t.Errorf("%s span = %s, want %s", tt.query.tab, got, want)
		}
	}
}

type paymentsRowSummary struct {
	Label     string
	URL       string
	Amount    intentions.Grosze
	Count     int
	Unpaid    int
	Width     string
	IsCurrent bool
}

func summarizeRows(rows []paymentsRow) []paymentsRowSummary {
	var out []paymentsRowSummary
	for _, r := range rows {
		out = append(out, paymentsRowSummary(r))
	}

	return out
}

func TestNewPaymentsViewWeek(t *testing.T) {
	start := date(t, "2026-11-09")
	counts := map[string]intentions.DayCount{
		"2026-11-08": {Filled: 9, Amount: 900000},
		"2026-11-09": {Filled: 3, Amount: 35000},
		"2026-11-10": {Filled: 3, Unpaid: 1, Amount: 25000, Placeholders: 2},
		"2026-11-15": {Filled: 5, Unpaid: 1, Amount: 50000},
		"2026-11-16": {Filled: 9, Amount: 900000},
	}

	view := newPaymentsView(paymentsQuery{tab: paymentsWeek}, start, time.Date(2026, 11, 10, 9, 0, 0, 0, intentions.TimeZone), counts)

	checks := []struct{ name, got, want string }{
		{"Title", view.Title, "9 – 15 listopada 2026"},
		{"FirstColumn", view.FirstColumn, "Dzień"},
		{"PrevURL", view.PrevURL, "/?week=2026-11-02&payments=week"},
		{"NextURL", view.NextURL, "/?week=2026-11-16&payments=week"},
		{"PrevLabel", view.PrevLabel, "Poprzedni tydzień"},
		{"WeekTabURL", view.WeekTabURL, "/?week=2026-11-09&payments=week"},
		{"MonthTabURL", view.MonthTabURL, "/?week=2026-11-09&payments=month"},
		{"YearTabURL", view.YearTabURL, "/?week=2026-11-09&payments=year"},
		{"CloseURL", view.CloseURL, "/?week=2026-11-09"},
		{"OpenWeekURL", view.OpenWeekURL, "/?week=2026-11-09"},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("%s = %q, want %q", c.name, c.got, c.want)
		}
	}
	if view.Total != 110000 || view.Count != 11 || view.Unpaid != 2 {
		t.Errorf("totals = %d zł / %d / %d unpaid, want 110000 / 11 / 2", view.Total, view.Count, view.Unpaid)
	}

	want := []paymentsRowSummary{
		{Label: "Poniedziałek 9.11", Amount: 35000, Count: 3, Width: "70.0%"},
		{Label: "Wtorek 10.11", Amount: 25000, Count: 3, Unpaid: 1, Width: "50.0%", IsCurrent: true},
		{Label: "Środa 11.11", Width: "0%"},
		{Label: "Czwartek 12.11", Width: "0%"},
		{Label: "Piątek 13.11", Width: "0%"},
		{Label: "Sobota 14.11", Width: "0%"},
		{Label: "Niedziela 15.11", Amount: 50000, Count: 5, Unpaid: 1, Width: "100.0%"},
	}
	if got := summarizeRows(view.Rows); !slices.Equal(got, want) {
		t.Errorf("rows =\n%+v\nwant\n%+v", got, want)
	}
	if got, want := view.Rows[0].Title(), "Poniedziałek 9.11: 350 zł, 3 intencje"; got != want {
		t.Errorf("row title = %q, want %q", got, want)
	}
}

func TestNewPaymentsViewMonth(t *testing.T) {
	current := date(t, "2026-11-09")
	counts := map[string]intentions.DayCount{
		"2026-10-31": {Filled: 9, Amount: 900000},
		"2026-11-01": {Filled: 6, Unpaid: 1, Amount: 65000},
		"2026-11-02": {Filled: 3, Amount: 30000},
		"2026-11-08": {Filled: 2, Amount: 20000},
		"2026-11-12": {Filled: 2, Unpaid: 2},
		"2026-11-30": {Filled: 1, Amount: 10000},
		"2026-12-01": {Filled: 9, Amount: 900000},
	}

	view := newPaymentsView(paymentsQuery{tab: paymentsMonth, month: date(t, "2026-11-01")}, current, current, counts)

	if view.Title != "Listopad 2026" || view.FirstColumn != "Tydzień" || view.OpenWeekURL != "" {
		t.Errorf("title, first column, open week = %q, %q, %q; want Listopad 2026, Tydzień and no week link", view.Title, view.FirstColumn, view.OpenWeekURL)
	}
	if view.PrevURL != "/?week=2026-11-09&payments=month&month=2026-10" || view.NextURL != "/?week=2026-11-09&payments=month&month=2026-12" {
		t.Errorf("prev, next = %q, %q", view.PrevURL, view.NextURL)
	}
	if view.Total != 125000 || view.Count != 14 || view.Unpaid != 3 {
		t.Errorf("totals = %d / %d / %d unpaid, want 125000 / 14 / 3 (days outside November are not counted)", view.Total, view.Count, view.Unpaid)
	}

	want := []paymentsRowSummary{
		{Label: "1.11", URL: "/?week=2026-10-26&payments=week", Amount: 65000, Count: 6, Unpaid: 1, Width: "100.0%"},
		{Label: "2 – 8.11", URL: "/?week=2026-11-02&payments=week", Amount: 50000, Count: 5, Width: "76.9%"},
		{Label: "9 – 15.11", URL: "/?week=2026-11-09&payments=week", Count: 2, Unpaid: 2, Width: "0%", IsCurrent: true},
		{Label: "16 – 22.11", URL: "/?week=2026-11-16&payments=week", Width: "0%"},
		{Label: "23 – 29.11", URL: "/?week=2026-11-23&payments=week", Width: "0%"},
		{Label: "30.11", URL: "/?week=2026-11-30&payments=week", Amount: 10000, Count: 1, Width: "15.4%"},
	}
	if got := summarizeRows(view.Rows); !slices.Equal(got, want) {
		t.Errorf("rows =\n%+v\nwant\n%+v", got, want)
	}
}

func TestNewPaymentsViewMonthStartingMidWeek(t *testing.T) {
	current := date(t, "2026-11-09")

	view := newPaymentsView(paymentsQuery{tab: paymentsMonth, month: date(t, "2026-10-01")}, current, current, nil)

	var labels []string
	for _, row := range view.Rows {
		labels = append(labels, row.Label)
	}
	want := []string{"1 – 4.10", "5 – 11.10", "12 – 18.10", "19 – 25.10", "26 – 31.10"}
	if !slices.Equal(labels, want) {
		t.Errorf("labels = %q, want %q", labels, want)
	}
	if view.Rows[0].URL != "/?week=2026-09-28&payments=week" {
		t.Errorf("first row URL = %q, want the whole week from 28.09", view.Rows[0].URL)
	}
	for _, row := range view.Rows {
		if row.IsCurrent {
			t.Errorf("row %q is current, want none (the open week is in November)", row.Label)
		}
	}
}

func TestNewPaymentsViewYear(t *testing.T) {
	current := date(t, "2026-11-09")
	counts := map[string]intentions.DayCount{
		"2025-12-31": {Filled: 9, Amount: 900000},
		"2026-01-01": {Filled: 2, Amount: 20000},
		"2026-03-15": {Filled: 4, Unpaid: 1, Amount: 30000},
		"2026-03-31": {Filled: 1, Amount: 10000},
		"2026-11-10": {Filled: 2, Unpaid: 2},
		"2026-12-31": {Filled: 1, Amount: 5050},
		"2027-01-01": {Filled: 9, Amount: 900000},
	}

	view := newPaymentsView(paymentsQuery{tab: paymentsYear, year: 2026}, current, current, counts)

	if view.Title != "2026" || view.FirstColumn != "Miesiąc" {
		t.Errorf("title, first column = %q, %q", view.Title, view.FirstColumn)
	}
	if view.PrevURL != "/?week=2026-11-09&payments=year&year=2025" || view.NextURL != "/?week=2026-11-09&payments=year&year=2027" {
		t.Errorf("prev, next = %q, %q", view.PrevURL, view.NextURL)
	}
	if view.Total != 65050 || view.Count != 10 || view.Unpaid != 3 {
		t.Errorf("totals = %d / %d / %d unpaid, want 65050 / 10 / 3", view.Total, view.Count, view.Unpaid)
	}
	if len(view.Rows) != 12 {
		t.Fatalf("rows = %d, want 12 months", len(view.Rows))
	}

	checks := []paymentsRowSummary{
		{Label: "Styczeń", URL: "/?week=2026-11-09&payments=month&month=2026-01", Amount: 20000, Count: 2, Width: "50.0%"},
		{Label: "Marzec", URL: "/?week=2026-11-09&payments=month&month=2026-03", Amount: 40000, Count: 5, Unpaid: 1, Width: "100.0%"},
		{Label: "Listopad", URL: "/?week=2026-11-09&payments=month&month=2026-11", Count: 2, Unpaid: 2, Width: "0%", IsCurrent: true},
		{Label: "Grudzień", URL: "/?week=2026-11-09&payments=month&month=2026-12", Amount: 5050, Count: 1, Width: "12.6%"},
	}
	got := summarizeRows(view.Rows)
	for _, want := range checks {
		if !slices.Contains(got, want) {
			t.Errorf("rows do not contain %+v\nrows: %+v", want, got)
		}
	}

	other := newPaymentsView(paymentsQuery{tab: paymentsYear, year: 2025}, current, current, counts)
	for _, row := range other.Rows {
		if row.IsCurrent {
			t.Errorf("2025 row %q is current, want none", row.Label)
		}
	}
	if other.Total != 900000 {
		t.Errorf("2025 total = %d, want only 31.12.2025 (900000)", other.Total)
	}
}

func TestPaymentsAverage(t *testing.T) {
	tests := []struct {
		total  intentions.Grosze
		count  int
		unpaid int
		want   intentions.Grosze
	}{
		{total: 190000, count: 19, unpaid: 3, want: 11900},
		{total: 15050, count: 1, want: 15100},
		{total: 15049, count: 1, want: 15000},
		{total: 30000, count: 4, unpaid: 1, want: 10000},
		{total: 0, count: 3, unpaid: 3, want: 0},
		{total: 0, count: 0, want: 0},
	}

	for _, tt := range tests {
		v := paymentsView{Total: tt.total, Count: tt.count, Unpaid: tt.unpaid}
		if got := v.Average(); got != tt.want {
			t.Errorf("Average(%d over %d, %d unpaid) = %d, want %d", tt.total, tt.count, tt.unpaid, got, tt.want)
		}
	}
}

func TestBarWidth(t *testing.T) {
	tests := []struct {
		amount, top intentions.Grosze
		want        string
	}{
		{amount: 0, top: 0, want: "0%"},
		{amount: 0, top: 100, want: "0%"},
		{amount: 100, top: 100, want: "100.0%"},
		{amount: 1, top: 3, want: "33.3%"},
		{amount: 2, top: 3, want: "66.7%"},
	}

	for _, tt := range tests {
		if got := barWidth(tt.amount, tt.top); got != tt.want {
			t.Errorf("barWidth(%d, %d) = %q, want %q", tt.amount, tt.top, got, tt.want)
		}
	}
}

func TestWeekPayments(t *testing.T) {
	start := date(t, "2026-11-09")
	paid := newIntention(t, 1, "2026-11-09 18:00", false, "+ Jan")
	paid.Amount = 15000
	unpaid := newIntention(t, 2, "2026-11-09 18:00", false, "Za parafian")
	outside := newIntention(t, 3, "2026-11-10 06:30", true, "+ Anna")
	outside.Amount = 10050
	placeholder := newIntention(t, 4, "2026-11-11 06:30", false, "")
	placeholder.IsPlaceholder = true
	laterUnpaid := newIntention(t, 5, "2026-11-12 06:30", true, "+ Józef")

	view := newWeekView(start, []intentions.Intention{paid, unpaid, outside, placeholder, laterUnpaid})

	got := view.WeekPayments()
	if got == nil {
		t.Fatal("WeekPayments() = nil, want a summary")
	}
	if got.Total != 25050 || got.Count != 4 || got.Unpaid != 2 {
		t.Errorf("payments = %d / %d / %d unpaid, want 25050 / 4 / 2 (placeholders do not count)", got.Total, got.Count, got.Unpaid)
	}
	if got.URL != "/?week=2026-11-09&payments=week" {
		t.Errorf("URL = %q", got.URL)
	}
	if got.UnpaidURL != "/?week=2026-11-09&unpaid=1#i-2" {
		t.Errorf("UnpaidURL = %q, want a link that scrolls to the first intention without an amount", got.UnpaidURL)
	}
	if got.Title() != "Podsumowanie wpłat · kwota wpisana przy 2 z 4 intencji" || got.UnpaidLabel() != "2 bez kwoty" {
		t.Errorf("title, label = %q, %q", got.Title(), got.UnpaidLabel())
	}

	view.ShowUnpaid = true
	if got := view.WeekPayments().UnpaidURL; got != "/?week=2026-11-09" {
		t.Errorf("UnpaidURL with highlighting on = %q, want the plain week to turn it off", got)
	}

	if got := newWeekView(start, []intentions.Intention{placeholder}).WeekPayments(); got != nil {
		t.Errorf("WeekPayments() for a week with only placeholders = %+v, want nil", got)
	}
}

func TestMarkUnpaid(t *testing.T) {
	paid := newIntention(t, 1, "2026-11-09 18:00", false, "+ Jan")
	paid.Amount = 15000
	unpaid := newIntention(t, 2, "2026-11-09 18:00", true, "Za parafian")
	placeholder := newIntention(t, 3, "2026-11-10 06:30", false, "")
	placeholder.IsPlaceholder = true

	view := newWeekView(date(t, "2026-11-09"), []intentions.Intention{paid, unpaid, placeholder})
	markUnpaid(view.Days)

	got := map[int]bool{}
	forEachIntention(view.Days, func(intention *intentionView) {
		got[intention.ID] = intention.IsUnpaid
	})
	if !got[2] || got[1] || got[3] {
		t.Errorf("IsUnpaid = %v, want only the intention without an amount (not the placeholder)", got)
	}
}

func TestQuickAmountOptions(t *testing.T) {
	got := quickAmountOptions(settings.DefaultQuickAmounts, "100,00")

	want := []quickAmount{
		{Value: "50", Label: "50 zł"},
		{Value: "100", Label: "100 zł", Pressed: true},
		{Value: "150", Label: "150 zł"},
		{Value: "200", Label: "200 zł"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("quickAmountOptions() = %+v, want %+v", got, want)
	}

	for _, current := range []string{"", "abc", "0"} {
		for _, option := range quickAmountOptions(settings.DefaultQuickAmounts, current) {
			if option.Pressed {
				t.Errorf("quickAmountOptions(%q): %s is pressed, want none", current, option.Value)
			}
		}
	}
}

func TestParseQuickAmounts(t *testing.T) {
	tests := []struct {
		text string
		want []intentions.Grosze
	}{
		{text: "", want: nil},
		{text: " , ;", want: nil},
		{text: "50, 100, 150, 200", want: []intentions.Grosze{5000, 10000, 15000, 20000}},
		{text: "70;100;300 zł", want: []intentions.Grosze{7000, 10000, 30000}},
		{text: "300, 100, 300, 100", want: []intentions.Grosze{30000, 10000}},
		{text: "1 000, 2 000", want: []intentions.Grosze{100000, 200000}},
		{text: "10,20,30,40,50,60", want: []intentions.Grosze{1000, 2000, 3000, 4000, 5000, 6000}},
		{text: "10,20,30,40,50,60,60", want: []intentions.Grosze{1000, 2000, 3000, 4000, 5000, 6000}},
	}

	for _, tt := range tests {
		got, message := parseQuickAmounts(tt.text)
		if message != "" {
			t.Errorf("parseQuickAmounts(%q) message = %q, want none", tt.text, message)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("parseQuickAmounts(%q) = %v, want %v", tt.text, got, tt.want)
		}
	}
}

func TestParseQuickAmountsInvalid(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "50, sto", want: quickAmountsMessage},
		{text: "0, 50", want: quickAmountsMessage},
		{text: "50.50", want: quickAmountsMessage},
		{text: "-50", want: quickAmountsMessage},
		{text: "100000", want: quickAmountsMessage},
		{text: "1, 2, 3, 4, 5, 6, 7", want: "Podaj najwyżej 6 kwot."},
	}

	for _, tt := range tests {
		got, message := parseQuickAmounts(tt.text)
		if message != tt.want || got != nil {
			t.Errorf("parseQuickAmounts(%q) = %v, %q; want nil, %q", tt.text, got, message, tt.want)
		}
	}
}
