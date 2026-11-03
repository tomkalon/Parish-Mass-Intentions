package gregorian

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	err = db.AutoMigrate(&intentions.Intention{}, &Series{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	return db
}

func at(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.ParseInLocation(intentions.MassAtLayout, value, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing %q: %v", value, err)
	}

	return parsed
}

func seedSeries(t *testing.T, db *gorm.DB, content string, first time.Time, days int, skip ...string) Series {
	t.Helper()

	series, err := Create(db, content, false)
	if err != nil {
		t.Fatalf("creating series: %v", err)
	}
	for day := first; days > 0; day = day.AddDate(0, 0, 1) {
		if slices.Contains(skip, day.Format(intentions.DateLayout)) {
			continue
		}
		intention := intentions.Intention{MassAt: day, Content: content, IsGregorian: true, IsDeceased: true, SeriesID: series.ID}
		if days == 30 || len(skip) == 0 && day.Equal(first) {
			intention.Amount = 150000
		}
		err = db.Create(&intention).Error
		if err != nil {
			t.Fatalf("creating mass: %v", err)
		}
		days--
	}

	return series
}

func TestEaster(t *testing.T) {
	tests := map[int]string{
		2024: "2024-03-31",
		2025: "2025-04-20",
		2026: "2026-04-05",
		2027: "2027-03-28",
		2038: "2038-04-25",
		2285: "2285-03-22",
	}

	for year, want := range tests {
		if got := Easter(year).Format(intentions.DateLayout); got != want {
			t.Errorf("Easter(%d) = %s, want %s", year, got, want)
		}
	}
}

func TestNextMassDaySkipsGoodFridayAndHolySaturday(t *testing.T) {
	tests := []struct{ day, want string }{
		{day: "2026-11-09", want: "2026-11-10"},
		{day: "2026-12-31", want: "2027-01-01"},
		{day: "2026-04-01", want: "2026-04-02"},
		{day: "2026-04-02", want: "2026-04-05"},
		{day: "2026-04-03", want: "2026-04-05"},
	}

	for _, tt := range tests {
		day, err := time.ParseInLocation(intentions.DateLayout, tt.day, intentions.TimeZone)
		if err != nil {
			t.Fatal(err)
		}
		if got := NextMassDay(day).Format(intentions.DateLayout); got != tt.want {
			t.Errorf("NextMassDay(%s) = %s, want %s", tt.day, got, tt.want)
		}
	}
}

func TestLoadSummary(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Jan Kowalski", at(t, "2026-10-27 18:00"), 17)

	got, err := Load(db, series.ID, at(t, "2026-11-03 16:00"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if got.Count() != 17 || got.Done != 7 || got.Status != StatusActive {
		t.Errorf("count, done, status = %d, %d, %d; want 17, 7, Active", got.Count(), got.Done, got.Status)
	}
	if got.Amount() != 150000 || got.FirstID() != 1 {
		t.Errorf("amount, first = %d, %d; want 150000, 1", got.Amount(), got.FirstID())
	}
	if d := got.NextDay.Format(intentions.DateLayout); d != "2026-11-13" {
		t.Errorf("NextDay = %s, want 2026-11-13", d)
	}
	if d := got.End.Format(intentions.DateLayout); d != "2026-11-25" {
		t.Errorf("End = %s, want 2026-11-25", d)
	}
	if got.NextTime() != "18:00" || len(got.Gaps) != 0 {
		t.Errorf("next time, gaps = %q, %v", got.NextTime(), got.Gaps)
	}
	if got.StepOn("2026-11-13") != 18 || got.StepOn("2026-10-27") != 1 {
		t.Errorf("StepOn = %d, %d; want 18, 1", got.StepOn("2026-11-13"), got.StepOn("2026-10-27"))
	}
	if !got.HasDay("2026-11-12") || got.HasDay("2026-11-13") {
		t.Error("HasDay is wrong around the last mass")
	}
	if planned := got.Planned(); len(planned) != 13 {
		t.Errorf("planned days = %d, want 13", len(planned))
	}
}

func TestLoadSummaryWithGap(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Henryk Zieliński", at(t, "2026-10-19 18:00"), 20, "2026-11-05")

	got, err := Load(db, series.ID, at(t, "2026-11-03 16:00"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if len(got.Gaps) != 1 || got.Gaps[0].Format(intentions.DateLayout) != "2026-11-05" {
		t.Errorf("Gaps = %v, want [2026-11-05]", got.Gaps)
	}
	if !slices.Equal(got.GapsAfter, []int{17}) {
		t.Errorf("GapsAfter = %v, want [17]", got.GapsAfter)
	}
	if d := got.End.Format(intentions.DateLayout); d != "2026-11-18" {
		t.Errorf("End = %s, want 2026-11-18 (one day later because of the gap)", d)
	}
	if got.StepOn("2026-11-05") != 18 {
		t.Errorf("StepOn(gap) = %d, want 18", got.StepOn("2026-11-05"))
	}
}

func TestSummaryAcrossTriduumHasNoGap(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Maria", at(t, "2026-03-30 18:00"), 6, "2026-04-03", "2026-04-04")

	got, err := Load(db, series.ID, at(t, "2026-03-01 10:00"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Gaps) != 0 {
		t.Errorf("Gaps = %v, want none over Good Friday and Holy Saturday", got.Gaps)
	}
}

func TestStatusesAndLists(t *testing.T) {
	db := newTestDB(t)
	now := at(t, "2026-11-03 16:00")
	active := seedSeries(t, db, "+ Jan", at(t, "2026-10-27 18:00"), 17)
	finished := seedSeries(t, db, "+ Maria", at(t, "2026-10-02 18:00"), 30)
	scheduled := seedSeries(t, db, "+ Anna", at(t, "2026-10-20 06:30"), 30)
	stopped := seedSeries(t, db, "+ Józef", at(t, "2026-10-25 07:00"), 5)
	err := Stop(db, stopped.ID, now)
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}

	list, err := Active(db, now)
	if err != nil {
		t.Fatalf("Active: %v", err)
	}
	var ids []int
	for _, s := range list {
		ids = append(ids, s.Series.ID)
	}
	if !slices.Equal(ids, []int{scheduled.ID, active.ID}) {
		t.Errorf("active = %v, want [%d %d] ordered by first mass (stopped and finished left out)", ids, scheduled.ID, active.ID)
	}
	if !list[0].NextDay.IsZero() {
		t.Errorf("series with all 30 masses has NextDay %v, want none", list[0].NextDay)
	}

	closed, total, err := Closed(db, now, 50)
	if err != nil {
		t.Fatalf("Closed: %v", err)
	}
	if total != 2 || len(closed) != 2 {
		t.Fatalf("closed = %d (total %d), want 2", len(closed), total)
	}
	if count, err := CountClosed(db, now); err != nil || count != 2 {
		t.Errorf("CountClosed = %d, %v; want 2", count, err)
	}
	statuses := map[int]Status{}
	for _, s := range closed {
		statuses[s.Series.ID] = s.Status
	}
	if statuses[finished.ID] != StatusFinished || statuses[stopped.ID] != StatusStopped {
		t.Errorf("statuses = %v, want finished and stopped", statuses)
	}

	if err := Stop(db, 999, now); !errors.Is(err, ErrSeriesNotFound) {
		t.Errorf("Stop(missing) = %v, want ErrSeriesNotFound", err)
	}
	if err := Stop(db, stopped.ID, now.Add(time.Hour)); err != nil {
		t.Errorf("stopping twice: %v", err)
	}
}

func TestSteps(t *testing.T) {
	db := newTestDB(t)
	a := seedSeries(t, db, "+ Jan", at(t, "2026-11-01 18:00"), 3)
	b := seedSeries(t, db, "+ Anna", at(t, "2026-10-31 06:30"), 2)

	got, err := Steps(db, []int{a.ID, b.ID})
	if err != nil {
		t.Fatalf("Steps: %v", err)
	}
	want := map[int]int{1: 1, 2: 2, 3: 3, 4: 1, 5: 2}
	for id, step := range want {
		if got[id] != step {
			t.Errorf("step of %d = %d, want %d", id, got[id], step)
		}
	}

	empty, err := Steps(db, nil)
	if err != nil || len(empty) != 0 {
		t.Errorf("Steps(nil) = %v, %v", empty, err)
	}
}

func TestRename(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Jan", at(t, "2026-11-01 18:00"), 3)

	err := Rename(db, series.ID, "+ Jan Kowalski")
	if err != nil {
		t.Fatalf("Rename: %v", err)
	}

	var contents []string
	err = db.Model(&intentions.Intention{}).Where("series_id = ?", series.ID).Pluck("content", &contents).Error
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(contents, []string{"+ Jan Kowalski", "+ Jan Kowalski", "+ Jan Kowalski"}) {
		t.Errorf("contents = %q", contents)
	}
	got, err := Find(db, series.ID)
	if err != nil || got.Content != "+ Jan Kowalski" {
		t.Errorf("series = %+v, %v", got, err)
	}
}

func amounts(t *testing.T, db *gorm.DB, id int) []intentions.Grosze {
	t.Helper()

	var list []intentions.Grosze
	err := db.Model(&intentions.Intention{}).Where("series_id = ?", id).Order("mass_at, id").Pluck("amount", &list).Error
	if err != nil {
		t.Fatal(err)
	}

	return list
}

func TestNormalizeKeepsAmountOnFirstMass(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Jan", at(t, "2026-11-01 18:00"), 3)

	err := db.Model(&intentions.Intention{}).Where("id = ?", 1).Update("mass_at", at(t, "2026-11-05 18:00")).Error
	if err != nil {
		t.Fatal(err)
	}
	err = Normalize(db, series.ID)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}

	if got := amounts(t, db, series.ID); !slices.Equal(got, []intentions.Grosze{150000, 0, 0}) {
		t.Errorf("amounts = %v, want the offering moved to the new first mass", got)
	}
}

func TestReleaseMovesAmountAndDeletesEmptySeries(t *testing.T) {
	db := newTestDB(t)
	series := seedSeries(t, db, "+ Jan", at(t, "2026-11-01 18:00"), 3)

	var first intentions.Intention
	err := db.First(&first, 1).Error
	if err != nil {
		t.Fatal(err)
	}
	err = db.Delete(&first).Error
	if err != nil {
		t.Fatal(err)
	}
	err = Release(db, []intentions.Intention{first})
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if got := amounts(t, db, series.ID); !slices.Equal(got, []intentions.Grosze{150000, 0}) {
		t.Errorf("amounts = %v, want the offering moved to the next mass", got)
	}

	var rest []intentions.Intention
	err = db.Where("series_id = ?", series.ID).Find(&rest).Error
	if err != nil {
		t.Fatal(err)
	}
	err = db.Delete(&rest).Error
	if err != nil {
		t.Fatal(err)
	}
	err = Release(db, rest)
	if err != nil {
		t.Fatalf("Release: %v", err)
	}
	if _, err := Find(db, series.ID); !errors.Is(err, ErrSeriesNotFound) {
		t.Errorf("Find after deleting every mass = %v, want ErrSeriesNotFound", err)
	}

	if err := Release(db, []intentions.Intention{{ID: 50, Amount: 100}}); err != nil {
		t.Errorf("Release without a series: %v", err)
	}
}

func TestActiveQueryUsesSeriesIndex(t *testing.T) {
	db := newTestDB(t)
	query := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		var ids []int
		return tx.Model(&intentions.Intention{}).Where("series_id > 0").Group("series_id").
			Having("COUNT(*) < ? OR MAX(mass_at) >= ?", Length, time.Now()).Pluck("series_id", &ids)
	})

	var plan []struct{ Detail string }
	err := db.Raw("EXPLAIN QUERY PLAN " + query).Scan(&plan).Error
	if err != nil {
		t.Fatalf("explaining %q: %v", query, err)
	}
	found := false
	for _, step := range plan {
		if slices.Contains([]string{"SEARCH intentions USING COVERING INDEX idx_intentions_series_mass (series_id>?)"}, step.Detail) {
			found = true
		}
	}
	if !found {
		t.Errorf("plan = %+v, want a covering search on idx_intentions_series_mass (no table lookups)", plan)
	}
}
