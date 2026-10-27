package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"
)

func newRouter(db *gorm.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RedirectSlashes)

	h := &intentionHandler{db: db}

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Witaj w systemie parafialnym!")
	})

	router.Get("/intentions", h.list)
	router.Get("/intentions/{id}", h.show)

	return router
}
