//go:build desktop

package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"go-parafia/internal/backup"
)

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

	logFile, err := os.OpenFile(filepath.Join(dir, "go-parafia.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return "", err
	}
	log.SetOutput(logFile)

	return dir, nil
}

var window = &desktopWindow{}

func saveFileDialog() func(string) (string, error) {
	return window.saveFile
}

func chooseDirDialog() func(string) (string, error) {
	return window.chooseDir
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
	mu      sync.Mutex
	ctx     context.Context
	backups *backup.Backups
}

func (w *desktopWindow) startup(ctx context.Context) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.ctx = ctx
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
