package intentions

import (
	"slices"
	"strconv"
	"testing"
	"time"
)

func TestFold(t *testing.T) {
	tests := map[string]string{
		"Za + Łukasza":             "za + lukasza",
		"ZAŻÓŁĆ GĘŚLĄ JAŹŃ":        "zazolc gesla jazn",
		"  Jana \t\n Kowalskiego ": "jana kowalskiego",
		"":                         "",
	}

	for input, want := range tests {
		if got := Fold(input); got != want {
			t.Errorf("Fold(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSearchReady(t *testing.T) {
	tests := map[string]bool{
		"":      false,
		"ab":    false,
		"  ab ": false,
		"żół":   true,
		"a b":   true,
		"jan":   true,
	}

	for query, want := range tests {
		if got := SearchReady(query); got != want {
			t.Errorf("SearchReady(%q) = %v, want %v", query, got, want)
		}
	}
}

func TestFind(t *testing.T) {
	tests := []struct {
		content string
		query   string
		want    [][2]int
	}{
		{content: "Za + Jana Kowalskiego", query: "kowal", want: [][2]int{{10, 15}}},
		{content: "Za + Łukasza", query: "lukasz", want: [][2]int{{5, 12}}},
		{content: "Za + Łukasza", query: "ŁUKASZ", want: [][2]int{{5, 12}}},
		{content: "Za Annę i Anielę", query: "ane", want: nil},
		{content: "Za Annę i Anielę", query: "an", want: [][2]int{{3, 5}, {11, 13}}},
		{content: "Jana  Kowalskiego", query: "jana kowal", want: [][2]int{{0, 11}}},
		{content: "aaaa", query: "aa", want: [][2]int{{0, 2}, {2, 4}}},
		{content: "Za Stefana", query: "", want: nil},
	}

	for _, test := range tests {
		got := Find(test.content, test.query)
		if !slices.Equal(got, test.want) {
			t.Errorf("Find(%q, %q) = %v, want %v", test.content, test.query, got, test.want)
		}
		for _, r := range got {
			if Fold(test.content[r[0]:r[1]]) != Fold(test.query) {
				t.Errorf("Find(%q, %q): range %v covers %q", test.content, test.query, r, test.content[r[0]:r[1]])
			}
		}
	}
}

func TestSearch(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-23 18:00", "Za + Łukasza Żółtowskiego")
	createIntention(t, db, "2026-11-20 06:30", "W intencji LUKASZA w dniu urodzin")
	createIntention(t, db, "2026-11-21 06:30", "Za + Jana")
	createIntention(t, db, "2025-01-05 10:00", "Dziękczynna – Łukasz i Anna")
	createIntention(t, db, "2026-11-24 18:00", "Za [Łukasza*] ?")
	createPlaceholder(t, db, "2026-11-25 06:30")

	tests := map[string][]string{
		"łukasz": {"Dziękczynna – Łukasz i Anna", "W intencji LUKASZA w dniu urodzin", "Za + Łukasza Żółtowskiego", "Za [Łukasza*] ?"},
		"ZOLT":   {"Za + Łukasza Żółtowskiego"},
		"kasz":   {"Dziękczynna – Łukasz i Anna", "W intencji LUKASZA w dniu urodzin", "Za + Łukasza Żółtowskiego", "Za [Łukasza*] ?"},
		"[łuk":   {"Za [Łukasza*] ?"},
		"a*] ?":  {"Za [Łukasza*] ?"},
		"za + j": {"Za + Jana"},
		"dziek":  {"Dziękczynna – Łukasz i Anna"},
		"marek":  nil,
		"ja":     nil,
		"   ":    nil,
	}

	for query, want := range tests {
		list, err := Search(db, query)
		if err != nil {
			t.Fatalf("Search(%q): %v", query, err)
		}
		if got := contents(list); !slices.Equal(got, want) {
			t.Errorf("Search(%q) = %q, want %q", query, got, want)
		}
	}
}

func TestSearchPrefilterKeepsEveryMatch(t *testing.T) {
	db := newTestDB(t)
	words := []string{"Ąnna", "ĆMA", "Ęla", "ŁĄKA", "Ńomo", "ÓSMY", "ŚWIT", "ŹRÓDŁO", "ŻABA", "zażółć"}
	for i, word := range words {
		createIntention(t, db, "2026-11-2"+strconv.Itoa(i%7+1)+" 18:00", "Za "+word)
	}

	for _, word := range words {
		list, err := Search(db, Fold(word))
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(contents(list), "Za "+word) {
			t.Errorf("Search(%q) = %q, missing %q", Fold(word), contents(list), "Za "+word)
		}
	}
}

func BenchmarkSearch(b *testing.B) {
	db := newTestDB(b)
	start := time.Date(2000, 1, 1, 6, 30, 0, 0, TimeZone)
	names := []string{"Jana", "Marii", "Stefana", "Zofii", "Łukasza", "Anny", "Józefa", "Krystyny"}
	batch := make([]Intention, 0, 1000)
	for i := range 150_000 {
		batch = append(batch, Intention{MassAt: start.Add(time.Duration(i) * 90 * time.Minute), Content: "Za + " + names[i%len(names)] + " i zmarłych z rodziny nr " + strconv.Itoa(i)})
		if len(batch) == cap(batch) {
			if err := db.Create(&batch).Error; err != nil {
				b.Fatal(err)
			}
			batch = batch[:0]
		}
	}

	for _, query := range []string{"nr 14999", "łukasza", "zmarłych"} {
		b.Run(query, func(b *testing.B) {
			for b.Loop() {
				if _, err := Search(db, query); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
