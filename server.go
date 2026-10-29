//go:build !desktop

package main

import (
	"log"
	"net/http"
)

func prepareDataDir() (string, error) {
	return ".", nil
}

func run(router http.Handler) error {
	log.Println("Go Parafia: http://localhost:8080")

	return http.ListenAndServe(":8080", router)
}
