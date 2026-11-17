package safefile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteReplacesFileAndLeavesNoTemp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.json")

	for _, content := range []string{"first", "second"} {
		err := Write(path, []byte(content), 0o644)
		if err != nil {
			t.Fatalf("Write(%q): %v", content, err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != content {
			t.Errorf("file = %q, want %q", got, content)
		}
	}

	_, err := os.Stat(path + ".tmp")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary file left behind: %v", err)
	}
}

func TestWriteFailsInMissingDir(t *testing.T) {
	err := Write(filepath.Join(t.TempDir(), "missing", "state.json"), []byte("x"), 0o644)
	if err == nil {
		t.Fatal("Write into a missing directory succeeded")
	}
}

func TestSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "copy.db")
	err := os.WriteFile(path, []byte("data"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	err = Sync(path)
	if err != nil {
		t.Errorf("Sync: %v", err)
	}

	err = Sync(filepath.Join(t.TempDir(), "missing.db"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Sync(missing) = %v, want ErrNotExist", err)
	}
}
