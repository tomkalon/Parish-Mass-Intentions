package web

import (
	"bytes"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/backup"
	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

func postForm(t *testing.T, handle http.HandlerFunc, target string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	handle(rec, req)

	return rec
}

func uploadImport(t *testing.T, h *handler, content []byte) *httptest.ResponseRecorder {
	t.Helper()

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	err := mw.WriteField("week", "2026-11-02")
	if err != nil {
		t.Fatal(err)
	}
	if content != nil {
		part, err := mw.CreateFormFile("file", "kopia.db")
		if err != nil {
			t.Fatal(err)
		}
		_, err = part.Write(content)
		if err != nil {
			t.Fatal(err)
		}
	}
	err = mw.Close()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	rec := httptest.NewRecorder()
	h.importData(rec, req)

	return rec
}

func exportFile(t *testing.T, parish string, list ...intentions.Intention) []byte {
	t.Helper()

	path := filepath.Join(t.TempDir(), "kopia.db")
	db, err := database.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := range list {
		err = intentions.Save(db, &list[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	err = settings.SaveParish(db, settings.Setting{ParishName: parish})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.Close()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	return content
}

func contents(t *testing.T, h *handler) []string {
	t.Helper()

	var list []intentions.Intention
	err := h.db.Order("mass_at, id").Find(&list).Error
	if err != nil {
		t.Fatalf("loading intentions: %v", err)
	}

	var result []string
	for _, intention := range list {
		result = append(result, intention.Content)
	}

	return result
}

func TestSettingsShowsBackupSection(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	body := getSettings(t, h, "/settings?week=2026-11-02").Body.String()

	for _, want := range []string{
		`<h2 class="settings-heading" id="backup-title">Kopia danych</h2>`,
		`<form method="post" action="/settings/export">`,
		`<form method="post" action="/settings/import" enctype="multipart/form-data">`,
		`<input class="visually-hidden" type="file" name="file" accept=".db,.bak" required data-autosubmit>`,
		`<code class="path">` + h.dataDir + `</code>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page does not contain %q", want)
		}
	}
}

func TestExportDownloadWithoutDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")

	rec := postForm(t, h.exportData, "/settings/export", url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `attachment; filename="intencje-kopia-2026-10-29.db"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if !strings.HasPrefix(rec.Body.String(), "SQLite format 3\x00") {
		t.Error("body is not an SQLite database")
	}

	path := filepath.Join(t.TempDir(), "pobrany.db")
	err := os.WriteFile(path, rec.Body.Bytes(), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	summary, err := database.PrepareImport(path)
	if err != nil || summary.Intentions != 1 {
		t.Errorf("downloaded file: %+v, %v; want 1 intention", summary, err)
	}

	left, _ := os.ReadDir(h.dataDir)
	if len(left) != 0 {
		t.Errorf("files left in the data folder: %v", left)
	}
}

func TestExportWithSaveDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")
	target := filepath.Join(t.TempDir(), "pendrive", "kopia.db")
	err := os.MkdirAll(filepath.Dir(target), 0o755)
	if err != nil {
		t.Fatal(err)
	}

	var suggested string
	h.saveFile = func(name string) (string, error) {
		suggested = name
		return target, nil
	}

	rec := postForm(t, h.exportData, "/settings/export", url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if suggested != "intencje-kopia-2026-10-29.db" {
		t.Errorf("suggested name = %q, want intencje-kopia-2026-10-29.db", suggested)
	}
	location := rec.Header().Get("Location")
	want := "/settings?" + (url.Values{"exported": {target}, "week": {"2026-11-02"}}).Encode()
	if location != want {
		t.Errorf("Location = %q, want %q", location, want)
	}
	if summary, err := database.PrepareImport(target); err != nil || summary.Intentions != 1 {
		t.Errorf("exported file: %+v, %v; want 1 intention", summary, err)
	}

	body := getSettings(t, h, location).Body.String()
	if want := `<p class="notice" role="status">Zapisano kopię danych: ` + target + `</p>`; !strings.Contains(body, want) {
		t.Errorf("settings page does not contain %q", want)
	}
}

func TestExportDialogCancelled(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	h.saveFile = func(string) (string, error) { return "", nil }

	rec := postForm(t, h.exportData, "/settings/export", url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/settings?week=2026-11-02"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestExportDialogError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	h.saveFile = func(string) (string, error) { return "", errors.New("window is not ready") }

	rec := postForm(t, h.exportData, "/settings/export", url.Values{})

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestImport(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "stara intencja")
	file := exportFile(t, "Parafia św. Jana",
		newIntention(t, 0, "2026-11-03 18:00", false, "nowa 1"),
		newIntention(t, 0, "2026-11-20 07:00", true, "nowa 2"),
	)

	rec := uploadImport(t, h, file)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("upload: status = %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	location := rec.Header().Get("Location")
	if want := "/settings?import=1&week=2026-11-02"; location != want {
		t.Fatalf("upload: Location = %q, want %q", location, want)
	}
	if got := contents(t, h); len(got) != 1 || got[0] != "stara intencja" {
		t.Fatalf("data changed before confirmation: %q", got)
	}

	dialog := getSettings(t, h, location).Body.String()
	for _, want := range []string{
		`<h2 class="dialog-title" id="dialog-title">Zamienić wszystkie dane?</h2>`,
		`W pliku: Parafia św. Jana, 2 intencje (3.11.2026 – 20.11.2026).`,
		`<form class="dialog-actions" method="post" action="/settings/import/confirm">`,
		`<button class="button button-danger" type="submit">Zamień dane</button>`,
		`<a class="button" href="/settings?week=2026-11-02" data-close autofocus>Anuluj</a>`,
	} {
		if !strings.Contains(dialog, want) {
			t.Errorf("confirmation dialog does not contain %q", want)
		}
	}

	rec = postForm(t, h.confirmImport, "/settings/import/confirm", url.Values{"week": {"2026-11-02"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm: status = %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body)
	}
	backup := "data-2026-10-29-100000.db.bak"
	location = rec.Header().Get("Location")
	if want := "/settings?imported=" + backup + "&week=2026-11-02"; location != want {
		t.Errorf("confirm: Location = %q, want %q", location, want)
	}

	if got := contents(t, h); strings.Join(got, ",") != "nowa 1,nowa 2" {
		t.Errorf("intentions after import = %q, want nowa 1, nowa 2", got)
	}
	setting, err := settings.Load(h.db)
	if err != nil || setting.ParishName != "Parafia św. Jana" {
		t.Errorf("parish after import = %q, %v; want Parafia św. Jana", setting.ParishName, err)
	}

	summary, err := database.PrepareImport(filepath.Join(h.dataDir, backup))
	if err != nil || summary.Intentions != 1 {
		t.Errorf("backup %s: %+v, %v; want the 1 old intention", backup, summary, err)
	}
	if _, err := os.Stat(h.importPath()); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("import file left behind: %v", err)
	}

	notice := getSettings(t, h, location).Body.String()
	if want := "Dane zaimportowane. Poprzednie dane zapisano w pliku " + backup; !strings.Contains(notice, want) {
		t.Errorf("settings page does not contain %q", want)
	}
}

func TestImportRejectsInvalidFile(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "stara intencja")

	tests := []struct {
		name    string
		content []byte
		status  int
		message string
	}{
		{name: "text file", content: []byte(strings.Repeat("to nie jest baza danych ", 10)), status: http.StatusUnprocessableEntity, message: invalidImportMessage},
		{name: "no file", content: nil, status: http.StatusUnprocessableEntity, message: missingImportMessage},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := uploadImport(t, h, tt.content)

			if rec.Code != tt.status {
				t.Fatalf("status = %d, want %d", rec.Code, tt.status)
			}
			if want := "<p>Nie zaimportowano. " + tt.message + "</p>"; !strings.Contains(rec.Body.String(), want) {
				t.Errorf("body does not contain %q", want)
			}
			if _, err := os.Stat(h.importPath()); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("rejected file kept: %v", err)
			}
			if got := contents(t, h); len(got) != 1 || got[0] != "stara intencja" {
				t.Errorf("data changed: %q", got)
			}
		})
	}
}

func TestImportWithoutUploadedFile(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "stara intencja")

	page := getSettings(t, h, "/settings?import=1").Body.String()
	if strings.Contains(page, "Zamienić wszystkie dane?") {
		t.Error("confirmation dialog shown without an uploaded file")
	}
	if want := "<p>Nie zaimportowano. " + lostImportMessage + "</p>"; !strings.Contains(page, want) {
		t.Errorf("settings page does not contain %q", want)
	}

	rec := postForm(t, h.confirmImport, "/settings/import/confirm", url.Values{"week": {"2026-11-02"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("confirm: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if got := contents(t, h); len(got) != 1 || got[0] != "stara intencja" {
		t.Errorf("data changed: %q", got)
	}
	if left, _ := os.ReadDir(h.dataDir); len(left) != 0 {
		t.Errorf("files created in the data folder: %v", left)
	}
}

func TestIntentionsLabel(t *testing.T) {
	tests := map[int64]string{
		1:   "1 intencja",
		2:   "2 intencje",
		4:   "4 intencje",
		5:   "5 intencji",
		12:  "12 intencji",
		22:  "22 intencje",
		112: "112 intencji",
	}

	for n, want := range tests {
		if got := intentionsLabel(n); got != want {
			t.Errorf("intentionsLabel(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBackupRoutes(t *testing.T) {
	db := newTestHandler(t, time.Now()).db
	dir := t.TempDir()
	exported := filepath.Join(t.TempDir(), "kopia.db")

	router := chi.NewRouter()
	RegisterRoutes(router, db, Options{
		DataDir:  dir,
		SaveFile: func(string) (string, error) { return exported, nil },
	})
	serve := func(req *http.Request) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec
	}
	form := func(target string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, target, strings.NewReader("week=2026-11-02"))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		return req
	}

	if rec := serve(form("/settings/export")); rec.Code != http.StatusSeeOther {
		t.Fatalf("export: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	content, err := os.ReadFile(exported)
	if err != nil {
		t.Fatalf("export did not use the save dialog path: %v", err)
	}

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("file", "kopia.db")
	if err != nil {
		t.Fatal(err)
	}
	_, err = part.Write(content)
	if err != nil {
		t.Fatal(err)
	}
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/settings/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	if rec := serve(req); rec.Code != http.StatusSeeOther {
		t.Fatalf("import: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if _, err := os.Stat(filepath.Join(dir, importFileName)); err != nil {
		t.Fatalf("uploaded file not stored in the data folder: %v", err)
	}

	if rec := serve(form("/settings/import/confirm")); rec.Code != http.StatusSeeOther {
		t.Fatalf("confirm: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "data-*.db.bak"))
	if len(backups) != 1 {
		t.Errorf("backups in the data folder = %v, want 1", backups)
	}
}

func TestBackupLabel(t *testing.T) {
	now := time.Date(2026, 11, 2, 23, 0, 0, 0, intentions.TimeZone)
	at := func(month time.Month, day, hour, minute int, year int) time.Time {
		return time.Date(year, month, day, hour, minute, 0, 0, intentions.TimeZone)
	}

	tests := []struct {
		name  string
		state backup.State
		want  string
	}{
		{name: "no folder", state: backup.State{}, want: "folder kopii nie jest wybrany"},
		{name: "never backed up", state: backup.State{Dir: "/kopie"}, want: "brak kopii"},
		{name: "changed since", state: backup.State{Dir: "/kopie", LastBackup: at(11, 2, 9, 0, 2026), LastChange: at(11, 2, 10, 0, 2026)}, want: "zmiany bez kopii"},
		{name: "today", state: backup.State{Dir: "/kopie", LastBackup: at(11, 2, 22, 50, 2026)}, want: "kopia: dziś, 22:50"},
		{name: "this year", state: backup.State{Dir: "/kopie", LastBackup: at(10, 30, 8, 5, 2026)}, want: "kopia: 30.10, 8:05"},
		{name: "last year", state: backup.State{Dir: "/kopie", LastBackup: at(12, 31, 18, 0, 2025)}, want: "kopia: 31.12.2025, 18:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := backupLabel(tt.state, now); got != tt.want {
				t.Errorf("backupLabel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTopbarShowsBackupButton(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	for _, body := range []string{
		getWeek(t, h, "/?week=2026-11-02").Body.String(),
		getSettings(t, h, "/settings").Body.String(),
	} {
		for _, want := range []string{
			`<form class="topbar-backup" method="post" action="/backup">`,
			`<span class="backup-status">folder kopii nie jest wybrany</span>`,
			`<button class="button-small" type="submit">Wykonaj kopię danych</button>`,
		} {
			if !strings.Contains(body, want) {
				t.Errorf("page does not contain %q", want)
			}
		}
	}

	err := h.backups.SetDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if want := `<span class="backup-status is-pending">brak kopii</span>`; !strings.Contains(getWeek(t, h, "/?week=2026-11-02").Body.String(), want) {
		t.Errorf("week page does not contain %q", want)
	}
}

func postBackup(t *testing.T, h *handler, referer string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/backup", nil)
	req.Header.Set("Referer", referer)
	rec := httptest.NewRecorder()
	h.runBackup(rec, req)

	return rec
}

func TestRunBackupWithoutFolder(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postBackup(t, h, "http://example.com/?week=2026-11-04")

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if want := "/settings?backup_error=nodir&week=2026-11-02"; location != want {
		t.Fatalf("Location = %q, want %q", location, want)
	}
	if want := "<p>Kopia danych: Najpierw wybierz folder kopii w sekcji „Kopia danych”.</p>"; !strings.Contains(getSettings(t, h, location).Body.String(), want) {
		t.Errorf("settings page does not contain %q", want)
	}
}

func TestRunBackup(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedIntention(t, h.db, "2026-11-02 18:00", false, "+ Jan Kowalski")
	folder := t.TempDir()
	err := h.backups.SetDir(folder)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		referer string
		want    string
	}{
		{referer: "http://example.com/?week=2026-11-09&edit=3", want: "/?week=2026-11-09&edit=3"},
		{referer: "http://example.com/settings/schedules/2?week=2026-11-09", want: "/settings/schedules/2?week=2026-11-09"},
		{referer: "http://inna-strona.pl/oszustwo", want: "/"},
		{referer: "", want: "/"},
	}

	for _, tt := range tests {
		t.Run(tt.referer, func(t *testing.T) {
			rec := postBackup(t, h, tt.referer)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := rec.Header().Get("Location"); got != tt.want {
				t.Errorf("Location = %q, want %q", got, tt.want)
			}
		})
	}

	summary, err := database.PrepareImport(filepath.Join(folder, backup.FileName))
	if err != nil || summary.Intentions != 1 {
		t.Errorf("backup file: %+v, %v; want 1 intention", summary, err)
	}
	if h.backups.Pending() {
		t.Error("still pending after the backup")
	}
}

func TestRunBackupFolderGone(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	folder := filepath.Join(t.TempDir(), "Google Drive")
	err := os.Mkdir(folder, 0o755)
	if err != nil {
		t.Fatal(err)
	}
	err = h.backups.SetDir(folder)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Remove(folder)
	if err != nil {
		t.Fatal(err)
	}

	rec := postBackup(t, h, "http://example.com/settings")

	location := rec.Header().Get("Location")
	if want := "/settings?backup_error=failed"; location != want {
		t.Fatalf("Location = %q, want %q", location, want)
	}
	if want := "Nie udało się zapisać kopii w folderze " + folder + "."; !strings.Contains(getSettings(t, h, location).Body.String(), want) {
		t.Errorf("settings page does not contain %q", want)
	}
}

func TestSetBackupDirByTyping(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	folder := t.TempDir()

	page := getSettings(t, h, "/settings").Body.String()
	if want := `<input class="input" type="text" id="backup-dir" name="dir" value=""`; !strings.Contains(page, want) {
		t.Errorf("settings page without a dialog does not contain %q", want)
	}

	rec := postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{"week": {"2026-11-02"}, "dir": {"  " + folder + " "}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	if want := "/settings?backup_dir=set&week=2026-11-02"; location != want {
		t.Errorf("Location = %q, want %q", location, want)
	}
	if got := h.backups.State().Dir; got != folder {
		t.Errorf("dir = %q, want %q", got, folder)
	}
	page = getSettings(t, h, location).Body.String()
	for _, want := range []string{
		`<p class="notice" role="status">Zapisano folder kopii.</p>`,
		`Folder: <code class="path">` + folder + `</code> · brak kopii`,
		`<button class="link-button" type="submit">Wyłącz</button>`,
	} {
		if !strings.Contains(page, want) {
			t.Errorf("settings page does not contain %q", want)
		}
	}

	missing := filepath.Join(folder, "brak")
	for _, tt := range []struct {
		dir     string
		message string
	}{
		{dir: missing, message: "Nie ma takiego folderu: " + missing},
		{dir: "", message: "Wpisz ścieżkę folderu."},
	} {
		rec = postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{"dir": {tt.dir}})
		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("dir %q: status = %d, want %d", tt.dir, rec.Code, http.StatusUnprocessableEntity)
		}
		body := rec.Body.String()
		if want := "<p>Kopia danych: " + tt.message + "</p>"; !strings.Contains(body, want) {
			t.Errorf("dir %q: body does not contain %q", tt.dir, want)
		}
		if want := `name="dir" value="` + tt.dir + `"`; !strings.Contains(body, want) {
			t.Errorf("dir %q: typed value not kept, want %q", tt.dir, want)
		}
	}
	if got := h.backups.State().Dir; got != folder {
		t.Errorf("dir = %q after rejected input, want %q", got, folder)
	}

	rec = postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{"off": {"1"}})
	if got, want := rec.Header().Get("Location"), "/settings?backup_dir=off"; got != want {
		t.Errorf("turning off: Location = %q, want %q", got, want)
	}
	if got := h.backups.State().Dir; got != "" {
		t.Errorf("dir = %q after turning off, want empty", got)
	}
}

func TestSetBackupDirWithDialog(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	first, second := t.TempDir(), t.TempDir()
	err := h.backups.SetDir(first)
	if err != nil {
		t.Fatal(err)
	}

	var current string
	answer := second
	h.chooseDir = func(dir string) (string, error) {
		current = dir
		return answer, nil
	}

	page := getSettings(t, h, "/settings").Body.String()
	if !strings.Contains(page, `<button class="button" type="submit">Wybierz folder…</button>`) || strings.Contains(page, `name="dir"`) {
		t.Error("settings page with a dialog should show „Wybierz folder…” instead of a text field")
	}

	rec := postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{"week": {"2026-11-02"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if current != first {
		t.Errorf("dialog opened at %q, want the current folder %q", current, first)
	}
	if got := h.backups.State().Dir; got != second {
		t.Errorf("dir = %q, want %q", got, second)
	}

	answer = ""
	rec = postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{"week": {"2026-11-02"}})
	if got, want := rec.Header().Get("Location"), "/settings?week=2026-11-02"; got != want {
		t.Errorf("cancelled: Location = %q, want %q", got, want)
	}
	if got := h.backups.State().Dir; got != second {
		t.Errorf("dir = %q after cancelling, want %q", got, second)
	}

	h.chooseDir = func(string) (string, error) { return "", errors.New("window is not ready") }
	if rec := postForm(t, h.setBackupDir, "/settings/backup/dir", url.Values{}); rec.Code != http.StatusInternalServerError {
		t.Errorf("dialog error: status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestRoutesTrackChanges(t *testing.T) {
	db := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)).db
	backups, err := backup.Load(db, "")
	if err != nil {
		t.Fatal(err)
	}
	err = backups.SetDir(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterRoutes(router, db, Options{DataDir: t.TempDir(), Backups: backups})
	serve := func(method, target string, form url.Values) int {
		var body *strings.Reader
		if form != nil {
			body = strings.NewReader(form.Encode())
		} else {
			body = strings.NewReader("")
		}
		req := httptest.NewRequest(method, target, body)
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		return rec.Code
	}

	if serve(http.MethodPost, "/backup", nil) != http.StatusSeeOther || backups.Pending() {
		t.Fatal("first backup failed")
	}

	for _, request := range []struct{ method, target string }{
		{http.MethodGet, "/?week=2026-11-02"},
		{http.MethodGet, "/settings"},
		{http.MethodGet, "/pdf?week=2026-11-02"},
		{http.MethodPost, "/settings/export"},
		{http.MethodPost, "/backup"},
	} {
		serve(request.method, request.target, url.Values{})
		if backups.Pending() {
			t.Errorf("%s %s marked the data as changed", request.method, request.target)
		}
	}

	if code := serve(http.MethodPost, "/intentions", url.Values{"week": {"2026-11-02"}, "day": {"2026-11-03"}, "time": {"18:00"}, "content": {""}}); code != http.StatusUnprocessableEntity {
		t.Fatalf("invalid intention: status = %d, want %d", code, http.StatusUnprocessableEntity)
	}
	if backups.Pending() {
		t.Error("a rejected form marked the data as changed")
	}

	if code := serve(http.MethodPost, "/intentions", url.Values{"week": {"2026-11-02"}, "day": {"2026-11-03"}, "time": {"18:00"}, "content": {"+ Jan Kowalski"}}); code != http.StatusSeeOther {
		t.Fatalf("saving intention: status = %d, want %d", code, http.StatusSeeOther)
	}
	if !backups.Pending() {
		t.Error("saving an intention did not mark the data as changed")
	}
}
