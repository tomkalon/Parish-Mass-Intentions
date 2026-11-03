package intentions

import (
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&Intention{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func createIntention(t *testing.T, db *gorm.DB, massAt, content string) {
	t.Helper()

	at, err := time.ParseInLocation(MassAtLayout, massAt, TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", massAt, err)
	}

	err = db.Create(&Intention{MassAt: at, Content: content}).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}
}

func contents(list []Intention) []string {
	result := make([]string, 0, len(list))
	for _, intention := range list {
		result = append(result, intention.Content)
	}

	return result
}

func TestForWeek(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-01 23:59", "sunday before")
	createIntention(t, db, "2026-11-02 18:00", "monday evening")
	createIntention(t, db, "2026-11-08 23:45", "sunday night")
	createIntention(t, db, "2026-11-02 00:00", "monday midnight")
	createIntention(t, db, "2026-11-09 00:00", "next monday")
	createIntention(t, db, "2026-11-02 18:00", "monday evening, added later")

	start := time.Date(2026, 11, 2, 0, 0, 0, 0, TimeZone)
	want := []string{"monday midnight", "monday evening", "monday evening, added later", "sunday night"}

	tests := []struct {
		name  string
		start time.Time
	}{
		{name: "start in Polish time", start: start},
		{name: "start in UTC", start: start.UTC()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := ForWeek(db, tt.start)
			if err != nil {
				t.Fatalf("ForWeek: %v", err)
			}

			if got := contents(list); !slices.Equal(got, want) {
				t.Errorf("ForWeek = %q, want %q", got, want)
			}
		})
	}
}

func TestForWeekAcrossDSTChange(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-10-19 00:00", "monday")
	createIntention(t, db, "2026-10-25 23:30", "sunday after the change")
	createIntention(t, db, "2026-10-26 00:00", "next monday")

	list, err := ForWeek(db, time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone))
	if err != nil {
		t.Fatalf("ForWeek: %v", err)
	}

	want := []string{"monday", "sunday after the change"}
	if got := contents(list); !slices.Equal(got, want) {
		t.Errorf("ForWeek = %q, want %q", got, want)
	}
}

func TestDeleteWeek(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-11-01 23:59", "sunday before")
	createIntention(t, db, "2026-11-02 00:00", "monday midnight")
	createIntention(t, db, "2026-11-05 18:00", "thursday")
	createIntention(t, db, "2026-11-08 23:45", "sunday night")
	createIntention(t, db, "2026-11-09 00:00", "next monday")

	err := DeleteWeek(db, time.Date(2026, 11, 2, 0, 0, 0, 0, TimeZone).UTC())
	if err != nil {
		t.Fatalf("DeleteWeek: %v", err)
	}

	var left []Intention
	err = db.Order("mass_at").Find(&left).Error
	if err != nil {
		t.Fatalf("loading intentions: %v", err)
	}

	want := []string{"sunday before", "next monday"}
	if got := contents(left); !slices.Equal(got, want) {
		t.Errorf("left after DeleteWeek = %q, want %q", got, want)
	}
}

func TestCountByDay(t *testing.T) {
	db := newTestDB(t)
	createIntention(t, db, "2026-09-30 23:45", "day before the range")
	createIntention(t, db, "2026-10-01 00:00", "first minute")
	createIntention(t, db, "2026-10-01 00:30", "still 30 September in UTC")
	createPlaceholder(t, db, "2026-10-01 18:00")
	createIntention(t, db, "2026-10-25 23:45", "day of the DST change")
	createIntention(t, db, "2026-11-01 00:00", "day after the range")

	err := db.Create(&Intention{MassAt: time.Date(2026, 10, 26, 7, 0, 0, 0, TimeZone), Content: "hidden", IsHidden: true}).Error
	if err != nil {
		t.Fatalf("creating hidden intention: %v", err)
	}

	from := time.Date(2026, 10, 1, 0, 0, 0, 0, TimeZone)
	got, err := CountByDay(db, from, from.AddDate(0, 1, 0))
	if err != nil {
		t.Fatalf("CountByDay() error = %v", err)
	}

	want := map[string]DayCount{
		"2026-10-01": {Filled: 2, Placeholders: 1, Unpaid: 2},
		"2026-10-25": {Filled: 1, Unpaid: 1},
		"2026-10-26": {Filled: 1, Unpaid: 1},
	}
	if !maps.Equal(got, want) {
		t.Errorf("CountByDay() = %v, want %v", got, want)
	}
}

func TestCountByDaySumsAmounts(t *testing.T) {
	db := newTestDB(t)
	day := time.Date(2026, 11, 9, 0, 0, 0, 0, TimeZone)
	err := db.Create(&[]Intention{
		{MassAt: day.Add(6*time.Hour + 30*time.Minute), Content: "+ Stanisław", Amount: 10000},
		{MassAt: day.Add(18 * time.Hour), Content: "+ Helena", Amount: 15050},
		{MassAt: day.Add(18 * time.Hour), Content: "Za parafian"},
		{MassAt: day.Add(18 * time.Hour), Content: "ukryta, ale opłacona", Amount: 5000, IsHidden: true},
		{MassAt: day.Add(20 * time.Hour), IsPlaceholder: true},
		{MassAt: day.AddDate(0, 0, 1).Add(7 * time.Hour), Content: "+ Józef", Amount: 20000, IsOutsideParish: true},
	}).Error
	if err != nil {
		t.Fatalf("creating intentions: %v", err)
	}

	got, err := CountByDay(db, day, day.AddDate(0, 0, 7))
	if err != nil {
		t.Fatalf("CountByDay() error = %v", err)
	}

	want := map[string]DayCount{
		"2026-11-09": {Filled: 4, Placeholders: 1, Unpaid: 1, Amount: 30050},
		"2026-11-10": {Filled: 1, Amount: 20000},
	}
	if !maps.Equal(got, want) {
		t.Errorf("CountByDay() = %v, want %v", got, want)
	}
}

