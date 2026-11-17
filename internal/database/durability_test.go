package database

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

const crashDBEnv = "GO_PARAFIA_CRASH_DB"

func TestOpenUsesFullSynchronousMode(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "data.db"))

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(3)

	for range 3 {
		var mode int
		err = db.Raw("PRAGMA synchronous").Scan(&mode).Error
		if err != nil {
			t.Fatal(err)
		}
		if mode != 2 {
			t.Errorf("PRAGMA synchronous = %d, want 2 (FULL)", mode)
		}
	}
}

func TestDSN(t *testing.T) {
	tests := map[string]string{
		`C:\Users\Ks\AppData\Roaming\Go Parafia\data.db`: `C:\Users\Ks\AppData\Roaming\Go Parafia\data.db?_pragma=synchronous(FULL)`,
		"file:data.db?mode=rwc":                          "file:data.db?mode=rwc&_pragma=synchronous(FULL)",
	}
	for path, want := range tests {
		if got := dsn(path); got != want {
			t.Errorf("dsn(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestCheck(t *testing.T) {
	path := filepath.Join(t.TempDir(), "data.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 400 {
		intention := intentions.Intention{
			MassAt:  time.Date(2026, 1, 1, 18, 0, 0, 0, intentions.TimeZone).AddDate(0, 0, i),
			Content: fmt.Sprintf("intencja numer %d z dłuższym opisem, żeby zająć kilka stron bazy", i),
		}
		err = intentions.Save(db, &intention)
		if err != nil {
			t.Fatal(err)
		}
	}

	err = Check(db)
	if err != nil {
		t.Fatalf("Check on a healthy database: %v", err)
	}
	mustClose(t, db)

	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	garbage := make([]byte, 4096)
	for i := range garbage {
		garbage[i] = byte(i*31 + 7)
	}
	_, err = f.WriteAt(garbage, 3*4096)
	err = errors.Join(err, f.Close())
	if err != nil {
		t.Fatal(err)
	}

	damaged, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mustClose(t, damaged) })

	err = Check(damaged)
	if !errors.Is(err, ErrDamaged) {
		t.Errorf("Check on a damaged database = %v, want ErrDamaged", err)
	}
}

func TestExportLeavesNoTemporaryFile(t *testing.T) {
	db := openTestDB(t, filepath.Join(t.TempDir(), "data.db"))
	target := filepath.Join(t.TempDir(), "kopia.db")

	err := Export(db, target)
	if err != nil {
		t.Fatalf("Export: %v", err)
	}

	_, err = os.Stat(target + ".tmp")
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("temporary copy left behind: %v", err)
	}
}

// TestCrashHelper is not a real test: TestCommittedIntentionsSurviveKill runs
// the test binary again with only this function, then kills it mid-write.
func TestCrashHelper(t *testing.T) {
	path := os.Getenv(crashDBEnv)
	if path == "" {
		t.Skip("helper process for TestCommittedIntentionsSurviveKill")
	}

	db, err := Open(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	start := time.Date(2026, 1, 1, 6, 30, 0, 0, intentions.TimeZone)
	for i := 0; ; i++ {
		intention := intentions.Intention{
			MassAt:  start.Add(time.Duration(i) * 15 * time.Minute),
			Content: fmt.Sprintf("intencja %d", i),
			Amount:  intentions.Grosze(i * 100),
		}
		err = intentions.Save(db, &intention)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println(intention.ID)
	}
}

func TestCommittedIntentionsSurviveKill(t *testing.T) {
	if testing.Short() {
		t.Skip("starts helper processes")
	}

	path := filepath.Join(t.TempDir(), "data.db")
	var committed []int

	for round := range 3 {
		ids := runAndKill(t, path, 60+round*37)
		if len(ids) == 0 {
			t.Fatalf("round %d: helper committed nothing", round)
		}
		committed = append(committed, ids...)

		db, err := Open(path)
		if err != nil {
			t.Fatalf("round %d: opening after kill: %v", round, err)
		}
		err = Check(db)
		if err != nil {
			t.Errorf("round %d: %v", round, err)
		}

		var found []int
		err = db.Model(&intentions.Intention{}).Where("id IN ?", committed).Pluck("id", &found).Error
		if err != nil {
			t.Fatal(err)
		}
		if len(found) != len(committed) {
			t.Errorf("round %d: %d of %d committed intentions survived the kill", round, len(found), len(committed))
		}
		mustClose(t, db)
	}
}

func runAndKill(t *testing.T, path string, after int) []int {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestCrashHelper$")
	cmd.Env = append(os.Environ(), crashDBEnv+"="+path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	var ids []int
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		id, err := strconv.Atoi(scanner.Text())
		if err != nil {
			continue
		}
		ids = append(ids, id)
		if len(ids) == after {
			err = cmd.Process.Kill()
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	err = cmd.Wait()
	if err == nil {
		t.Fatal("helper process ended on its own")
	}

	return ids
}
