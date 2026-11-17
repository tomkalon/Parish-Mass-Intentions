package web

import (
	"crypto/sha256"
	"encoding/hex"
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

func installableUpdater(t *testing.T, api string, restarts *[]string) *update.Updater {
	t.Helper()

	exe := filepath.Join(t.TempDir(), "Intencje Mszalne.exe")
	err := os.WriteFile(exe, []byte("MZ 1.0.0"), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	u := update.New("1.0.0", exe, func(path string) error {
		*restarts = append(*restarts, path)
		return nil
	})
	u.API = api

	return u
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
	var restarts []string
	h.updater = installableUpdater(t, fakeRelease(t, "v1.1.0", []byte("MZ nowy program")), &restarts)
	postUpdate(t, h, "/update/check", url.Values{})

	body := getSettings(t, h, "/settings").Body.String()
	checkBody(t, body, `<input type="hidden" name="version" value="1.1.0">`, `Pobierz i zainstaluj</button>`)

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	checkBody(t, rec.Body.String(), `Pobrano wersję 1.1.0. Program za chwilę zamknie się i uruchomi ponownie.`)

	if len(restarts) != 1 {
		t.Errorf("restarts = %q, want one", restarts)
	}
	data, err := os.ReadFile(h.updater.ExePath)
	if err != nil || string(data) != "MZ nowy program" {
		t.Errorf("program after update = %q, %v", data, err)
	}

	backups, err := filepath.Glob(filepath.Join(h.dataDir, "data-2026-11-21-093000-przed-aktualizacja.db.bak"))
	if err != nil || len(backups) != 1 {
		t.Errorf("backup before the update: %v, %v", backups, err)
	}
}

func TestInstallUpdateBrokenDownload(t *testing.T) {
	h := newTestHandler(t, updateNow)
	var restarts []string
	h.updater = installableUpdater(t, fakeRelease(t, "v1.1.0", []byte("<html>to nie program")), &restarts)
	postUpdate(t, h, "/update/check", url.Values{})

	rec := postUpdate(t, h, "/update/install", url.Values{"version": {"1.1.0"}, "week": {"2026-11-23"}})

	if got, want := rec.Header().Get("Location"), "/settings?week=2026-11-23#aktualizacje"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	checkBody(t, getSettings(t, h, "/settings").Body.String(), `<p class="update-error" role="alert">Pobrany plik nie jest programem dla Windows.</p>`)
	if len(restarts) != 0 {
		t.Error("the program was restarted after a failed download")
	}
}

func TestInstallErrorMessage(t *testing.T) {
	if got := installErrorMessage(fmt.Errorf("wrapped: %w", update.ErrStart)); !strings.HasPrefix(got, "Windows nie pozwolił uruchomić nowej wersji") {
		t.Errorf("message for ErrStart = %q", got)
	}
	if got := installErrorMessage(os.ErrPermission); !strings.Contains(got, "program działa dalej w obecnej wersji") {
		t.Errorf("generic message = %q", got)
	}
}
