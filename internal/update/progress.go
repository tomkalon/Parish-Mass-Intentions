package update

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"time"
	"unicode"
	"unicode/utf8"
)

const installTimeout = 10 * time.Minute

const (
	StageIdle     = ""
	StageBackup   = "backup"
	StageDownload = "download"
	StageVerify   = "verify"
	StageInstall  = "install"
	StageRestart  = "restart"
	StageError    = "error"
)

var ErrBackup = errors.New("nie udało się zapisać kopii danych przed aktualizacją, więc aktualizacja została przerwana")

type Progress struct {
	Version string
	Stage   string
	Done    int64
	Total   int64
	Err     string
}

func (p Progress) Active() bool {
	return p.Stage != StageIdle && p.Stage != StageError
}

func (u *Updater) Progress() Progress {
	u.mu.Lock()
	defer u.mu.Unlock()

	return u.progress
}

// Start runs prepare (the data backup) and the installation in the
// background; Progress reports how far it got.
func (u *Updater) Start(version string, prepare func() error) error {
	release, err := u.begin(version)
	if err != nil {
		return err
	}

	go func() {
		defer func() {
			if p := recover(); p != nil {
				log.Printf("panic while installing update %s: %v\n%s", version, p, debug.Stack())
				u.fail(fmt.Errorf("unexpected error: %v", p))
			}
		}()

		ctx, cancel := context.WithTimeout(context.Background(), installTimeout)
		defer cancel()

		err := u.run(ctx, release, prepare)
		if err != nil {
			u.fail(err)
		}
	}()

	return nil
}

func (u *Updater) setStage(stage string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.progress.Stage = stage
}

func (u *Updater) setDownloadTotal(total int64) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.progress.Done = 0
	u.progress.Total = max(total, 0)
}

type progressWriter struct {
	u *Updater
}

func (w progressWriter) Write(b []byte) (int, error) {
	w.u.mu.Lock()
	w.u.progress.Done += int64(len(b))
	w.u.mu.Unlock()

	return len(b), nil
}

// Message is the sentence shown to the user for an installation error.
func Message(err error) string {
	for _, known := range []error{ErrBackup, ErrStart, ErrChecksum, ErrNotProgram, ErrMissingAsset, ErrNotAvailable, ErrInProgress, ErrCannotInstall} {
		if errors.Is(err, known) {
			return capitalize(known.Error()) + "."
		}
	}

	return "Nie udało się zainstalować aktualizacji. Sprawdź połączenie z internetem i spróbuj ponownie – program działa dalej w obecnej wersji."
}

func capitalize(text string) string {
	r, size := utf8.DecodeRuneInString(text)

	return string(unicode.ToUpper(r)) + text[size:]
}
