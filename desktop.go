//go:build desktop

package main

import (
	"context"
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

func run(router http.Handler, dataDir string) error {
	window := &desktopWindow{}

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
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "go-parafia-b6a1c0e4-intencje",
			OnSecondInstanceLaunch: window.bringToFront,
		},
	})
}

type desktopWindow struct {
	mu  sync.Mutex
	ctx context.Context
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
