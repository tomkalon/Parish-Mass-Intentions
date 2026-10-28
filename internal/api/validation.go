package api

import (
	"encoding/json"
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]any{"errors": messages})
}
