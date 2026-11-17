package draft

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var savedAt = time.Date(2026, 11, 17, 14, 32, 0, 0, time.UTC)

func newTestStore(t *testing.T) *Store {
	t.Helper()

	s := NewStore(filepath.Join(t.TempDir(), FileName))
	s.now = func() time.Time { return savedAt }

	return s
}

func sample(token string) Draft {
	return Draft{
		Token:    token,
		EditID:   7,
		Week:     "2026-11-30",
		Day:      "2026-12-01",
		Time:     "18:00",
		Content:  "+ Jan Kowalski",
		Deceased: true,
		Amount:   "100",
		SeriesID: 3,
	}
}

func mustLoad(t *testing.T, s *Store) (Draft, bool) {
	t.Helper()

	d, ok, err := s.Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	return d, ok
}

func TestSaveAndLoad(t *testing.T) {
	s := newTestStore(t)

	_, ok := mustLoad(t, s)
	if ok {
		t.Fatal("new store has a draft")
	}

	err := s.Save(sample("a"))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, ok := mustLoad(t, s)
	if !ok {
		t.Fatal("draft not found after Save")
	}
	want := sample("a")
	want.SavedAt = savedAt
	if !got.SavedAt.Equal(want.SavedAt) {
		t.Errorf("SavedAt = %v, want %v", got.SavedAt, want.SavedAt)
	}
	got.SavedAt = want.SavedAt
	if got != want {
		t.Errorf("Load = %+v, want %+v", got, want)
	}
}

func TestSaveWithoutTokenIsIgnored(t *testing.T) {
	s := newTestStore(t)

	err := s.Save(sample(""))
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	_, ok := mustLoad(t, s)
	if ok {
		t.Error("draft without token was saved")
	}
}

func TestSaveEmptyClearsOnlyOwnDraft(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("old"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Save(Draft{Token: "new", Content: "  "})
	if err != nil {
		t.Fatalf("Save(empty, other token): %v", err)
	}
	got, ok := mustLoad(t, s)
	if !ok || got.Token != "old" {
		t.Fatalf("empty draft of another panel removed the old one: %+v, %v", got, ok)
	}

	err = s.Save(Draft{Token: "old"})
	if err != nil {
		t.Fatalf("Save(empty, same token): %v", err)
	}
	_, ok = mustLoad(t, s)
	if ok {
		t.Error("emptied draft is still stored")
	}
}

func TestAmountAloneIsWorthKeeping(t *testing.T) {
	s := newTestStore(t)

	err := s.Save(Draft{Token: "a", Amount: "50"})
	if err != nil {
		t.Fatal(err)
	}

	_, ok := mustLoad(t, s)
	if !ok {
		t.Error("draft with only an amount was not stored")
	}
}

func TestFinishRemovesMatchingDraftAndBlocksLateSaves(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Finish("a")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}
	_, ok := mustLoad(t, s)
	if ok {
		t.Fatal("draft still stored after Finish")
	}

	err = s.Save(sample("a"))
	if err != nil {
		t.Fatalf("late Save: %v", err)
	}
	_, ok = mustLoad(t, s)
	if ok {
		t.Error("late autosave of a finished panel brought the draft back")
	}
}

func TestFinishKeepsOtherDraft(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("crash"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Finish("other")
	if err != nil {
		t.Fatalf("Finish: %v", err)
	}

	got, ok := mustLoad(t, s)
	if !ok || got.Token != "crash" {
		t.Errorf("Finish of another panel removed the draft: %+v, %v", got, ok)
	}
}

func TestFinishWithoutTokenDoesNothing(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Finish("")
	if err != nil {
		t.Fatal(err)
	}

	_, ok := mustLoad(t, s)
	if !ok {
		t.Error("Finish(\"\") removed the draft")
	}
}

func TestFinishedTokensAreBounded(t *testing.T) {
	s := newTestStore(t)

	for i := range finishedLimit + 10 {
		err := s.Finish(string(rune('A' + i)))
		if err != nil {
			t.Fatal(err)
		}
	}

	if len(s.finished) != finishedLimit {
		t.Errorf("remembered %d finished tokens, want %d", len(s.finished), finishedLimit)
	}
}

func TestDiscard(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.Discard()
	if err != nil {
		t.Fatalf("Discard: %v", err)
	}
	_, ok := mustLoad(t, s)
	if ok {
		t.Fatal("draft still stored after Discard")
	}

	err = s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}
	_, ok = mustLoad(t, s)
	if ok {
		t.Error("late autosave brought a discarded draft back")
	}

	err = s.Discard()
	if err != nil {
		t.Errorf("Discard without a draft: %v", err)
	}
}

func TestDiscardEdit(t *testing.T) {
	s := newTestStore(t)
	err := s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}

	err = s.DiscardEdit(8)
	if err != nil {
		t.Fatalf("DiscardEdit(other): %v", err)
	}
	_, ok := mustLoad(t, s)
	if !ok {
		t.Fatal("draft of intention 7 removed when 8 was deleted")
	}

	err = s.DiscardEdit(7)
	if err != nil {
		t.Fatalf("DiscardEdit: %v", err)
	}
	_, ok = mustLoad(t, s)
	if ok {
		t.Fatal("draft of a deleted intention is still stored")
	}

	err = s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}
	_, ok = mustLoad(t, s)
	if ok {
		t.Error("late autosave brought back the draft of a deleted intention")
	}
}

func TestCorruptFileIsMovedAside(t *testing.T) {
	s := newTestStore(t)
	err := os.WriteFile(s.path, make([]byte, 64), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, ok, err := s.Load()
	if !errors.Is(err, ErrCorrupt) {
		t.Fatalf("Load error = %v, want ErrCorrupt", err)
	}
	if ok {
		t.Error("corrupt draft reported as found")
	}

	_, err = os.Stat(s.path + ".bad")
	if err != nil {
		t.Errorf("corrupt file not kept as .bad: %v", err)
	}

	_, ok = mustLoad(t, s)
	if ok {
		t.Error("draft found after the corrupt file was moved aside")
	}

	err = s.Save(sample("a"))
	if err != nil {
		t.Errorf("Save after corruption: %v", err)
	}
}

func TestEmptyFileContentIsRemoved(t *testing.T) {
	s := newTestStore(t)
	err := os.WriteFile(s.path, []byte(`{"token":"a"}`), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	_, ok := mustLoad(t, s)
	if ok {
		t.Error("draft without content reported as found")
	}

	_, err = os.Stat(s.path)
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("empty draft file not removed: %v", err)
	}
}

func TestStoreWithoutPath(t *testing.T) {
	s := NewStore("")

	err := s.Save(sample("a"))
	if err != nil {
		t.Fatal(err)
	}
	_, ok := mustLoad(t, s)
	if ok {
		t.Error("store without a path returned a draft")
	}
	err = s.Discard()
	if err != nil {
		t.Error(err)
	}
	err = s.Finish("a")
	if err != nil {
		t.Error(err)
	}
}
