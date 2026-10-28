package api

import (
	"log"
	"net/http"

	"go-parafia/internal/intentions"
)

func writeValidationError(w http.ResponseWriter, err error) {
	messages, ok := intentions.ValidationMessages(err)
	if !ok {
		log.Printf("unexpected validation error: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusBadRequest, map[string]any{"errors": messages})
}
