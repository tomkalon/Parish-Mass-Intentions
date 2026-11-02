//go:build !desktop

package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func prepareDataDir() (string, error) {
	return os.Getwd()
}

func run(router http.Handler, _ string) error {
	server := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Go Parafia: http://localhost:8080")

	return server.ListenAndServe()
}

func saveFileDialog() func(string) (string, error) {
	return nil
}
