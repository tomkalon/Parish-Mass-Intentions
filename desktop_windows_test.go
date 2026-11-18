//go:build desktop

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

const helperEnv = "GO_PARAFIA_TEST_HELPER_SLEEP"

func TestHelperProcess(t *testing.T) {
	value := os.Getenv(helperEnv)
	if value == "" {
		t.Skip("helper process only")
	}
	sleep, err := time.ParseDuration(value)
	if err != nil {
		os.Exit(2)
	}
	time.Sleep(sleep)
	os.Exit(0)
}

func startHelper(t *testing.T, sleep time.Duration) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"="+sleep.String())
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	return cmd
}

func TestWaitForProcessThatCloses(t *testing.T) {
	cmd := startHelper(t, 300*time.Millisecond)
	start := time.Now()

	outcome, err := waitForProcess(cmd.Process.Pid, 10*time.Second, time.Now().Add(time.Minute))

	if err != nil || outcome != previousClosed {
		t.Fatalf("outcome = %v, %v, want closed", outcome, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("waited %v for a process that closed by itself", elapsed)
	}
}

func TestWaitForProcessEndsAHangingOne(t *testing.T) {
	cmd := startHelper(t, time.Minute)
	start := time.Now()

	outcome, err := waitForProcess(cmd.Process.Pid, 500*time.Millisecond, time.Now().Add(time.Minute))

	if err != nil || outcome != previousTerminated {
		t.Fatalf("outcome = %v, %v, want terminated", outcome, err)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Errorf("took %v to end a hanging process", elapsed)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			t.Error("the ended process reports a clean exit")
		}
	case <-time.After(5 * time.Second):
		t.Error("the hanging process is still running")
	}
}

func TestWaitForProcessLeavesANewerOne(t *testing.T) {
	cmd := startHelper(t, time.Minute)
	start := time.Now()

	outcome, err := waitForProcess(cmd.Process.Pid, 5*time.Second, time.Now().Add(-time.Hour))

	if err != nil || outcome != previousNewer {
		t.Fatalf("outcome = %v, %v, want newer", outcome, err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v for a process that is not the previous version", elapsed)
	}
	if cmd.ProcessState != nil {
		t.Error("the newer process was stopped")
	}
}

func TestWaitForProcessThatIsGone(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperProcess$")
	cmd.Env = append(os.Environ(), helperEnv+"=1ms")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}

	outcome, err := waitForProcess(cmd.Process.Pid, 5*time.Second, time.Now().Add(time.Minute))

	if err != nil || (outcome != previousGone && outcome != previousClosed) {
		t.Errorf("outcome = %v, %v, want gone", outcome, err)
	}
}

func TestCurrentProcessStart(t *testing.T) {
	start, err := currentProcessStart()
	if err != nil {
		t.Fatal(err)
	}
	if start.After(time.Now()) || time.Since(start) > time.Hour {
		t.Errorf("start = %v, want a moment ago", start)
	}
}
