//go:build !desktop

package main

import (
	"log"
	"net/http"
	"time"
)

func prepareDataDir() (string, error) {
	return ".", nil
}

func run(router http.Handler) error {
	server := &http.Server{
		Addr:              "127.0.0.1:8080",
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	log.Println("Go Parafia: http://localhost:8080")

	return server.ListenAndServe()
}
