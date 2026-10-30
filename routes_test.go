package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go-parafia/internal/masses"
)

func TestRouter(t *testing.T) {
	db, err := openDatabase(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	schedules, err := masses.Default()
	if err != nil {
		t.Fatalf("loading schedules: %v", err)
	}

	router := newRouter(db, schedules)

	tests := []struct {
		path            string
		wantContentType string
	}{
		{path: "/api/intentions", wantContentType: "application/json"},
		{path: "/", wantContentType: "text/html; charset=utf-8"},
		{path: "/static/app.css", wantContentType: "text/css; charset=utf-8"},
		{path: "/static/app.js", wantContentType: "text/javascript; charset=utf-8"},
		{path: "/pdf", wantContentType: "application/pdf"},
		{path: "/print", wantContentType: "text/html; charset=utf-8"},
		{path: "/settings", wantContentType: "text/html; charset=utf-8"},
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
