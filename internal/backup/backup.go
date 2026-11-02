package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"go-parafia/internal/database"
)

const FileName = "intencje-kopia.db"

var (
	ErrNoDir     = errors.New("nie wybrano folderu kopii")
	ErrNotFolder = errors.New("nie ma takiego folderu")
)

type State struct {
	Dir        string    `json:"dir"`
	LastBackup time.Time `json:"last_backup"`
	LastChange time.Time `json:"last_change"`
}

func (s State) Pending() bool {
	return s.Dir != "" && (s.LastBackup.IsZero() || s.LastChange.After(s.LastBackup))
}

type Backups struct {
	mu    sync.Mutex
	db    *gorm.DB
	path  string
	state State
	now   func() time.Time
}

func Load(db *gorm.DB, path string) (*Backups, error) {
	b := &Backups{db: db, path: path, now: time.Now}
	if path == "" {
		return b, nil
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(data, &b.state)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	return b, nil
}

func (b *Backups) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.state
}

func (b *Backups) Pending() bool {
	return b.State().Pending()
}

func (b *Backups) SetDir(dir string) error {
	if dir != "" {
		info, err := os.Stat(dir)
		if err != nil || !info.IsDir() {
			return fmt.Errorf("%w: %s", ErrNotFolder, dir)
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.Dir = dir
	return b.save()
}

func (b *Backups) MarkChanged() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.LastChange = b.now()
	return b.save()
}

func (b *Backups) Run() (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.state.Dir == "" {
		return "", ErrNoDir
	}

	path := filepath.Join(b.state.Dir, FileName)
	err := database.Export(b.db, path)
	if err != nil {
		return "", err
	}

	b.state.LastBackup = b.now()
	return path, b.save()
}

func (b *Backups) RunIfPending() error {
	if !b.Pending() {
		return nil
	}

	_, err := b.Run()
	return err
}

func (b *Backups) TrackChanges(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}

		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)

		if ww.Status() < http.StatusBadRequest {
			err := b.MarkChanged()
			if err != nil {
				log.Printf("error marking data as changed: %v", err)
			}
		}
	})
}

func (b *Backups) save() error {
	if b.path == "" {
		return nil
	}

	data, err := json.MarshalIndent(b.state, "", "  ")
	if err != nil {
		return err
	}

	tmp := b.path + ".tmp"
	err = os.WriteFile(tmp, data, 0o644)
	if err != nil {
		return err
	}

	return os.Rename(tmp, b.path)
}
