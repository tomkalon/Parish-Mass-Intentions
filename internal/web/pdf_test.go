package web

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"go-parafia/internal/intentions"
)

type fakeMeasurer struct{}

func (fakeMeasurer) width(text string, _ pdfFont, size, spacing float64) float64 {
	runes := float64(utf8.RuneCountInString(text))
	return runes * (size*0.5 + spacing)
}

func pageTexts(page pdfPage) []string {
	texts := make([]string, 0, len(page.Runs))
	for _, run := range page.Runs {
		texts = append(texts, run.Text)
	}

	return texts
}

func TestWeekInWords(t *testing.T) {
	tests := []struct {
		start time.Time
		want  string
	}{
		{start: time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone), want: "2 – 8 listopada 2026"},
		{start: time.Date(2026, 10, 26, 0, 0, 0, 0, intentions.TimeZone), want: "26 października – 1 listopada 2026"},
		{start: time.Date(2026, 12, 28, 0, 0, 0, 0, intentions.TimeZone), want: "28 grudnia 2026 – 3 stycznia 2027"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := weekInWords(tt.start); got != tt.want {
				t.Errorf("weekInWords = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLayoutWeek(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, "+ Jan Kowalski"),
		newIntention(t, 2, "2026-11-02 18:00", false, "+ Anna Nowak"),
		newIntention(t, 3, "2026-11-02 18:00", true, "+ Piotr Wiśniewski"),
	})
	view.ParishName = "Parafia pw. Trójcy Przenajświętszej i św. Leopolda w Rzezawie"

	pages := layoutWeek(view, fakeMeasurer{})

	if len(pages) != 1 {
		t.Fatalf("pages = %d, want 1", len(pages))
	}

	texts := pageTexts(pages[0])
	for _, want := range []string{
		"Parafia pw. Trójcy Przenajświętszej i św. Leopolda w Rzezawie",
		"INTENCJE MSZY ŚWIĘTYCH",
		"2 – 8 listopada 2026",
		"Poniedziałek",
		"2 listopada",
		"6:30",
		"18:00",
		"+ Jan Kowalski",
		"POZA PARAFIĄ · ",
		"+ Piotr Wiśniewski",
		"— brak intencji —",
		"N",
		"IEDZIELA",
		"8 listopada",
	} {
		if !slices.Contains(texts, want) {
			t.Errorf("page does not contain %q", want)
		}
	}

	numbers := 0
	for _, text := range texts {
		if text == "1." || text == "2." {
			numbers++
		}
	}
	if numbers != 0 {
		t.Errorf("numbers = %d, want 0 (at 18:00 the parish and the outside list have one intention each)", numbers)
	}

	for _, text := range texts {
		if strings.Contains(text, "Poza Parafią:") {
			t.Errorf("page contains %q, want the „poza parafią ·” prefix instead", text)
		}
	}
}

func TestLayoutWeekWrapsLongIntentions(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	long := strings.Repeat("słowo ", 80)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, long),
	})

	pages := layoutWeek(view, fakeMeasurer{})

	var lines []pdfRun
	for _, run := range pages[0].Runs {
		if strings.HasPrefix(run.Text, "słowo") {
			lines = append(lines, run)
		}
	}
	if len(lines) < 2 {
		t.Fatalf("intention lines = %d, want at least 2", len(lines))
	}
	for _, line := range lines[1:] {
		if line.X != lines[0].X {
			t.Errorf("continuation line starts at x = %.1f, want %.1f (under the text, not under the number)", line.X, lines[0].X)
		}
	}
}

func intentionsPerMass(t *testing.T, count int) []intentions.Intention {
	t.Helper()

	var list []intentions.Intention
	for k := range count {
		day := 2 + k%7
		hour := 6 + (k/7)%15
		massAt := fmt.Sprintf("2026-11-%02d %02d:00", day, hour)
		list = append(list, newIntention(t, k+1, massAt, false, "+ Jan Kowalski od rodziny"))
	}
	slices.SortFunc(list, func(a, b intentions.Intention) int {
		return a.MassAt.Compare(b.MassAt)
	})

	return list
}

func TestLayoutWeekShrinksBeforeAddingAPage(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	m := fakeMeasurer{}
	header := layoutHeader(newWeekView(start, nil), m)
	smallest := pdfStyles[len(pdfStyles)-1]

	for count := 1; count < 300; count++ {
		view := newWeekView(start, intentionsPerMass(t, count))
		if header.Height+totalHeight(layoutDays(printableDays(view), pdfStyles[0], m)) <= pageContent {
			continue
		}

		if header.Height+totalHeight(layoutDays(printableDays(view), smallest, m)) > pageContent {
			t.Fatalf("with %d intentions even the smallest style does not fit, cannot test shrinking", count)
		}
		if pages := layoutWeek(view, m); len(pages) != 1 {
			t.Errorf("pages = %d, want 1 (the week fits after shrinking)", len(pages))
		}
		return
	}

	t.Fatal("the week never outgrew one page")
}

