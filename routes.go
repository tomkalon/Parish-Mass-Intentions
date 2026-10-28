package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"go-parafia/internal/api"
)

func newRouter(db *gorm.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RedirectSlashes)

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintln(w, "Witaj w systemie parafialnym!")
	})

	api.RegisterRoutes(router, db)

	return router
}
