package web

import (
	"bytes"
	"fmt"
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
	if numbers != 2 {
		t.Errorf("numbers = %d, want 2 (only the 18:00 mass has more than one intention)", numbers)
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
	header := layoutHeader(start, m)
	smallest := pdfStyles[len(pdfStyles)-1]

	for count := 1; count < 300; count++ {
		view := newWeekView(start, intentionsPerMass(t, count))
		if header.Height+totalHeight(layoutDays(view, pdfStyles[0], m)) <= pageContent {
			continue
		}

		if header.Height+totalHeight(layoutDays(view, smallest, m)) > pageContent {
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