func TestLayoutWeekKeepsDaysOnOnePage(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, intentionsPerMass(t, 7*15*3))

	pages := layoutWeek(view, fakeMeasurer{})

	if len(pages) < 2 {
		t.Fatalf("pages = %d, want at least 2", len(pages))
	}

	for i, day := range []string{"Poniedziałek", "Wtorek", "Środa", "Czwartek", "Piątek", "Sobota"} {
		dayPage := -1
		datePage := -1
		date := dateInWords(start.AddDate(0, 0, i))
		for p, page := range pages {
			texts := pageTexts(page)
			if slices.Contains(texts, day) {
				dayPage = p
			}
			if slices.Contains(texts, date) {
				datePage = p
			}
		}
		if dayPage < 0 || dayPage != datePage {
			t.Errorf("%s: name on page %d, date on page %d, want both on the same page", day, dayPage, datePage)
		}
	}
}

func TestWriteWeekPDF(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 06:30", false, "++ Władysław Kmiecik w 59 rocznicę śmierci i żona Józefa"),
		newIntention(t, 2, "2026-11-02 06:30", true, "+ Czesława Dziadowiec od koleżanek i kolegów"),
	})

	var buf bytes.Buffer
	err := writeWeekPDF(&buf, view)
	if err != nil {
		t.Fatalf("writeWeekPDF: %v", err)
	}

	if !bytes.HasPrefix(buf.Bytes(), []byte("%PDF-")) {
		t.Errorf("output does not start with %%PDF-: %q", buf.Bytes()[:min(20, buf.Len())])
	}
}

func TestLayoutWeekSkipsPlaceholders(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	placeholder := func(massAt string) intentions.Intention {
		slot := newIntention(t, 0, massAt, false, "")
		slot.IsPlaceholder = true
		return slot
	}
	view := newWeekView(start, []intentions.Intention{
		placeholder("2026-11-02 06:30"),
		placeholder("2026-11-03 18:00"),
		newIntention(t, 3, "2026-11-03 18:00", false, "+ Jan Kowalski"),
	})

	texts := pageTexts(layoutWeek(view, fakeMeasurer{})[0])

	if slices.Contains(texts, "6:30") {
		t.Error("a mass with only a placeholder is printed")
	}
	for _, want := range []string{"18:00", "+ Jan Kowalski"} {
		if !slices.Contains(texts, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	if slices.Contains(texts, "1.") {
		t.Error("the only printed intention is numbered, the placeholder must not count")
	}
	if got := strings.Count(strings.Join(texts, "\n"), "— brak intencji —"); got != 6 {
		t.Errorf(`"— brak intencji —" appears %d times, want 6 (Monday has only a placeholder)`, got)
	}
}

func TestLayoutWeekPrintsFeast(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-04 18:00", false, "+ Jan Kowalski"),
	})
	setFeasts(view.Days, map[string]string{
		"2026-11-04": "wspomnienie św. Karola Boromeusza",
		"2026-11-05": "dzień bez mszy",
	})

	runs := layoutWeek(view, fakeMeasurer{})[0].Runs
	find := func(text string) pdfRun {
		t.Helper()
		i := slices.IndexFunc(runs, func(run pdfRun) bool { return run.Text == text })
		if i < 0 {
			t.Fatalf("page does not contain %q", text)
		}
		return runs[i]
	}

	feast := find("wspomnienie św. Karola Boromeusza")
	mass := find("18:00")
	if feast.Font != fontItalic {
		t.Errorf("feast font = %v, want italic", feast.Font)
	}
	if feast.Baseline >= mass.Baseline {
		t.Errorf("feast baseline %.1f is not above the mass baseline %.1f", feast.Baseline, mass.Baseline)
	}

	quiet := find("dzień bez mszy")
	var empty []pdfRun
	for _, run := range runs {
		if run.Text == "— brak intencji —" && run.Baseline > quiet.Baseline && run.Baseline < quiet.Baseline+30 {
			empty = append(empty, run)
		}
	}
	if len(empty) != 1 {
		t.Errorf("day with a feast and no masses: %d „— brak intencji —” lines right below the feast, want 1", len(empty))
	}
}

