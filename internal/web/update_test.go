package web

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go-parafia/internal/intentions"
	"go-parafia/internal/update"
)

var updateNow = time.Date(2026, 11, 21, 9, 30, 0, 0, intentions.TimeZone)

func fakeRelease(t *testing.T, tag string, exe []byte) string {
	t.Helper()

	sum := sha256.Sum256(exe)
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, `{"tag_name": %q, "body": "Nowy wydruk.\nPoprawki.", "published_at": "2026-11-20T10:00:00Z", "assets": [
			{"name": %q, "browser_download_url": %q},
			{"name": %q, "browser_download_url": %q}]}`,
			tag, update.AssetName, srv.URL+"/exe", update.ChecksumsName, srv.URL+"/sums")
	})
	mux.HandleFunc("/exe", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(exe) })
	mux.HandleFunc("/sums", func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), update.AssetName)
	})
	srv = httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	return srv.URL
}

func postUpdate(t *testing.T, h *handler, path string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	if path == "/update/check" {
		h.checkUpdate(rec, req)
	} else {
		h.installUpdate(rec, req)
	}

	return rec
}

func installableUpdater(t *testing.T, api string, restarted chan string) *update.Updater {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "Intencje Mszalne.exe")
	err := os.WriteFile(exe, []byte("MZ 1.0.0"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	u := update.New("1.0.0", exe, func(path string) error {
		restarted <- path
		return nil
	})
	u.API = api

	return u
}

func waitForStage(t *testing.T, u *update.Updater, stages ...string) update.Progress {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		p := u.Progress()
		for _, stage := range stages {
			if p.Stage == stage {
				return p
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("update did not reach %q, progress = %+v", stages, u.Progress())

	return update.Progress{}
}

func getUpdatePage(t *testing.T, h *handler, target string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	if strings.HasPrefix(target, "/update/status") {
		h.updateStatus(rec, req)
	} else {
		h.updateProgress(rec, req)
	}

	return rec
}

func TestSettingsShowUpdatesSection(t *testing.T) {
	h := newTestHandler(t, updateNow)

	body := getSettings(t, h, "/settings?week=2026-11-23").Body.String()

	checkBody(t, body,
		`<section class="settings-section updates" id="aktualizacje" aria-labelledby="updates-title">`,
		`Zainstalowana wersja: <strong>1.0.0</strong></p>`,
		`<form method="post" action="/update/check">
							<input type="hidden" name="week" value="2026-11-23">
							<button class="button" type="submit">Sprawdź aktualizacje</button>`,
	)
	if strings.Contains(body, "Masz najnowszą wersję") || strings.Contains(body, "Dostępna nowa wersja") {
		t.Error("update result shown before checking")
	}
}

func TestCheckUpdateShowsNewVersion(t *testing.T) {
	h := newTestHandler(t, updateNow)
	h.updater.API = fakeRelease(t, "v1.1.0", []byte("MZ nowy"))

	rec := postUpdate(t, h, "/update/check", url.Values{"week": {"2026-11-23"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/settings?week=2026-11-23#aktualizacje"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	body := getSettings(t, h, "/settings?week=2026-11-23").Body.String()
	checkBody(t, body,
		`<p class="update-title">Dostępna nowa wersja <strong>1.1.0</strong> z 20.11.2026</p>`,
		"<p class=\"update-notes\">Nowy wydruk.\nPoprawki.</p>",
		`<p class="hint">Instalacja działa tylko w programie dla Windows.</p>`,
	)
	if strings.Contains(body, `action="/update/install"`) {
		t.Error("install button shown in the development server")
	}

	checkBody(t, getStart(t, h), `<a href="/settings#aktualizacje">Dostępna nowa wersja 1.1.0 – zaktualizuj w Ustawieniach</a>`)
}

func TestCheckUpdateUpToDateAndErrors(t *testing.T) {
	h := newTestHandler(t, updateNow)
	h.updater.API = fakeRelease(t, "v1.0.0", []byte("MZ"))

	postUpdate(t, h, "/update/check", url.Values{})
	checkBody(t, getSettings(t, h, "/settings").Body.String(), `<p class="update-ok">Masz najnowszą wersję.</p>`, ` · sprawdzono `)
	if strings.Contains(getStart(t, h), "start-update") {
		t.Error("start page offers an update without a newer version")
	}

	h.updater.API = "http://127.0.0.1:1"
	rec := postUpdate(t, h, "/update/check", url.Values{})
	if got := rec.Header().Get("Location"); got != "/settings#aktualizacje" {
		t.Errorf("Location = %q", got)
	}
	checkBody(t, getSettings(t, h, "/settings").Body.String(), `<p class="update-error" role="alert">Nie udało się sprawdzić aktualizacji. Sprawdź połączenie z internetem.</p>`)
}

func TestInstallUpdateInDevelopmentServer(t *testing.T) {
	h := newTestHandler(t, updateNow)

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	checkBody(t, getSettings(t, h, "/settings").Body.String(), `<p class="update-error" role="alert">Aktualizację można zainstalować tylko w programie dla Windows.</p>`)
}

func TestInstallUpdate(t *testing.T) {
	h := newTestHandler(t, updateNow)
	restarted := make(chan string, 1)
	h.updater = installableUpdater(t, fakeRelease(t, "v1.1.0", []byte("MZ nowy program")), restarted)
	postUpdate(t, h, "/update/check", url.Values{})

	body := getSettings(t, h, "/settings").Body.String()
	checkBody(t, body, `<input type="hidden" name="version" value="1.1.0">`, `Pobierz i zainstaluj</button>`)

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}, "week": {"2026-11-23"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/update?week=2026-11-23"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	select {
	case path := <-restarted:
		if path != h.updater.ExePath {
			t.Errorf("restarted %q, want %q", path, h.updater.ExePath)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the program was not restarted")
	}
	p := waitForStage(t, h.updater, update.StageRestart)
	if p.Done != int64(len("MZ nowy program")) || p.Total != p.Done {
		t.Errorf("download progress = %d of %d", p.Done, p.Total)
	}

	data, err := os.ReadFile(h.updater.ExePath)
	if err != nil || string(data) != "MZ nowy program" {
		t.Errorf("program after update = %q, %v", data, err)
	}
	backups, err := filepath.Glob(filepath.Join(h.dataDir, "data-2026-11-21-093000-przed-aktualizacja.db.bak"))
	if err != nil || len(backups) != 1 {
		t.Errorf("backup before the update: %v, %v", backups, err)
	}

	page := getUpdatePage(t, h, "/update?week=2026-11-23")
	checkBody(t, page.Body.String(),
		`<main class="start update-progress" data-update-progress="/update/status">`,
		`<p class="start-parish">Wersja 1.1.0</p>`,
		`<li class="is-done" data-step="backup">Kopia danych</li>`,
		`<li class="is-current" data-step="restart">Ponowne uruchomienie</li>`,
		`aria-valuenow="100"`,
		`<p class="update-label" role="status" aria-live="polite">Uruchamiam nową wersję… Program antywirusowy (np. Avast) może najpierw sprawdzić nowy plik – to potrwa do minuty. Nie zamykaj tego okna.</p>`,
		`<div class="update-failed" hidden>`,
	)

	status := getUpdatePage(t, h, "/update/status")
	if got := status.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q", got)
	}
	var got progressView
	err = json.Unmarshal(status.Body.Bytes(), &got)
	if err != nil {
		t.Fatal(err)
	}
	if got.Stage != "restart" || got.Percent != 100 || got.Failed || len(got.Steps) != 5 || got.Steps[4].State != "current" {
		t.Errorf("status = %+v", got)
	}

	checkBody(t, getSettings(t, h, "/settings").Body.String(), `Aktualizacja do wersji 1.1.0 trwa. <a href="/update">Zobacz postęp</a>`)
	rec = postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}})
	if got := rec.Header().Get("Location"); got != "/settings#aktualizacje" {
		t.Errorf("second install: Location = %q, want back to settings", got)
	}
}

func TestInstallUpdateBrokenDownload(t *testing.T) {
	h := newTestHandler(t, updateNow)
	restarted := make(chan string, 1)
	h.updater = installableUpdater(t, fakeRelease(t, "v1.1.0", []byte("<html>to nie program")), restarted)
	postUpdate(t, h, "/update/check", url.Values{})

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}, "week": {"2026-11-23"}})
	if got, want := rec.Header().Get("Location"), "/update?week=2026-11-23"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	waitForStage(t, h.updater, update.StageError)
	page := getUpdatePage(t, h, "/update?week=2026-11-23").Body.String()
	checkBody(t, page,
		`<div class="update-failed">`,
		`<p class="update-error" role="alert" data-update-error>Pobrany plik nie jest programem dla Windows.</p>`,
		`<input type="hidden" name="version" value="1.1.0">`,
		`<button class="button button-primary" type="submit">Spróbuj ponownie</button>`,
		`<a class="button" href="/settings?week=2026-11-23#aktualizacje">Wróć do ustawień</a>`,
		`<p class="update-note" hidden>`,
	)
	checkBody(t, getSettings(t, h, "/settings").Body.String(), `<p class="update-error" role="alert">Pobrany plik nie jest programem dla Windows.</p>`)
	if len(restarted) != 0 {
		t.Error("the program was restarted after a failed download")
	}
}

