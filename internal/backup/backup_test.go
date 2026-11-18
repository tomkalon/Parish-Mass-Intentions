package backup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
)

func newTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := database.Open(filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.Close()
		}
		if err != nil {
			t.Errorf("closing database: %v", err)
		}
	})

	return db
}

func addIntention(t *testing.T, db *gorm.DB, content string) {
	t.Helper()

	intention := intentions.Intention{MassAt: time.Date(2026, 11, 2, 18, 0, 0, 0, intentions.TimeZone), Content: content}
	err := intentions.Save(db, &intention)
	if err != nil {
		t.Fatal(err)
	}
}

type clock struct{ t time.Time }

func (c *clock) now() time.Time {
	c.t = c.t.Add(time.Minute)
	return c.t
}

func loadWithClock(t *testing.T, db *gorm.DB, path string) *Backups {
	t.Helper()

	b, err := Load(db, path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	c := &clock{t: time.Date(2026, 11, 2, 22, 0, 0, 0, intentions.TimeZone)}
	b.now = c.now
	b.autoDelay = 0

	return b
}

func TestLoadWithoutFile(t *testing.T) {
	b, err := Load(nil, filepath.Join(t.TempDir(), "kopia.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := b.State(); got != (State{DailyCopies: DefaultDailyCopies}) {
		t.Errorf("state = %+v, want empty with the default daily copies", got)
	}
}

func TestLoadBrokenFileDoesNotBlockStart(t *testing.T) {
	for name, content := range map[string][]byte{
		"invalid json":       []byte("{nie json"),
		"zeros after outage": make([]byte, 128),
		"empty":              nil,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "kopia.json")
			err := os.WriteFile(path, content, 0o644)
			if err != nil {
				t.Fatal(err)
			}

			b, err := Load(nil, path)
			if err != nil {
				t.Fatalf("Load: %v", err)
			}
			if got := b.State(); got != (State{DailyCopies: DefaultDailyCopies}) {
				t.Errorf("state = %+v, want empty with the default daily copies", got)
			}

			_, err = os.Stat(path + ".bad")
			if err != nil {
				t.Errorf("broken file not kept as .bad: %v", err)
			}

			err = b.SetDir(dir)
			if err != nil {
				t.Fatalf("SetDir after a broken file: %v", err)
			}
			reloaded, err := Load(nil, path)
			if err != nil {
				t.Fatal(err)
			}
			if reloaded.State().Dir != dir {
				t.Errorf("Dir after reload = %q, want %q", reloaded.State().Dir, dir)
			}
		})
	}
}

func TestSetDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kopia.json")
	b := loadWithClock(t, nil, path)
	folder := t.TempDir()
	file := filepath.Join(folder, "plik.txt")
	err := os.WriteFile(file, nil, 0o644)
	if err != nil {
		t.Fatal(err)
	}

	for _, bad := range []string{filepath.Join(folder, "brak"), file} {
		if err := b.SetDir(bad); !errors.Is(err, ErrNotFolder) {
			t.Errorf("SetDir(%q) error = %v, want ErrNotFolder", bad, err)
		}
	}

	err = b.SetDir(folder)
	if err != nil {
		t.Fatalf("SetDir: %v", err)
	}
	reloaded, err := Load(nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reloaded.State().Dir; got != folder {
		t.Errorf("reloaded dir = %q, want %q", got, folder)
	}

	err = b.SetDir("")
	if err != nil {
		t.Fatalf("SetDir(\"\"): %v", err)
	}
	if got := b.State().Dir; got != "" {
		t.Errorf("dir = %q after turning off, want empty", got)
	}
}

