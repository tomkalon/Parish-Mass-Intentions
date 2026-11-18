//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"go-parafia/internal/applog"
	"go-parafia/internal/backup"
	"go-parafia/internal/update"
)

const waitPIDEnv = "GO_PARAFIA_WAIT_PID"

func prepareDataDir() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(config, "Go Parafia")
	err = os.MkdirAll(dir, 0o755)
	if err != nil {
		return "", err
	}

	_, err = applog.Open(dir)
	if err != nil {
		return "", err
	}
	shownLogFile = applog.Path(dir)

	return dir, nil
}

func logFile(dir string) string {
	return applog.Path(dir)
}

var window = &desktopWindow{}

func saveFileDialog() func(string) (string, error) {
	return window.saveFile
}

func chooseDirDialog() func(string) (string, error) {
	return window.chooseDir
}

func programPath() string {
	exe, err := os.Executable()
	if err != nil {
		log.Printf("error finding the program file: %v", err)
		return ""
	}
	resolved, err := filepath.EvalSymlinks(exe)
	if err == nil {
		exe = resolved
	}

	return exe
}

func restartProgram() func(string) error {
	return window.restart
}

func run(router http.Handler, dataDir string, backups *backup.Backups) error {
	window.backups = backups

	return wails.Run(&options.App{
		Title:            "Go Parafia – intencje mszalne",
		Width:            1280,
		Height:           800,
		MinWidth:         1024,
		MinHeight:        700,
		BackgroundColour: &options.RGBA{R: 0xF6, G: 0xF1, B: 0xE9, A: 0xFF},
		AssetServer:      &assetserver.Options{Handler: router},
		Windows:          &windows.Options{WebviewUserDataPath: filepath.Join(dataDir, "webview")},
		OnStartup:        window.startup,
		OnBeforeClose:    window.beforeClose,
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "go-parafia-b6a1c0e4-intencje",
			OnSecondInstanceLaunch: window.bringToFront,
		},
	})
}

type desktopWindow struct {
	mu       sync.Mutex
	ctx      context.Context
	backups  *backup.Backups
	updating atomic.Bool
}

func (w *desktopWindow) startup(ctx context.Context) {
	w.mu.Lock()
	w.ctx = ctx
	w.mu.Unlock()

	err := update.Cleanup(programPath())
	if err != nil {
		log.Printf("error removing files left by the update: %v", err)
	}
}

func (w *desktopWindow) restart(exe string) error {
	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), waitPIDEnv+"="+strconv.Itoa(os.Getpid()))
	err := cmd.Start()
	if err != nil {
		return err
	}
	log.Printf("update: started the new version (pid %d)", cmd.Process.Pid)
	err = cmd.Process.Release()
	if err != nil {
		log.Printf("error releasing the new program process: %v", err)
	}

	w.updating.Store(true)
	time.AfterFunc(1500*time.Millisecond, func() {
		time.AfterFunc(3*time.Second, func() {
			log.Printf("update: this window did not close, ending the process")
			terminateSelf()
		})

		ctx, err := w.context()
		if err != nil {
			terminateSelf()
		}
		log.Printf("update: closing this window")
		runtime.Quit(ctx)
	})

	return nil
}

func (w *desktopWindow) bringToFront(options.SecondInstanceData) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ctx != nil {
		runtime.WindowShow(w.ctx)
	}
}

func (w *desktopWindow) saveFile(name string) (string, error) {
	ctx, err := w.context()
	if err != nil {
		return "", err
	}

	return runtime.SaveFileDialog(ctx, runtime.SaveDialogOptions{
		Title:           "Eksport danych",
		DefaultFilename: name,
		Filters:         []runtime.FileFilter{{DisplayName: "Kopia danych Go Parafia (*.db)", Pattern: "*.db"}},
	})
}

func (w *desktopWindow) context() (context.Context, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.ctx == nil {
		return nil, errors.New("window is not ready")
	}

	return w.ctx, nil
}

func (w *desktopWindow) chooseDir(current string) (string, error) {
	ctx, err := w.context()
	if err != nil {
		return "", err
	}

	if info, err := os.Stat(current); err != nil || !info.IsDir() {
		current = ""
	}

	return runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title:            "Folder kopii danych",
		DefaultDirectory: current,
	})
}

func (w *desktopWindow) beforeClose(ctx context.Context) bool {
	if w.updating.Load() {
		return false
	}

	err := w.backups.RunIfPending()
	if err == nil {
		return false
	}
	log.Printf("error making backup before closing: %v", err)

	answer, dialogErr := runtime.MessageDialog(ctx, runtime.MessageDialogOptions{
		Type:          runtime.QuestionDialog,
		Title:         "Kopia danych",
		Message:       fmt.Sprintf("Nie udało się zapisać kopii danych w folderze:\n%s\n\n%v\n\nZamknąć program mimo to?", w.backups.State().Dir, err),
		DefaultButton: "No",
	})
	if dialogErr != nil {
		log.Printf("error showing close dialog: %v", dialogErr)
		return false
	}

	return answer != "Yes"
}
