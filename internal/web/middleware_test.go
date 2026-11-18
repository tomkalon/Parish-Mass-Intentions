package web

import (
	"bytes"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()

	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	return &buf
}

func serveThrough(handler http.HandlerFunc, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	RecoverPanics(handler).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))

	return rec
}

func TestRecoverPanicsShowsErrorPage(t *testing.T) {
	logs := captureLog(t)

	rec := serveThrough(func(http.ResponseWriter, *http.Request) {
		var view *searchView
		_ = view.Total
	}, "/search?q=kowal")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	checkBody(t, rec.Body.String(), `<h1>Coś poszło nie tak</h1>`, `<a class="button button-primary" href="/">Wróć do strony głównej</a>`)
	if got := rec.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q", got)
	}
	if !strings.Contains(logs.String(), "panic in GET /search?q=kowal: runtime error: invalid memory address") || !strings.Contains(logs.String(), "middleware_test.go") {
		t.Errorf("log = %q, want the panic with its stack", logs.String())
	}
}

func TestRecoverPanicsKeepsStartedResponse(t *testing.T) {
	logs := captureLog(t)

	rec := serveThrough(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("part of a page"))
		panic("broken template")
	}, "/")

	if rec.Code != http.StatusOK || rec.Body.String() != "part of a page" {
		t.Errorf("response = %d %q, want the part already sent", rec.Code, rec.Body.String())
	}
	if !strings.Contains(logs.String(), "panic in GET /: broken template") {
		t.Errorf("log = %q", logs.String())
	}
}

func TestRecoverPanicsPassesAbortHandler(t *testing.T) {
	captureLog(t)
	defer func() {
		if p := recover(); p != http.ErrAbortHandler {
			t.Errorf("recovered %v, want http.ErrAbortHandler", p)
		}
	}()

	serveThrough(func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) }, "/")
	t.Error("ErrAbortHandler was swallowed")
}

func TestRecoverPanicsLogsFailedAndSlowRequests(t *testing.T) {
	logs := captureLog(t)
	slow := slowRequest
	slowRequest = 20 * time.Millisecond
	t.Cleanup(func() { slowRequest = slow })

	serveThrough(func(w http.ResponseWriter, _ *http.Request) { serverError(w) }, "/settings")
	serveThrough(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(30 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}, "/pdf?week=2026-11-23")
	serveThrough(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }, "/fast")

	got := logs.String()
	if !strings.Contains(got, "GET /settings: status 500 in") {
		t.Errorf("log = %q, want the failed request", got)
	}
	if !strings.Contains(got, "GET /pdf?week=2026-11-23: status 200 in") {
		t.Errorf("log = %q, want the slow request", got)
	}
	if strings.Contains(got, "/fast") {
		t.Errorf("log = %q, want no entry for a fast request", got)
	}
}

func TestServerErrorPage(t *testing.T) {
	rec := httptest.NewRecorder()
	serverError(rec)

	if rec.Code != http.StatusInternalServerError || rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("response = %d %v", rec.Code, rec.Header())
	}
	checkBody(t, rec.Body.String(), `<html lang="pl">`, `Szczegóły zapisano w dzienniku programu`, `href="/settings#kopia"`)
}
