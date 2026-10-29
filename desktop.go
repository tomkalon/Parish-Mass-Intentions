//go:build desktop

package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
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

func run(router http.Handler) error {
	return wails.Run(&options.App{
		Title:            "Go Parafia – intencje mszalne",
		Width:            1280,
		Height:           800,
		MinWidth:         1024,
		MinHeight:        700,
		BackgroundColour: &options.RGBA{R: 0xF6, G: 0xF1, B: 0xE9, A: 0xFF},
		AssetServer:      &assetserver.Options{Handler: router},
	})
}