func TestCountByDayQueryUsesMassAtIndex(t *testing.T) {
	db := newTestDB(t)
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, TimeZone)
	query := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return inRange(tx, from, from.AddDate(1, 0, 0)).Select("mass_at", "is_placeholder", "amount").Find(&[]Intention{})
	})

	var plan []struct{ Detail string }
	err := db.Raw("EXPLAIN QUERY PLAN " + query).Scan(&plan).Error
	if err != nil {
		t.Fatalf("explaining %q: %v", query, err)
	}

	var details []string
	for _, step := range plan {
		details = append(details, step.Detail)
	}
	if got := strings.Join(details, "; "); !strings.Contains(got, "USING INDEX idx_intentions_mass_at") {
		t.Errorf("summary query plan = %q, want a search using idx_intentions_mass_at (a year summary must not read the whole table)", got)
	}
}

func createPlaceholder(t *testing.T, db *gorm.DB, massAt string) {
	t.Helper()

	at, err := time.ParseInLocation(MassAtLayout, massAt, TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", massAt, err)
	}

	err = db.Create(&Intention{MassAt: at, IsPlaceholder: true}).Error
	if err != nil {
		t.Fatalf("creating placeholder: %v", err)
	}
}

func TestSaveReplacesPlaceholderAtSameMass(t *testing.T) {
	sunday := time.Date(2026, 11, 8, 7, 0, 0, 0, TimeZone)

	tests := []struct {
		name   string
		massAt time.Time
	}{
		{name: "mass time in Polish time", massAt: sunday},
		{name: "mass time in UTC", massAt: sunday.UTC()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := newTestDB(t)
			createPlaceholder(t, db, "2026-11-08 07:00")
			createIntention(t, db, "2026-11-08 07:00", "already there")
			createPlaceholder(t, db, "2026-11-08 08:30")
			createPlaceholder(t, db, "2026-11-09 07:00")

			err := Save(db, &Intention{MassAt: tt.massAt, Content: "Jan Nowak"})
			if err != nil {
				t.Fatalf("Save: %v", err)
			}

			var left []Intention
			err = db.Order("mass_at, id").Find(&left).Error
			if err != nil {
				t.Fatalf("loading intentions: %v", err)
			}

			var got []string
			for _, intention := range left {
				label := intention.Content
				if intention.IsPlaceholder {
					label = "placeholder"
				}
				got = append(got, intention.MassAt.In(TimeZone).Format(MassAtLayout)+" "+label)
			}

			want := []string{
				"2026-11-08 07:00 already there",
				"2026-11-08 07:00 Jan Nowak",
				"2026-11-08 08:30 placeholder",
				"2026-11-09 07:00 placeholder",
			}
			if !slices.Equal(got, want) {
				t.Errorf("after Save = %q, want %q", got, want)
			}
		})
	}
}

func TestWeekQueryUsesMassAtIndex(t *testing.T) {
	db := newTestDB(t)
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, TimeZone)
	query := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return inWeek(tx, start).Order("mass_at, id").Find(&[]Intention{})
	})

	var plan []struct{ Detail string }
	err := db.Raw("EXPLAIN QUERY PLAN " + query).Scan(&plan).Error
	if err != nil {
		t.Fatalf("explaining %q: %v", query, err)
	}

	var details []string
	for _, step := range plan {
		details = append(details, step.Detail)
	}
	got := strings.Join(details, "; ")
	if !strings.Contains(got, "USING INDEX idx_intentions_mass_at") {
		t.Errorf("week query plan = %q, want a search using idx_intentions_mass_at (otherwise every week reads the whole table)", got)
	}
	if strings.Contains(got, "TEMP B-TREE") {
		t.Errorf("week query plan = %q, want the order taken from the index, not a temporary sort", got)
	}
}

func TestCountByDayCountsSeriesPaymentOnce(t *testing.T) {
	db := newTestDB(t)
	day := time.Date(2026, 11, 9, 18, 0, 0, 0, TimeZone)
	var list []Intention
	for i := range 3 {
		list = append(list, Intention{MassAt: day.AddDate(0, 0, i), Content: "+ Jan", IsGregorian: true, SeriesID: 1})
	}
	list[0].Amount = 150000
	for i := range 2 {
		list = append(list, Intention{MassAt: day.AddDate(0, 0, i).Add(-11 * time.Hour), Content: "+ Anna", IsGregorian: true, SeriesID: 2})
	}
	list = append(list, Intention{MassAt: day.AddDate(0, 0, -7), Content: "+ Józef", IsGregorian: true, SeriesID: 3, Amount: 120000})
	list = append(list, Intention{MassAt: day, Content: "+ Józef", IsGregorian: true, SeriesID: 3})
	err := db.Create(&list).Error
	if err != nil {
		t.Fatalf("creating intentions: %v", err)
	}

	got, err := CountByDay(db, day.AddDate(0, 0, -1), day.AddDate(0, 0, 6))
	if err != nil {
		t.Fatalf("CountByDay() error = %v", err)
	}

	want := map[string]DayCount{
		"2026-11-09": {Filled: 3, Unpaid: 1, Amount: 150000},
		"2026-11-10": {Filled: 2},
		"2026-11-11": {Filled: 1},
	}
	if !maps.Equal(got, want) {
		t.Errorf("CountByDay() = %v, want %v (only the unpaid first mass of a series counts as bez kwoty)", got, want)
	}
}
