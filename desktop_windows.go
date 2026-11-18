//go:build desktop

package main

import (
	"errors"
	"log"
	"os"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

const (
	previousGrace         = 8 * time.Second
	errorInvalidParameter = syscall.Errno(87)
	mbIconError           = 0x10
	mbSetForeground       = 0x10000
)

type previousOutcome int

const (
	previousGone previousOutcome = iota
	previousClosed
	previousTerminated
	previousNewer
)

var shownLogFile string

// waitForPreviousVersion lets a freshly installed version wait until the
// version that installed it has closed, so the single instance lock and the
// database are free. A previous version that hangs is ended after a while.
func waitForPreviousVersion() {
	value := os.Getenv(waitPIDEnv)
	if value == "" {
		return
	}
	_ = os.Unsetenv(waitPIDEnv)

	pid, err := strconv.Atoi(value)
	if err != nil {
		log.Printf("update: invalid previous process id %q", value)
		return
	}

	started := time.Now()
	own, err := currentProcessStart()
	if err != nil {
		log.Printf("update: reading the start time of this process: %v", err)
		own = started
	}

	outcome, err := waitForProcess(pid, previousGrace, own)
	elapsed := time.Since(started).Round(time.Millisecond)
	switch {
	case err != nil:
		log.Printf("update: waiting for the previous version (pid %d): %v", pid, err)
	case outcome == previousClosed:
		log.Printf("update: previous version (pid %d) closed after %v", pid, elapsed)
	case outcome == previousTerminated:
		log.Printf("update: previous version (pid %d) did not close within %v and was ended", pid, previousGrace)
	case outcome == previousNewer:
		log.Printf("update: process %d is not the previous version, not waiting for it", pid)
	}
}

func waitForProcess(pid int, grace time.Duration, olderThan time.Time) (previousOutcome, error) {
	const access = syscall.SYNCHRONIZE | syscall.PROCESS_TERMINATE | syscall.PROCESS_QUERY_INFORMATION
	handle, err := syscall.OpenProcess(access, false, uint32(pid))
	if errors.Is(err, errorInvalidParameter) {
		return previousGone, nil
	}
	if err != nil {
		return previousGone, err
	}
	defer func() { _ = syscall.CloseHandle(handle) }()

	start, err := processStart(handle)
	if err != nil {
		return previousGone, err
	}
	if !start.Before(olderThan) {
		return previousNewer, nil
	}

	event, err := syscall.WaitForSingleObject(handle, uint32(grace.Milliseconds()))
	if err != nil {
		return previousGone, err
	}
	if event == syscall.WAIT_OBJECT_0 {
		return previousClosed, nil
	}

	err = syscall.TerminateProcess(handle, 1)
	if err != nil {
		return previousGone, err
	}
	_, _ = syscall.WaitForSingleObject(handle, 5000)

	return previousTerminated, nil
}

func currentProcessStart() (time.Time, error) {
	handle, err := syscall.GetCurrentProcess()
	if err != nil {
		return time.Time{}, err
	}

	return processStart(handle)
}

func processStart(handle syscall.Handle) (time.Time, error) {
	var created, exited, kernel, user syscall.Filetime
	err := syscall.GetProcessTimes(handle, &created, &exited, &kernel, &user)
	if err != nil {
		return time.Time{}, err
	}

	return time.Unix(0, created.Nanoseconds()), nil
}

func terminateSelf() {
	handle, err := syscall.GetCurrentProcess()
	if err == nil {
		_ = syscall.TerminateProcess(handle, 0)
	}
	os.Exit(0)
}

func fatal(message string, err error) {
	log.Printf("%s %v", message, err)

	text := message + "\n\n" + err.Error()
	if shownLogFile != "" {
		text += "\n\nSzczegóły są w dzienniku programu:\n" + shownLogFile
	}
	showError("Intencje mszalne", text)
	os.Exit(1)
}

var messageBox = syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")

func showError(title, text string) {
	textPtr, err := syscall.UTF16PtrFromString(text)
	if err != nil {
		return
	}
	titlePtr, err := syscall.UTF16PtrFromString(title)
	if err != nil {
		return
	}

	_, _, _ = messageBox.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), mbIconError|mbSetForeground)
}
