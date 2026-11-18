package intentions

import (
	"slices"
	"strconv"
	"testing"
	"time"

	"gorm.io/gorm"
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

func searchContents(t testing.TB, db *gorm.DB, query string) []string {
	t.Helper()

	hits, err := Search(db, query)
	if err != nil {
		t.Fatalf("Search(%q): %v", query, err)
	}
	ids := make([]int, 0, len(hits))
	for _, hit := range hits {
		ids = append(ids, hit.ID)
	}
	loaded, err := ByIDs(db, ids)
	if err != nil {
		t.Fatal(err)
	}

	var result []string
	for _, hit := range hits {
		intention, ok := loaded[hit.ID]
		if !ok || !intention.MassAt.Equal(hit.MassAt) {
			t.Fatalf("hit %+v does not match the stored intention %+v", hit, intention)
		}
		result = append(result, intention.Content)
	}

	return result
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
		if got := searchContents(t, db, query); !slices.Equal(got, want) {
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
		if got := searchContents(t, db, Fold(word)); !slices.Contains(got, "Za "+word) {
			t.Errorf("Search(%q) = %q, missing %q", Fold(word), got, "Za "+word)
		}
	}
}

func TestSearchOrdersByMassThenID(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-23 18:00", "Za + Jana (druga)")
	createIntention(t, db, "2026-11-20 06:30", "Za + Jana (wcześniej)")
	createIntention(t, db, "2026-11-23 18:00", "Za + Jana (trzecia)")

	got := searchContents(t, db, "jana")
	want := []string{"Za + Jana (wcześniej)", "Za + Jana (druga)", "Za + Jana (trzecia)"}
	if !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
}

func TestByIDs(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-23 18:00", "Za + Jana")
	createIntention(t, db, "2026-11-24 18:00", "Za + Annę")

	loaded, err := ByIDs(db, []int{2, 99})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != 1 || loaded[2].Content != "Za + Annę" {
		t.Errorf("ByIDs = %+v, want only intention 2", loaded)
	}
	if loaded, err := ByIDs(db, nil); err != nil || len(loaded) != 0 {
		t.Errorf("ByIDs(nil) = %v, %v", loaded, err)
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
