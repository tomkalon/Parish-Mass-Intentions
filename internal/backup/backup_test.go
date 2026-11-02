package backup

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
			sqlDB.Close()
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

	return b
}

func TestLoadWithoutFile(t *testing.T) {
	b, err := Load(nil, filepath.Join(t.TempDir(), "kopia.json"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := b.State(); got != (State{}) {
		t.Errorf("state = %+v, want empty", got)
	}
}

func TestLoadBrokenFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kopia.json")
	err := os.WriteFile(path, []byte("{nie json"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, err = Load(nil, path)
	if err == nil {
		t.Error("Load accepted a broken file")
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
	entries, _ := os.ReadDir(folder)
	if len(entries) != 1 {
		t.Errorf("files in the backup folder = %d, want 1", len(entries))
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
