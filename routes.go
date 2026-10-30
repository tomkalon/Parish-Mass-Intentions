package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"go-parafia/internal/api"
	"go-parafia/internal/masses"
	"go-parafia/internal/web"
)

func newRouter(db *gorm.DB, schedules []masses.Schedule) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RedirectSlashes)

	router.Route("/api", func(r chi.Router) {
		api.RegisterRoutes(r, db)
	})

	web.RegisterRoutes(router, db, schedules)

	return router
}
