package web

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
)

func RegisterRoutes(router chi.Router, db *gorm.DB) {
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "Witaj w systemie parafialnym!")
	})
}
