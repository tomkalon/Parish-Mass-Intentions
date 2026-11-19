package trash

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

var testNow = time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}
	err = db.AutoMigrate(&intentions.Intention{}, &gregorian.Series{}, &Entry{})
	if err != nil {
		t.Fatalf("migrating: %v", err)
	}
	err = masses.Migrate(db)
	if err != nil {
		t.Fatalf("migrating masses: %v", err)
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

func create(t *testing.T, db *gorm.DB, intention intentions.Intention) intentions.Intention {
	t.Helper()

	err := db.Create(&intention).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}

	return intention
}

func remove(t *testing.T, db *gorm.DB, list []intentions.Intention, week time.Time, now time.Time) int64 {
	t.Helper()

	ids := make([]int, 0, len(list))
	for _, intention := range list {
		ids = append(ids, intention.ID)
	}

	var batch int64
	err := db.Transaction(func(tx *gorm.DB) error {
		var fresh []intentions.Intention
		err := tx.Order("mass_at, id").Find(&fresh, ids).Error
		if err != nil {
			return err
		}
		list = fresh

		batch, err = Move(tx, list, week, now)
		if err != nil {
			return err
		}
		for _, intention := range list {
			err = tx.Delete(&intentions.Intention{}, intention.ID).Error
			if err != nil {
				return err
			}
		}
		if !week.IsZero() {
			err = masses.Unmark(tx, week)
			if err != nil {
				return err
			}
		}

		return gregorian.Release(tx, list)
	})
	if err != nil {
		t.Fatalf("removing intentions: %v", err)
	}

	return batch
}

func all(t *testing.T, db *gorm.DB) []intentions.Intention {
	t.Helper()

	var list []intentions.Intention
	err := db.Order("mass_at, id").Find(&list).Error
	if err != nil {
		t.Fatalf("loading intentions: %v", err)
	}

	return list
}

func TestMoveAndRestoreKeepsTheIntention(t *testing.T) {
	db := newTestDB(t)
	kept := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: "inna"})
	deleted := create(t, db, intentions.Intention{
		MassAt: at(t, "2026-12-01 18:00"), Content: "+ Jan Kowalski", IsDeceased: true, IsHidden: true,
		Amount: 10000, AcceptedOn: "2026-11-10",
	})

	batch := remove(t, db, []intentions.Intention{deleted}, time.Time{}, testNow)
	if batch == 0 {
		t.Fatal("Move returned batch 0")
	}
	if got := all(t, db); len(got) != 1 || got[0].ID != kept.ID {
		t.Fatalf("intentions after delete = %+v, want only %d", got, kept.ID)
	}

	b, err := Load(db, batch)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(b.Intentions) != 1 || b.Intentions[0].Content != "+ Jan Kowalski" || b.Week != "" || !b.TrashedAt.Equal(testNow) {
		t.Errorf("batch = %+v", b)
	}

	restored, err := Restore(db, batch)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 1 || restored[0].ID != deleted.ID {
		t.Fatalf("restored = %+v, want intention %d", restored, deleted.ID)
	}

	var got intentions.Intention
	db.First(&got, deleted.ID)
	if got.Content != deleted.Content || !got.IsDeceased || !got.IsHidden || got.Amount != 10000 || got.AcceptedOn != "2026-11-10" || !got.MassAt.Equal(deleted.MassAt) {
		t.Errorf("restored intention = %+v, want %+v", got, deleted)
	}

	_, err = Load(db, batch)
	if !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("batch still in the trash after restoring, err = %v", err)
	}
	_, err = Restore(db, batch)
	if !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("second restore err = %v, want ErrBatchNotFound", err)
	}
}

func TestRestoreUsesNewIDWhenTheOldOneIsTaken(t *testing.T) {
	db := newTestDB(t)
	deleted := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: "pierwsza"})
	batch := remove(t, db, []intentions.Intention{deleted}, time.Time{}, testNow)
	reused := create(t, db, intentions.Intention{ID: deleted.ID, MassAt: at(t, "2026-12-02 18:00"), Content: "nowa"})

	restored, err := Restore(db, batch)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if restored[0].ID == reused.ID {
		t.Fatalf("restored intention overwrote %d", reused.ID)
	}
	if got := all(t, db); len(got) != 2 || got[0].Content != "pierwsza" || got[1].Content != "nowa" {
		t.Errorf("intentions = %+v, want both", got)
	}
}