func TestUpdateProgressWithoutUpdateRedirects(t *testing.T) {
	h := newTestHandler(t, updateNow)

	rec := getUpdatePage(t, h, "/update?week=2026-11-23")

	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings?week=2026-11-23#aktualizacje" {
		t.Errorf("status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestProgressLabels(t *testing.T) {
	tests := []struct {
		p       update.Progress
		label   string
		percent int
	}{
		{update.Progress{Stage: update.StageBackup}, "Zapisuję kopię danych…", 0},
		{update.Progress{Stage: update.StageDownload, Done: 5 << 20, Total: 20 << 20}, "Pobieram nową wersję… 5,0 MB z 20,0 MB", 25},
		{update.Progress{Stage: update.StageDownload, Done: 3 << 19}, "Pobieram nową wersję… 1,5 MB", 0},
		{update.Progress{Stage: update.StageVerify}, "Sprawdzam pobrany plik…", 100},
		{update.Progress{Stage: update.StageInstall}, "Instaluję nową wersję…", 100},
		{update.Progress{Stage: update.StageError, Err: "x"}, "Aktualizacja nie powiodła się.", 0},
	}
	for _, tt := range tests {
		view := newProgressView(tt.p)
		if view.Label != tt.label || view.Percent != tt.percent {
			t.Errorf("%s: label %q, percent %d; want %q, %d", tt.p.Stage, view.Label, view.Percent, tt.label, tt.percent)
		}
	}
}

func TestInstallUpdateStopsWhenBackupFails(t *testing.T) {
	h := newTestHandler(t, updateNow)
	restarted := make(chan string, 1)
	h.updater = installableUpdater(t, fakeRelease(t, "v1.1.0", []byte("MZ nowy program")), restarted)
	h.dataDir = filepath.Join(t.TempDir(), "brak-folderu")
	postUpdate(t, h, "/update/check", url.Values{})

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}})
	if got := rec.Header().Get("Location"); got != "/update" {
		t.Fatalf("Location = %q, want /update", got)
	}

	p := waitForStage(t, h.updater, update.StageError)
	if p.Err != "Nie udało się zapisać kopii danych przed aktualizacją, więc aktualizacja została przerwana." {
		t.Errorf("error = %q", p.Err)
	}
	data, err := os.ReadFile(h.updater.ExePath)
	if err != nil || string(data) != "MZ 1.0.0" {
		t.Errorf("program changed although the backup failed: %q, %v", data, err)
	}
	if len(restarted) != 0 {
		t.Error("the program was restarted although the backup failed")
	}
}

func TestSettingsLayout(t *testing.T) {
	h := newTestHandler(t, updateNow)
	h.version = "1.0.2"

	body := getSettings(t, h, "/settings?week=2026-11-23").Body.String()

	checkBody(t, body,
		`<main class="settings settings-page">`,
		`<nav class="settings-nav" aria-label="Sekcje ustawień">`,
		`<p class="app-credit">Go Parafia 1.0.2 · autor: Tomasz Kaliński, Rzezawa</p>`,
	)
	for _, id := range []string{"parafia", "wplaty", "swieta", "porzadki", "kosz", "kopia", "aktualizacje"} {
		checkBody(t, body, `<li><a href="#`+id+`">`, `id="`+id+`"`)
	}
	if got := strings.Count(body, `<section class="settings-section`); got != 7 {
		t.Errorf("sections = %d, want 7 cards", got)
	}
	if got := strings.Count(body, `<header class="settings-section-head">`); got != 7 {
		t.Errorf("section headers = %d, want one per card", got)
	}
	if strings.Contains(body, "<legend>") {
		t.Error("settings still use fieldset legends")
	}
}
