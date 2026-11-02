package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"go-parafia/internal/backup"
	"go-parafia/internal/database"
	"go-parafia/internal/web"
)

func TestRouter(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	backups, err := backup.Load(db, "")
	if err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, web.Options{DataDir: dir, Backups: backups})

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

func TestRouterTracksAPIChanges(t *testing.T) {
	dir := t.TempDir()
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	backups, err := backup.Load(db, "")
	if err != nil {
		t.Fatal(err)
	}
	err = backups.SetDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	_, err = backups.Run()
	if err != nil {
		t.Fatal(err)
	}

	router := newRouter(db, web.Options{DataDir: dir, Backups: backups})
	serve := func(method, body string) int {
		req := httptest.NewRequest(method, "/api/intentions", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := serve(http.MethodGet, ""); code != http.StatusOK || backups.Pending() {
		t.Errorf("GET: status %d, pending %v; want 200 and no change", code, backups.Pending())
	}
	if code := serve(http.MethodPost, `{"mass_at":"2026-11-02 18:00","content":"+ Jan Kowalski"}`); code != http.StatusCreated {
		t.Fatalf("POST: status = %d, want %d", code, http.StatusCreated)
	}
	if !backups.Pending() {
		t.Error("creating an intention through the API did not mark the data as changed")
	}
}
