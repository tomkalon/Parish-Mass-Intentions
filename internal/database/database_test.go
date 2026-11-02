package database

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/calendar"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

func openTestDB(t *testing.T, path string) *gorm.DB {
	t.Helper()

	db, err := Open(path)
	if err != nil {
		t.Fatalf("opening %s: %v", path, err)
	}
	t.Cleanup(func() { closeDB(db) })

	return db
}

type fixture struct {
	parish    string
	feastDate string
	feast     string
	schedule  string
	contents  []string
}

func seed(t *testing.T, db *gorm.DB, f fixture) {
	t.Helper()

	for i, content := range f.contents {
		at := time.Date(2026, 11, 2+i, 18, 0, 0, 0, intentions.TimeZone)
		intention := intentions.Intention{MassAt: at, Content: content}
		err := intentions.Save(db, &intention)
		if err != nil {
			t.Fatalf("saving intention: %v", err)
		}
	}

	err := calendar.SaveFeast(db, f.feastDate, f.feast)
	if err != nil {
		t.Fatalf("saving feast: %v", err)
	}
	err = settings.SaveParish(db, settings.Setting{ParishName: f.parish})
	if err != nil {
		t.Fatalf("saving settings: %v", err)
	}
	_, err = masses.Save(db, masses.Schedule{Name: f.schedule, Masses: [7][]string{time.Sunday: {"10:00"}}})
	if err != nil {
		t.Fatalf("saving schedule: %v", err)
	}
}

type snapshot struct {
	contents  []string
	feasts    map[string]string
	parish    string
	schedules []string
}

func read(t *testing.T, db *gorm.DB) snapshot {
	t.Helper()

	var list []intentions.Intention
	err := db.Order("mass_at, id").Find(&list).Error
	if err != nil {
		t.Fatalf("loading intentions: %v", err)
	}
	var contents []string
	for _, intention := range list {
		contents = append(contents, intention.Content)
	}

	var days []calendar.Day
	err = db.Find(&days).Error
	if err != nil {
		t.Fatalf("loading feasts: %v", err)
	}
	feasts := make(map[string]string)
	for _, day := range days {
		feasts[day.Date] = day.Feast
	}

	setting, err := settings.Load(db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}

	schedules, err := masses.Load(db)
	if err != nil {
		t.Fatalf("loading schedules: %v", err)
	}
	var names []string
	for _, s := range schedules {
		names = append(names, s.Name)
	}

	return snapshot{contents: contents, feasts: feasts, parish: setting.ParishName, schedules: names}
}

