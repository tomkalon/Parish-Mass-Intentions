package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func TestRouter(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&intentions.Intention{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	router := newRouter(db)

	tests := []struct {
		path            string
		wantContentType string
	}{
		{path: "/api/intentions", wantContentType: "application/json"},
		{path: "/", wantContentType: "text/html; charset=utf-8"},
		{path: "/static/app.css", wantContentType: "text/css; charset=utf-8"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			router.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if got := rec.Header().Get("Content-Type"); got != tt.wantContentType {
				t.Errorf("Content-Type = %q, want %q", got, tt.wantContentType)
			}
		})
	}
}
