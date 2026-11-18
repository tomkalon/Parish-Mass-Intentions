package applog

import (
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
)

const (
	FileName = "go-parafia.log"
	maxSize  = 1 << 20
)

func Path(dir string) string {
	return filepath.Join(dir, FileName)
}

func Open(dir string) (*os.File, error) {
	path := Path(dir)
	if info, err := os.Stat(path); err == nil && info.Size() > maxSize {
		_ = os.Rename(path, path+".1")
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}

	log.SetOutput(file)
	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	err = debug.SetCrashOutput(file, debug.CrashOptions{})
	if err != nil {
		log.Printf("error sending crash reports to the log: %v", err)
	}

	return file, nil
}

func Recover(task string) {
	if p := recover(); p != nil {
		log.Printf("panic while %s: %v\n%s", task, p, debug.Stack())
	}
}