func TestExport(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "data.db"))
	seed(t, db, fixture{parish: "Parafia A", feastDate: "2026-11-04", feast: "wspomnienie", schedule: "Zimowy", contents: []string{"+ Jan Kowalski"}})

	target := filepath.Join(dir, "kopia.db")
	err := os.WriteFile(target, []byte("stary plik"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = Export(db, target)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	got := read(t, openTestDB(t, target))
	want := read(t, db)
	if !slices.Equal(got.contents, want.contents) || got.parish != want.parish || got.feasts["2026-11-04"] != "wspomnienie" || !slices.Contains(got.schedules, "Zimowy") {
		t.Errorf("exported = %+v, want %+v", got, want)
	}
	if _, err := os.Stat(target + ".tmp"); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

func TestPrepareImportRejectsInvalidFiles(t *testing.T) {
	dir := t.TempDir()

	text := filepath.Join(dir, "notatki.txt")
	err := os.WriteFile(text, []byte("to nie jest baza danych, tylko zwykły tekst o długości ponad stu bajtów, żeby SQLite przeczytał nagłówek"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(dir, "pusty.db")
	err = os.WriteFile(empty, nil, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	other := filepath.Join(dir, "inna.db")
	otherDB, err := gorm.Open(sqlite.Open(other), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	err = otherDB.Exec("CREATE TABLE products (id integer PRIMARY KEY, name text)").Error
	closeDB(otherDB)
	if err != nil {
		t.Fatal(err)
	}

	for _, path := range []string{text, empty, other, filepath.Join(dir, "brak.db")} {
		t.Run(filepath.Base(path), func(t *testing.T) {
			_, err := PrepareImport(path)
			if !errors.Is(err, ErrInvalidFile) {
				t.Errorf("PrepareImport error = %v, want ErrInvalidFile", err)
			}
		})
	}
}

type oldIntention struct {
	ID              int
	MassAt          time.Time
	Content         string
	IsOutsideParish bool
}

func (oldIntention) TableName() string {
	return "intentions"
}

func TestPrepareImportMigratesOldSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stara.db")
	old, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	err = old.AutoMigrate(&oldIntention{})
	if err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 10, 18, 7, 0, 0, 0, intentions.TimeZone)
	last := time.Date(2026, 10, 25, 18, 0, 0, 0, intentions.TimeZone)
	err = old.Create(&[]oldIntention{
		{MassAt: last, Content: "+ Anna Stolarz"},
		{MassAt: first, Content: "+ Helena Cichoń"},
	}).Error
	closeDB(old)
	if err != nil {
		t.Fatal(err)
	}

	summary, err := PrepareImport(path)
	if err != nil {
		t.Fatalf("PrepareImport: %v", err)
	}

	if summary.Intentions != 2 || !summary.First.Equal(first) || !summary.Last.Equal(last) {
		t.Errorf("summary = %+v, want 2 intentions from %v to %v", summary, first, last)
	}
	if summary.ParishName != settings.DefaultParishName {
		t.Errorf("parish = %q, want the default %q (no settings in the file)", summary.ParishName, settings.DefaultParishName)
	}

	migrated := openTestDB(t, path)
	if !migrated.Migrator().HasColumn(&intentions.Intention{}, "IsHidden") {
		t.Error("intentions.is_hidden missing after PrepareImport")
	}
}

func TestRestore(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "data.db"))
	seed(t, db, fixture{parish: "Parafia stara", feastDate: "2026-11-04", feast: "stare święto", schedule: "Stary porządek", contents: []string{"stara 1", "stara 2", "stara 3"}})
	before := read(t, db)

	importPath := filepath.Join(dir, "import.db")
	source := openTestDB(t, importPath)
	seed(t, source, fixture{parish: "Parafia nowa", feastDate: "2026-12-25", feast: "Boże Narodzenie", schedule: "Nowy porządek", contents: []string{"nowa 1"}})
	want := read(t, source)
	closeDB(source)

	backup := filepath.Join(dir, "data-2026-11-02-221700.db.bak")
	err := Restore(db, importPath, backup)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}

	got := read(t, db)
	if !slices.Equal(got.contents, want.contents) {
		t.Errorf("intentions = %q, want %q", got.contents, want.contents)
	}
	if len(got.feasts) != 1 || got.feasts["2026-12-25"] != "Boże Narodzenie" {
		t.Errorf("feasts = %v, want only Boże Narodzenie", got.feasts)
	}
	if got.parish != "Parafia nowa" {
		t.Errorf("parish = %q, want %q", got.parish, "Parafia nowa")
	}
	if !slices.Equal(got.schedules, want.schedules) {
		t.Errorf("schedules = %q, want %q", got.schedules, want.schedules)
	}

	saved := read(t, openTestDB(t, backup))
	if !slices.Equal(saved.contents, before.contents) || saved.parish != "Parafia stara" || saved.feasts["2026-11-04"] != "stare święto" {
		t.Errorf("backup = %+v, want the data from before the import %+v", saved, before)
	}

	intention := intentions.Intention{MassAt: time.Date(2026, 11, 9, 18, 0, 0, 0, intentions.TimeZone), Content: "po imporcie"}
	err = intentions.Save(db, &intention)
	if err != nil {
		t.Fatalf("saving after restore: %v", err)
	}

	err = Restore(db, backup, filepath.Join(dir, "drugi.db.bak"))
	if err != nil {
		t.Fatalf("second Restore: %v", err)
	}
	if got := read(t, db); !slices.Equal(got.contents, before.contents) {
		t.Errorf("after restoring the backup: intentions = %q, want %q", got.contents, before.contents)
	}
}

func TestRestoreInvalidFileChangesNothing(t *testing.T) {
	dir := t.TempDir()
	db := openTestDB(t, filepath.Join(dir, "data.db"))
	seed(t, db, fixture{parish: "Parafia", feastDate: "2026-11-04", feast: "święto", schedule: "Porządek", contents: []string{"+ Jan Kowalski"}})
	before := read(t, db)

	bad := filepath.Join(dir, "zly.db")
	err := os.WriteFile(bad, []byte("nie baza danych, tylko tekst, który na pewno nie zaczyna się od nagłówka SQLite format 3 i ma sto bajtów"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	backup := filepath.Join(dir, "kopia.db.bak")
	err = Restore(db, bad, backup)
	if !errors.Is(err, ErrInvalidFile) {
		t.Fatalf("Restore error = %v, want ErrInvalidFile", err)
	}

	if got := read(t, db); !slices.Equal(got.contents, before.contents) || got.parish != before.parish {
		t.Errorf("data changed: %+v, want %+v", got, before)
	}
	if _, err := os.Stat(backup); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("backup created for a rejected file: %v", err)
	}
}
