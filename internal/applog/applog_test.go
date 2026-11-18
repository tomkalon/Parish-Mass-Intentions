package applog

import (
	"bytes"
	"log"
	"os"
	"runtime/debug"
	"strings"
	"testing"
)

func restoreLog(t *testing.T) {
	t.Helper()

	flags := log.Flags()
	t.Cleanup(func() {
		log.SetOutput(os.Stderr)
		log.SetFlags(flags)
		_ = debug.SetCrashOutput(nil, debug.CrashOptions{})
	})
}

func TestOpenAppendsToTheLog(t *testing.T) {
	restoreLog(t)
	dir := t.TempDir()
	err := os.WriteFile(Path(dir), []byte("earlier line\n"), 0o644)
	if err != nil {
		t.Fatal(err)
	}

	file, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("new line")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "earlier line\n") || !strings.Contains(string(data), "new line") {
		t.Errorf("log = %q, want the earlier line followed by the new one", data)
	}
	if _, err := os.Stat(Path(dir) + ".1"); !os.IsNotExist(err) {
		t.Errorf("small log was rotated: %v", err)
	}
	if log.Flags()&log.Lmicroseconds == 0 {
		t.Error("log lines have no sub-second time")
	}
}

func TestOpenRotatesALargeLog(t *testing.T) {
	restoreLog(t)
	dir := t.TempDir()
	old := bytes.Repeat([]byte("x"), maxSize+1)
	if err := os.WriteFile(Path(dir), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(dir)+".1", []byte("oldest"), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Printf("after rotation")
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	rotated, err := os.ReadFile(Path(dir) + ".1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(rotated, old) {
		t.Errorf("rotated log has %d bytes, want the previous %d", len(rotated), len(old))
	}
	current, err := os.ReadFile(Path(dir))
	if err != nil {
		t.Fatal(err)
	}
	if len(current) > 200 || !strings.Contains(string(current), "after rotation") {
		t.Errorf("current log = %q, want only the new line", current)
	}
}

func TestOpenFailsWithoutFolder(t *testing.T) {
	restoreLog(t)
	if _, err := Open(Path(t.TempDir()) + "-missing"); err == nil {
		t.Error("Open succeeded in a folder that does not exist")
	}
}

func TestRecoverLogsThePanic(t *testing.T) {
	restoreLog(t)
	var buf bytes.Buffer
	log.SetOutput(&buf)

	done := make(chan struct{})
	go func() {
		defer close(done)
		defer Recover("checking for updates")
		panic("no network")
	}()
	<-done

	if !strings.Contains(buf.String(), "panic while checking for updates: no network") || !strings.Contains(buf.String(), "applog_test.go") {
		t.Errorf("log = %q, want the panic with its stack", buf.String())
	}
}