func TestRestoreClearedWeek(t *testing.T) {
	db := newTestDB(t)
	week := at(t, "2026-11-30 00:00")
	err := masses.MarkLoaded(db, week, 4)
	if err != nil {
		t.Fatalf("marking week: %v", err)
	}
	list := []intentions.Intention{
		create(t, db, intentions.Intention{MassAt: at(t, "2026-11-30 07:00"), Content: "A"}),
		create(t, db, intentions.Intention{MassAt: at(t, "2026-11-30 18:00"), IsPlaceholder: true}),
		create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), IsPlaceholder: true}),
		create(t, db, intentions.Intention{MassAt: at(t, "2026-12-02 18:00"), Content: "poza", IsOutsideParish: true}),
	}

	batch := remove(t, db, list, week, testNow)
	if id, _ := masses.LoadedSchedule(db, week); id != 0 {
		t.Fatalf("week still marked as loaded with schedule %d", id)
	}

	b, err := Load(db, batch)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if b.Week != "2026-11-30" || len(b.Intentions) != 2 || b.Empty != 2 {
		t.Errorf("batch = %+v, want week 2026-11-30, 2 intentions, 2 empty", b)
	}

	create(t, db, intentions.Intention{MassAt: at(t, "2026-11-30 18:00"), Content: "dopisana po wyczyszczeniu"})

	restored, err := Restore(db, batch)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(restored) != 3 {
		t.Errorf("restored %d rows, want 3 (the placeholder at a taken Mass is skipped)", len(restored))
	}

	var got []string
	for _, intention := range all(t, db) {
		label := intention.MassAt.Format("02 15:04") + " " + intention.Content
		if intention.IsPlaceholder {
			label += "(puste)"
		}
		got = append(got, label)
	}
	want := []string{"30 07:00 A", "30 18:00 dopisana po wyczyszczeniu", "01 18:00 (puste)", "02 18:00 poza"}
	if !slices.Equal(got, want) {
		t.Errorf("intentions = %q, want %q", got, want)
	}
	if id, _ := masses.LoadedSchedule(db, week); id != 4 {
		t.Errorf("loaded schedule after restore = %d, want 4", id)
	}
}

func seedSeries(t *testing.T, db *gorm.DB, n int, amount intentions.Grosze) (gregorian.Series, []intentions.Intention) {
	t.Helper()

	series, err := gregorian.Create(db, "+ Piotr", false)
	if err != nil {
		t.Fatalf("creating series: %v", err)
	}
	first := at(t, "2026-12-01 07:00")
	var list []intentions.Intention
	for i := range n {
		intention := intentions.Intention{MassAt: first.AddDate(0, 0, i), Content: "+ Piotr", IsGregorian: true, IsDeceased: true, SeriesID: series.ID}
		if i == 0 {
			intention.Amount = amount
		}
		list = append(list, create(t, db, intention))
	}

	return series, list
}

func seriesAmounts(t *testing.T, db *gorm.DB, id int) []intentions.Grosze {
	t.Helper()

	var amounts []intentions.Grosze
	for _, intention := range all(t, db) {
		if intention.SeriesID == id {
			amounts = append(amounts, intention.Amount)
		}
	}

	return amounts
}

func TestRestoreFirstMassOfSeries(t *testing.T) {
	db := newTestDB(t)
	series, list := seedSeries(t, db, 3, 300000)

	batch := remove(t, db, list[:1], time.Time{}, testNow)
	if got, want := seriesAmounts(t, db, series.ID), []intentions.Grosze{300000, 0}; !slices.Equal(got, want) {
		t.Fatalf("amounts after delete = %v, want %v", got, want)
	}

	_, err := Restore(db, batch)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, want := seriesAmounts(t, db, series.ID), []intentions.Grosze{300000, 0, 0}; !slices.Equal(got, want) {
		t.Errorf("amounts after restore = %v, want %v", got, want)
	}
}