func TestLayoutHeader(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		name       string
		parishName string
		address    string
		want       []string
	}{
		{
			name:       "name and address",
			parishName: "Parafia św. Jana",
			address:    "ul. Kościelna 1, 32-765 Rzezawa",
			want:       []string{"Parafia św. Jana", "ul. Kościelna 1, 32-765 Rzezawa", "INTENCJE MSZY ŚWIĘTYCH", "2 – 8 listopada 2026"},
		},
		{
			name:       "name without address",
			parishName: "Parafia św. Jana",
			want:       []string{"Parafia św. Jana", "INTENCJE MSZY ŚWIĘTYCH", "2 – 8 listopada 2026"},
		},
		{
			name: "nothing configured",
			want: []string{"INTENCJE MSZY ŚWIĘTYCH", "2 – 8 listopada 2026"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			view := newWeekView(start, nil)
			view.ParishName = tt.parishName
			view.Address = tt.address

			header := layoutHeader(view, fakeMeasurer{})

			var got []string
			for _, run := range header.Runs {
				got = append(got, run.Text)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("header = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrintedContent(t *testing.T) {
	tests := []struct {
		name      string
		intention intentionView
		want      string
	}{
		{name: "plain", intention: intentionView{Content: "W int. Parafian"}, want: "W int. Parafian"},
		{name: "deceased", intention: intentionView{Content: "Anna Kutyba", IsDeceased: true}, want: "+ Anna Kutyba"},
		{name: "deceased with plus already", intention: intentionView{Content: "++ Jan i Maria", IsDeceased: true}, want: "++ Jan i Maria"},
		{name: "gregorian", intention: intentionView{Content: "+ Anna Kutyba", IsGregorian: true}, want: "+ Anna Kutyba (greg.)"},
		{name: "gregorian already marked", intention: intentionView{Content: "+ Anna Kutyba (Greg.)", IsGregorian: true}, want: "+ Anna Kutyba (Greg.)"},
		{name: "both", intention: intentionView{Content: "Stanisław Grocholski", IsDeceased: true, IsGregorian: true}, want: "+ Stanisław Grocholski (greg.)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := printedContent(tt.intention); got != tt.want {
				t.Errorf("printedContent = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestLayoutWeekPrintsGregorianMark(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	gregorian := newIntention(t, 1, "2026-11-02 18:00", true, "Stanisław Grocholski")
	gregorian.IsDeceased = true
	gregorian.IsGregorian = true

	texts := pageTexts(layoutWeek(newWeekView(start, []intentions.Intention{gregorian}), fakeMeasurer{})[0])

	if !slices.Contains(texts, "+ Stanisław Grocholski (greg.)") {
		t.Errorf("page does not contain the marked gregorian intention: %q", texts)
	}
}

func findRun(t *testing.T, runs []pdfRun, text string) pdfRun {
	t.Helper()

	i := slices.IndexFunc(runs, func(run pdfRun) bool { return run.Text == text })
	if i < 0 {
		t.Fatalf("page does not contain %q", text)
	}

	return runs[i]
}

func TestLayoutWeekNumbersParishAndOutsideSeparately(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	view := newWeekView(start, []intentions.Intention{
		newIntention(t, 1, "2026-11-02 18:00", false, "Parafia A"),
		newIntention(t, 2, "2026-11-02 18:00", false, "Parafia B"),
		newIntention(t, 3, "2026-11-02 18:00", true, "Poza C"),
		newIntention(t, 4, "2026-11-02 18:00", true, "Poza D"),
		newIntention(t, 5, "2026-11-03 18:00", false, "Parafia E"),
		newIntention(t, 6, "2026-11-03 18:00", false, "Parafia F"),
		newIntention(t, 7, "2026-11-03 18:00", true, "Poza G"),
	})

	runs := layoutWeek(view, fakeMeasurer{})[0].Runs

	var numbers []string
	for _, run := range runs {
		if strings.HasSuffix(run.Text, ".") && run.Font == fontRegular && len(run.Text) <= 3 {
			numbers = append(numbers, run.Text)
		}
	}
	if want := []string{"1.", "2.", "1.", "2.", "1.", "2."}; !slices.Equal(numbers, want) {
		t.Errorf("numbers = %q, want %q (parish and outside counted separately, a single intention not numbered)", numbers, want)
	}

	a, b, c := findRun(t, runs, "Parafia A"), findRun(t, runs, "Parafia B"), findRun(t, runs, "Poza C")
	if got, want := c.Baseline-b.Baseline, b.Baseline-a.Baseline+groupGap; math.Abs(got-want) > 0.01 {
		t.Errorf("space before the outside list = %.2f, want %.2f (one line plus a small gap)", got, want)
	}

	e := findRun(t, runs, "Parafia E")
	var prefix pdfRun
	for _, run := range runs {
		if run.Text == "POZA PARAFIĄ · " && run.Baseline > e.Baseline {
			prefix = run
			break
		}
	}
	if prefix.X != e.X {
		t.Errorf("unnumbered outside intention starts at x = %.1f, want %.1f (aligned with the numbered parish ones)", prefix.X, e.X)
	}
}

func TestPrintedDaySplit(t *testing.T) {
	mass := func(time string, contents ...string) printedMass {
		m := printedMass{time: time, numbered: len(contents) > 1}
		for i, content := range contents {
			m.intentions = append(m.intentions, printedIntention{content: content, number: i + 1})
		}
		return m
	}
	contents := func(masses []printedMass) []string {
		var list []string
		for _, m := range masses {
			for _, intention := range m.intentions {
				list = append(list, m.time+" "+intention.content)
			}
		}
		return list
	}

	day := printedDay{
		feast:     "uroczystość",
		masses:    []printedMass{mass("7:00", "A", "B"), mass("9:00", "C", "D", "E")},
		ruleBelow: true,
	}

	tests := []struct {
		n         int
		wantFirst []string
		wantRest  []string
	}{
		{n: 2, wantFirst: []string{"7:00 A", "7:00 B"}, wantRest: []string{"9:00 C", "9:00 D", "9:00 E"}},
		{n: 3, wantFirst: []string{"7:00 A", "7:00 B", "9:00 C"}, wantRest: []string{"9:00 D", "9:00 E"}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.n), func(t *testing.T) {
			first, rest := day.split(tt.n)

			if got := contents(first.masses); !slices.Equal(got, tt.wantFirst) {
				t.Errorf("first = %q, want %q", got, tt.wantFirst)
			}
			if got := contents(rest.masses); !slices.Equal(got, tt.wantRest) {
				t.Errorf("rest = %q, want %q", got, tt.wantRest)
			}
			if first.feast != "uroczystość" || first.continued || first.ruleBelow {
				t.Errorf("first = feast %q, continued %v, rule %v; want the feast, not continued, no rule (the day goes on)", first.feast, first.continued, first.ruleBelow)
			}
			if rest.feast != "" || !rest.continued || !rest.ruleBelow {
				t.Errorf("rest = feast %q, continued %v, rule %v; want no feast, continued, the day's rule", rest.feast, rest.continued, rest.ruleBelow)
			}
		})
	}

	_, rest := day.split(3)
	if got := rest.masses[0].intentions[0].number; got != 2 {
		t.Errorf("first number after the split = %d, want 2 (numbering goes on)", got)
	}
	if day.masses[1].intentions[0].content != "C" {
		t.Error("split changed the original day")
	}
}

func TestLayoutWeekSplitsDayTallerThanAPage(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)
	list := []intentions.Intention{newIntention(t, 1, "2026-11-07 18:00", false, "Sobota")}
	for k := range 90 {
		massAt := fmt.Sprintf("2026-11-08 %02d:00", 7+k/15)
		list = append(list, newIntention(t, k+2, massAt, false, fmt.Sprintf("Intencja %d", k)))
	}
	view := newWeekView(start, list)

	pages := layoutWeek(view, fakeMeasurer{})

	if len(pages) < 2 {
		t.Fatalf("pages = %d, want at least 2", len(pages))
	}

	var all []string
	var numbers []string
	for p, page := range pages {
		for _, run := range page.Runs {
			if run.Baseline > marginTop+pageContent {
				t.Errorf("page %d: %q at %.0f is below the bottom margin (%.0f)", p+1, run.Text, run.Baseline, marginTop+pageContent)
			}
			all = append(all, run.Text)
			if run.Font == fontRegular && strings.HasSuffix(run.Text, ".") && len(run.Text) <= 3 {
				numbers = append(numbers, run.Text)
			}
		}
	}

	for k := range 90 {
		if got := slices.Index(all, fmt.Sprintf("Intencja %d", k)); got < 0 {
			t.Errorf("Intencja %d is missing", k)
		}
	}

	var want []string
	for range 6 {
		for n := 1; n <= 15; n++ {
			want = append(want, fmt.Sprintf("%d.", n))
		}
	}
	if !slices.Equal(numbers, want) {
		t.Errorf("numbers = %q, want 1.–15. for each of the 6 masses", numbers)
	}

	first := pageTexts(pages[0])
	if !slices.Contains(first, "Sobota") || !slices.Contains(first, "Intencja 0") {
		t.Error("Sunday does not start on the first page right after Saturday")
	}
	for p, page := range pages[1:] {
		texts := pageTexts(page)
		if !slices.Contains(texts, "ciąg dalszy") || !slices.Contains(texts, "IEDZIELA") {
			t.Errorf("page %d does not start with „Niedziela – ciąg dalszy”", p+2)
		}
	}
}
