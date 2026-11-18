package backup

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
	"go-parafia/internal/safefile"
)

const (
	FileName           = "intencje-kopia.db"
	DefaultDailyCopies = 7
	MaxDailyCopies     = 90
	dailyPrefix        = "intencje-kopia-"
	dailyLayout        = "2006-01-02"
	autoDelay          = time.Minute
)

var (
	ErrNoDir     = errors.New("nie wybrano folderu kopii")
	ErrNotFolder = errors.New("nie ma takiego folderu")
)

type State struct {
	Dir         string    `json:"dir"`
	LastBackup  time.Time `json:"last_backup"`
	LastChange  time.Time `json:"last_change"`
	DailyCopies int       `json:"daily_copies"`
}

func (s State) Pending() bool {
	return s.Dir != "" && (s.LastBackup.IsZero() || s.LastChange.After(s.LastBackup))
}

type Backups struct {
	mu        sync.Mutex
	db        *gorm.DB
	path      string
	state     State
	now       func() time.Time
	autoDelay time.Duration
	timer     *time.Timer
}

func Load(db *gorm.DB, path string) (*Backups, error) {
	b := &Backups{db: db, path: path, now: time.Now, autoDelay: autoDelay, state: State{DailyCopies: DefaultDailyCopies}}
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
		log.Printf("error reading %s, starting without backup settings: %v", path, err)
		b.state = State{DailyCopies: DefaultDailyCopies}
		renameErr := os.Rename(path, path+".bad")
		if renameErr != nil {
			log.Printf("error moving %s aside: %v", path, renameErr)
		}
	}

	b.state.DailyCopies = min(max(b.state.DailyCopies, 0), MaxDailyCopies)

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

func (b *Backups) SetDailyCopies(n int) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.DailyCopies = min(max(n, 0), MaxDailyCopies)
	err := b.save()
	if err != nil {
		return err
	}
	if b.state.Dir != "" && b.state.DailyCopies > 0 {
		err = pruneDaily(b.state.Dir, b.state.DailyCopies)
		if err != nil {
			log.Printf("error removing old daily backup copies: %v", err)
		}
	}

	return nil
}

func (b *Backups) MarkChanged() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.state.LastChange = b.now()
	if b.state.Dir != "" && b.autoDelay > 0 && b.timer == nil {
		b.timer = time.AfterFunc(b.autoDelay, b.autoRun)
	}
	return b.save()
}

func (b *Backups) autoRun() {
	b.mu.Lock()
	b.timer = nil
	b.mu.Unlock()

	err := b.RunIfPending()
	if err != nil {
		log.Printf("error making automatic backup: %v", err)
	}
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

	now := b.now()
	if b.state.DailyCopies > 0 {
		err = writeDaily(b.state.Dir, path, now, b.state.DailyCopies)
		if err != nil {
			log.Printf("error making the daily backup copy: %v", err)
		}
	}

	b.state.LastBackup = now
	return path, b.save()
}

func DailyName(day time.Time) string {
	return dailyPrefix + day.In(intentions.TimeZone).Format(dailyLayout) + ".db"
}

func writeDaily(dir, current string, now time.Time, keep int) error {
	data, err := os.ReadFile(current)
	if err != nil {
		return err
	}
	err = safefile.Write(filepath.Join(dir, DailyName(now)), data, 0o644)
	if err != nil {
		return err
	}

	return pruneDaily(dir, keep)
}

func pruneDaily(dir string, keep int) error {
	days, err := DailyCopies(dir)
	if err != nil {
		return err
	}
	var errs []error
	for _, day := range days[min(keep, len(days)):] {
		errs = append(errs, os.Remove(filepath.Join(dir, DailyName(day))))
	}

	return errors.Join(errs...)
}

// DailyCopies lists the days of the daily copies in dir, newest first.
func DailyCopies(dir string) ([]time.Time, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	var days []time.Time
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, dailyPrefix) || !strings.HasSuffix(name, ".db") {
			continue
		}
		day, err := time.ParseInLocation(dailyLayout, strings.TrimSuffix(strings.TrimPrefix(name, dailyPrefix), ".db"), intentions.TimeZone)
		if err != nil {
			continue
		}
		days = append(days, day)
	}
	slices.SortFunc(days, func(a, b time.Time) int { return b.Compare(a) })

	return days, nil
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

	return safefile.Write(b.path, data, 0o644)
}
