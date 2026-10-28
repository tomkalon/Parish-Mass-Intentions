package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"go-parafia/internal/api"
	"go-parafia/internal/web"
)

func newRouter(db *gorm.DB) http.Handler {
	router := chi.NewRouter()
	router.Use(middleware.RedirectSlashes)

	api.RegisterRoutes(router, db)
	web.RegisterRoutes(router, db)

	return router
}