func TestStatePending(t *testing.T) {
	early := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	late := early.Add(time.Hour)

	tests := []struct {
		name  string
		state State
		want  bool
	}{
		{name: "no folder", state: State{LastChange: late}, want: false},
		{name: "folder, never backed up", state: State{Dir: "/kopie"}, want: true},
		{name: "changed after backup", state: State{Dir: "/kopie", LastBackup: early, LastChange: late}, want: true},
		{name: "backed up after change", state: State{Dir: "/kopie", LastBackup: late, LastChange: early}, want: false},
		{name: "backed up, no changes", state: State{Dir: "/kopie", LastBackup: early}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.state.Pending(); got != tt.want {
				t.Errorf("Pending = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRun(t *testing.T) {
	db := newTestDB(t)
	path := filepath.Join(t.TempDir(), "kopia.json")
	b := loadWithClock(t, db, path)

	if _, err := b.Run(); !errors.Is(err, ErrNoDir) {
		t.Fatalf("Run without a folder: error = %v, want ErrNoDir", err)
	}

	folder := t.TempDir()
	err := b.SetDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	addIntention(t, db, "pierwsza")

	file, err := b.Run()
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if want := filepath.Join(folder, FileName); file != want {
		t.Errorf("backup file = %q, want %q", file, want)
	}

	addIntention(t, db, "druga")
	_, err = b.Run()
	if err != nil {
		t.Fatalf("second Run: %v", err)
	}

	summary, err := database.PrepareImport(file)
	if err != nil || summary.Intentions != 2 {
		t.Errorf("backup file: %+v, %v; want both intentions (the file is overwritten)", summary, err)
	}
	if got := fileNames(t, folder); !slices.Equal(got, []string{"intencje-kopia-2026-11-02.db", FileName}) {
		t.Errorf("files in the backup folder = %q, want the current copy and the copy of the day", got)
	}

	reloaded, err := Load(db, path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.State().LastBackup.IsZero() {
		t.Error("LastBackup not saved to the config file")
	}
}

func TestRunIfPending(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, "")
	folder := t.TempDir()
	file := filepath.Join(folder, FileName)

	err := b.RunIfPending()
	if err != nil {
		t.Fatalf("RunIfPending without a folder: %v", err)
	}

	err = b.SetDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	err = b.RunIfPending()
	if err != nil {
		t.Fatalf("first RunIfPending: %v", err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("no backup after choosing a folder: %v", err)
	}

	err = os.Remove(file)
	if err != nil {
		t.Fatal(err)
	}
	err = b.RunIfPending()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
		t.Error("backup made although nothing changed")
	}

	err = b.MarkChanged()
	if err != nil {
		t.Fatal(err)
	}
	err = b.RunIfPending()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("no backup after a change: %v", err)
	}
	if b.Pending() {
		t.Error("still pending after the backup")
	}
}

func TestRunIfPendingFolderGone(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, "")
	folder := filepath.Join(t.TempDir(), "chmura")
	err := os.Mkdir(folder, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = b.SetDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Remove(folder)
	if err != nil {
		t.Fatal(err)
	}

	if err := b.RunIfPending(); err == nil {
		t.Error("RunIfPending succeeded although the folder is gone")
	}
	if !b.Pending() {
		t.Error("not pending after a failed backup")
	}
}

func TestTrackChanges(t *testing.T) {
	tests := []struct {
		method string
		status int
		want   bool
	}{
		{method: http.MethodGet, status: http.StatusOK, want: false},
		{method: http.MethodPost, status: http.StatusSeeOther, want: true},
		{method: http.MethodPost, status: 0, want: true},
		{method: http.MethodDelete, status: http.StatusNoContent, want: true},
		{method: http.MethodPost, status: http.StatusUnprocessableEntity, want: false},
		{method: http.MethodPost, status: http.StatusInternalServerError, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.method+" "+http.StatusText(tt.status), func(t *testing.T) {
			b := loadWithClock(t, nil, "")
			handler := b.TrackChanges(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.status != 0 {
					w.WriteHeader(tt.status)
				}
			}))

			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(tt.method, "/", nil))

			if got := !b.State().LastChange.IsZero(); got != tt.want {
				t.Errorf("marked as changed = %v, want %v", got, tt.want)
			}
		})
	}
}

func fileNames(t *testing.T, dir string) []string {
	t.Helper()

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	slices.Sort(names)

	return names
}

