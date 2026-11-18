package web

import (
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

var (
	slowRequest = time.Second
	errorPage   = mustReadTemplate("templates/error.html")
)

func mustReadTemplate(name string) string {
	data, err := templateFS.ReadFile(name)
	if err != nil {
		panic(err)
	}

	return string(data)
}

func serverError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = io.WriteString(w, errorPage)
}

// RecoverPanics keeps a failing page from closing the whole program and logs
// failed and slow requests.
func RecoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		defer func() {
			if p := recover(); p != nil {
				if p == http.ErrAbortHandler {
					panic(p)
				}
				log.Printf("panic in %s %s: %v\n%s", r.Method, r.URL.RequestURI(), p, debug.Stack())
				if ww.Status() == 0 {
					serverError(ww)
				}
				return
			}

			elapsed := time.Since(start)
			if status := ww.Status(); status >= http.StatusInternalServerError || elapsed >= slowRequest {
				log.Printf("%s %s: status %d in %v", r.Method, r.URL.RequestURI(), status, elapsed.Round(time.Millisecond))
			}
		}()

		next.ServeHTTP(ww, r)
	})
}