func TestRestoreWholeSeriesRecreatesIt(t *testing.T) {
	db := newTestDB(t)
	series, list := seedSeries(t, db, 2, 300000)

	batch := remove(t, db, list, time.Time{}, testNow)
	if _, err := gregorian.Find(db, series.ID); !errors.Is(err, gregorian.ErrSeriesNotFound) {
		t.Fatalf("series still exists after deleting all its masses, err = %v", err)
	}

	_, err := Restore(db, batch)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	found, err := gregorian.Find(db, series.ID)
	if err != nil || found.Content != "+ Piotr" {
		t.Fatalf("series after restore = %+v, %v", found, err)
	}
	if got, want := seriesAmounts(t, db, series.ID), []intentions.Grosze{300000, 0}; !slices.Equal(got, want) {
		t.Errorf("amounts = %v, want %v", got, want)
	}
}

func TestRestoreSeriesInAnyOrderKeepsTheOffering(t *testing.T) {
	for _, order := range [][2]int{{0, 1}, {1, 0}} {
		db := newTestDB(t)
		series, list := seedSeries(t, db, 3, 300000)
		first := remove(t, db, list[:1], time.Time{}, testNow)
		rest := remove(t, db, list[1:], time.Time{}, testNow)
		batches := [2]int64{first, rest}

		for _, i := range order {
			_, err := Restore(db, batches[i])
			if err != nil {
				t.Fatalf("order %v: Restore: %v", order, err)
			}
		}

		if got, want := seriesAmounts(t, db, series.ID), []intentions.Grosze{300000, 0, 0}; !slices.Equal(got, want) {
			t.Errorf("order %v: amounts = %v, want %v", order, got, want)
		}
	}
}

func TestListAndPurge(t *testing.T) {
	db := newTestDB(t)
	old := create(t, db, intentions.Intention{MassAt: at(t, "2026-10-01 18:00"), Content: "stara"})
	recent := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: "świeża"})
	oldBatch := remove(t, db, []intentions.Intention{old}, time.Time{}, testNow.AddDate(0, 0, -31))
	newBatch := remove(t, db, []intentions.Intention{recent}, time.Time{}, testNow.Add(-time.Hour))

	batches, err := List(db, testNow)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(batches) != 1 || batches[0].ID != newBatch {
		t.Fatalf("batches = %+v, want only %d", batches, newBatch)
	}
	if _, err := Load(db, oldBatch); !errors.Is(err, ErrBatchNotFound) {
		t.Errorf("batch older than %d days still in the trash, err = %v", RetentionDays, err)
	}
}

func TestListNewestFirst(t *testing.T) {
	db := newTestDB(t)
	var ids []int64
	for i, content := range []string{"pierwsza", "druga", "trzecia"} {
		intention := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: content})
		ids = append(ids, remove(t, db, []intentions.Intention{intention}, time.Time{}, testNow.Add(time.Duration(i)*time.Minute)))
	}

	batches, err := List(db, testNow.Add(time.Hour))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var got []string
	var gotIDs []int64
	for _, b := range batches {
		got = append(got, b.Intentions[0].Content)
		gotIDs = append(gotIDs, b.ID)
	}
	if want := []string{"trzecia", "druga", "pierwsza"}; !slices.Equal(got, want) {
		t.Errorf("order = %q, want %q", got, want)
	}
	slices.Reverse(ids)
	if !slices.Equal(gotIDs, ids) {
		t.Errorf("batch ids = %v, want %v", gotIDs, ids)
	}
}

func TestBatchIDsAreUniqueAtTheSameTime(t *testing.T) {
	db := newTestDB(t)
	a := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: "a"})
	b := create(t, db, intentions.Intention{MassAt: at(t, "2026-12-01 18:00"), Content: "b"})

	first := remove(t, db, []intentions.Intention{a}, time.Time{}, testNow)
	second := remove(t, db, []intentions.Intention{b}, time.Time{}, testNow)

	if first == second {
		t.Errorf("two deletions share batch %d", first)
	}
}

func TestMoveNothing(t *testing.T) {
	db := newTestDB(t)

	batch, err := Move(db, nil, time.Time{}, testNow)
	if err != nil || batch != 0 {
		t.Errorf("Move(nil) = %d, %v, want 0, nil", batch, err)
	}
}