func backupCount(t *testing.T, path string) int64 {
	t.Helper()

	summary, err := database.PrepareImport(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return summary.Intentions
}

func TestDailyCopiesKeepTheNewestDays(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	folder := t.TempDir()
	if err := b.SetDir(folder); err != nil {
		t.Fatal(err)
	}
	if err := b.SetDailyCopies(3); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"intencje-kopia-notatki.db", "intencje-kopia-2026-13-45.db", "inne.txt"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	day := time.Date(2026, 11, 2, 9, 0, 0, 0, intentions.TimeZone)
	for i := range 5 {
		b.now = func() time.Time { return day.AddDate(0, 0, i) }
		addIntention(t, db, "intencja")
		if _, err := b.Run(); err != nil {
			t.Fatalf("Run on day %d: %v", i, err)
		}
	}

	want := []string{
		"inne.txt",
		"intencje-kopia-2026-11-04.db",
		"intencje-kopia-2026-11-05.db",
		"intencje-kopia-2026-11-06.db",
		"intencje-kopia-2026-13-45.db",
		"intencje-kopia-notatki.db",
		FileName,
	}
	if got := fileNames(t, folder); !slices.Equal(got, want) {
		t.Errorf("files = %q, want %q", got, want)
	}
	for i, name := range []string{"intencje-kopia-2026-11-04.db", "intencje-kopia-2026-11-05.db", "intencje-kopia-2026-11-06.db", FileName} {
		if got, want := backupCount(t, filepath.Join(folder, name)), int64(min(3+i, 5)); got != want {
			t.Errorf("%s has %d intentions, want %d", name, got, want)
		}
	}

	days, err := DailyCopies(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(days) != 3 || days[0].Format("2006-01-02") != "2026-11-06" || days[2].Format("2006-01-02") != "2026-11-04" {
		t.Errorf("DailyCopies = %v, want 6, 5 and 4 November", days)
	}
}

func TestDailyCopyKeepsTheLastStateOfTheDay(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	folder := t.TempDir()
	if err := b.SetDir(folder); err != nil {
		t.Fatal(err)
	}

	for range 3 {
		addIntention(t, db, "intencja")
		if _, err := b.Run(); err != nil {
			t.Fatal(err)
		}
	}

	if got := fileNames(t, folder); !slices.Equal(got, []string{"intencje-kopia-2026-11-02.db", FileName}) {
		t.Errorf("files = %q, want one copy of the day", got)
	}
	if got := backupCount(t, filepath.Join(folder, "intencje-kopia-2026-11-02.db")); got != 3 {
		t.Errorf("copy of the day has %d intentions, want the last state (3)", got)
	}
}

func TestNoDailyCopiesWhenSwitchedOff(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	folder := t.TempDir()
	if err := b.SetDir(folder); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(folder, "intencje-kopia-2026-10-01.db")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := b.SetDailyCopies(0); err != nil {
		t.Fatal(err)
	}

	addIntention(t, db, "intencja")
	if _, err := b.Run(); err != nil {
		t.Fatal(err)
	}

	if got := fileNames(t, folder); !slices.Equal(got, []string{"intencje-kopia-2026-10-01.db", FileName}) {
		t.Errorf("files = %q, want no new daily copy and the old one kept", got)
	}
}

func TestSetDailyCopiesIsSavedAndLimited(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kopia.json")
	b, err := Load(nil, path)
	if err != nil {
		t.Fatal(err)
	}

	for n, want := range map[int]int{5: 5, 0: 0, -3: 0, 500: MaxDailyCopies} {
		if err := b.SetDailyCopies(n); err != nil {
			t.Fatal(err)
		}
		reloaded, err := Load(nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if got := reloaded.State().DailyCopies; got != want {
			t.Errorf("SetDailyCopies(%d) then Load = %d, want %d", n, got, want)
		}
	}

	if err := os.WriteFile(path, []byte(`{"dir": "", "daily_copies": 400}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := Load(nil, path); err != nil || b.State().DailyCopies != MaxDailyCopies {
		t.Errorf("Load of a too large value = %+v, %v", b.State(), err)
	}
	if err := os.WriteFile(path, []byte(`{"dir": ""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := Load(nil, path); err != nil || b.State().DailyCopies != DefaultDailyCopies {
		t.Errorf("Load of settings from before daily copies = %+v, %v, want the default", b.State(), err)
	}
}

func TestChangesAreBackedUpAutomatically(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	b.now = time.Now
	b.autoDelay = 50 * time.Millisecond
	folder := t.TempDir()
	if err := b.SetDir(folder); err != nil {
		t.Fatal(err)
	}

	addIntention(t, db, "pierwsza")
	for range 3 {
		if err := b.MarkChanged(); err != nil {
			t.Fatal(err)
		}
	}

	deadline := time.Now().Add(5 * time.Second)
	for b.Pending() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if b.Pending() {
		t.Fatal("changes were not backed up automatically")
	}
	if got := backupCount(t, filepath.Join(folder, FileName)); got != 1 {
		t.Errorf("automatic backup has %d intentions, want 1", got)
	}

	addIntention(t, db, "druga")
	if err := b.MarkChanged(); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(5 * time.Second)
	for b.Pending() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := backupCount(t, filepath.Join(folder, FileName)); got != 2 {
		t.Errorf("second automatic backup has %d intentions, want 2", got)
	}
}

func TestNoAutomaticBackupWithoutFolder(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	b.autoDelay = 10 * time.Millisecond

	if err := b.MarkChanged(); err != nil {
		t.Fatal(err)
	}

	b.mu.Lock()
	timer := b.timer
	b.mu.Unlock()
	if timer != nil {
		t.Error("an automatic backup was planned without a backup folder")
	}
}

func TestLoweringDailyCopiesRemovesTheOldest(t *testing.T) {
	db := newTestDB(t)
	b := loadWithClock(t, db, filepath.Join(t.TempDir(), "kopia.json"))
	folder := t.TempDir()
	if err := b.SetDir(folder); err != nil {
		t.Fatal(err)
	}
	for _, day := range []string{"2026-11-01", "2026-11-02", "2026-11-03", "2026-11-04"} {
		if err := os.WriteFile(filepath.Join(folder, "intencje-kopia-"+day+".db"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := b.SetDailyCopies(2); err != nil {
		t.Fatal(err)
	}

	if got := fileNames(t, folder); !slices.Equal(got, []string{"intencje-kopia-2026-11-03.db", "intencje-kopia-2026-11-04.db"}) {
		t.Errorf("files = %q, want the two newest daily copies", got)
	}
}
