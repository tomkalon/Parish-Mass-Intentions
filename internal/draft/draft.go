package draft

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"go-parafia/internal/safefile"
)

const FileName = "szkic-intencji.json"

const finishedLimit = 32

var ErrCorrupt = errors.New("uszkodzony plik szkicu")

type Draft struct {
	Token     string    `json:"token"`
	SavedAt   time.Time `json:"saved_at"`
	EditID    int       `json:"edit_id,omitempty"`
	Week      string    `json:"week"`
	Day       string    `json:"day"`
	Time      string    `json:"time"`
	Content   string    `json:"content"`
	Outside   bool      `json:"outside,omitempty"`
	Deceased  bool      `json:"deceased,omitempty"`
	Gregorian bool      `json:"gregorian,omitempty"`
	Hidden    bool      `json:"hidden,omitempty"`
	Amount    string    `json:"amount,omitempty"`
	SeriesID  int       `json:"series_id,omitempty"`
}

func (d Draft) Empty() bool {
	return strings.TrimSpace(d.Content) == "" && strings.TrimSpace(d.Amount) == ""
}

type Store struct {
	mu       sync.Mutex
	path     string
	finished []string
	now      func() time.Time
}

func NewStore(path string) *Store {
	return &Store{path: path, now: time.Now}
}

func (s *Store) Save(d Draft) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.path == "" || d.Token == "" || slices.Contains(s.finished, d.Token) {
		return nil
	}
	if d.Empty() {
		return s.remove(d.Token)
	}

	if d.SavedAt.IsZero() {
		d.SavedAt = s.now()
	}
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}

	return safefile.Write(s.path, data, 0o644)
}

func (s *Store) Load() (Draft, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.load()
}

func (s *Store) Finish(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if token == "" {
		return nil
	}

	s.finished = append(s.finished, token)
	if len(s.finished) > finishedLimit {
		s.finished = s.finished[len(s.finished)-finishedLimit:]
	}

	return s.remove(token)
}

func (s *Store) Discard() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok, err := s.load()
	if err != nil || !ok {
		return err
	}
	if d.Token != "" {
		s.finished = append(s.finished, d.Token)
	}

	return removeFile(s.path)
}

func (s *Store) DiscardEdit(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	d, ok, err := s.load()
	if err != nil || !ok || d.EditID != id {
		return err
	}
	s.finished = append(s.finished, d.Token)

	return removeFile(s.path)
}

func (s *Store) load() (Draft, bool, error) {
	if s.path == "" {
		return Draft{}, false, nil
	}

	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return Draft{}, false, nil
	}
	if err != nil {
		return Draft{}, false, err
	}

	var d Draft
	err = json.Unmarshal(data, &d)
	if err != nil {
		renameErr := os.Rename(s.path, s.path+".bad")
		return Draft{}, false, errors.Join(fmt.Errorf("%w %s: %v", ErrCorrupt, s.path, err), renameErr)
	}
	if d.Empty() {
		return Draft{}, false, removeFile(s.path)
	}

	return d, true, nil
}

func (s *Store) remove(token string) error {
	d, ok, err := s.load()
	if err != nil || !ok || d.Token != token {
		return err
	}

	return removeFile(s.path)
}

func removeFile(path string) error {
	err := os.Remove(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}
