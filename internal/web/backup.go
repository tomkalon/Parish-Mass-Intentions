package web

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"go-parafia/internal/backup"
	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

const (
	importFileName = "import.db"
	maxImportSize  = 100 << 20

	invalidImportMessage = "Ten plik nie zawiera danych Go Parafia. Wybierz plik z eksportu (.db) albo kopię sprzed importu (.bak)."
	missingImportMessage = "Wybierz plik do importu."
	lostImportMessage    = "Nie znaleziono wczytanego pliku. Wybierz go jeszcze raz."
)

func (h *handler) importPath() string {
	return filepath.Join(h.dataDir, importFileName)
}

func (h *handler) exportData(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := r.PostForm.Get("week")
	name := "intencje-kopia-" + h.now().In(intentions.TimeZone).Format(intentions.DateLayout) + ".db"

	if h.saveFile == nil {
		h.downloadExport(w, name)
		return
	}

	path, err := h.saveFile(name)
	if err != nil {
		log.Printf("error choosing export file: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	if path == "" {
		http.Redirect(w, r, settingsURL(week, false), http.StatusSeeOther)
		return
	}

	err = database.Export(h.db, path)
	if err != nil {
		log.Printf("error exporting data to %s: %v", path, err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, settingsNoticeURL(week, "exported", path), http.StatusSeeOther)
}

func (h *handler) downloadExport(w http.ResponseWriter, name string) {
	tmp, err := os.CreateTemp(h.dataDir, "export-*.db")
	if err != nil {
		log.Printf("error creating export file: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer removeFile(tmp.Name())

	err = tmp.Close()
	if err != nil {
		log.Printf("error creating export file: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	err = database.Export(h.db, tmp.Name())
	if err != nil {
		log.Printf("error exporting data: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	file, err := os.Open(tmp.Name())
	if err != nil {
		log.Printf("error opening export file: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = file.Close() }()

	w.Header().Set("Content-Type", "application/vnd.sqlite3")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)

	_, err = io.Copy(w, file)
	if err != nil {
		log.Printf("error writing export: %v", err)
	}
}

func (h *handler) importData(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportSize)
	err := r.ParseMultipartForm(8 << 20)
	if err != nil {
		log.Printf("error reading import upload: %v", err)
		h.renderImportError(w, http.StatusBadRequest, "", invalidImportMessage)
		return
	}
	week := r.PostForm.Get("week")

	upload, _, err := r.FormFile("file")
	if err != nil {
		h.renderImportError(w, http.StatusUnprocessableEntity, week, missingImportMessage)
		return
	}
	defer func() { _ = upload.Close() }()

	path := h.importPath()
	err = saveUpload(upload, path)
	if err != nil {
		log.Printf("error saving import file: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	_, err = database.PrepareImport(path)
	if err != nil {
		removeFile(path)
		if errors.Is(err, database.ErrInvalidFile) {
			log.Printf("rejected import file: %v", err)
			h.renderImportError(w, http.StatusUnprocessableEntity, week, invalidImportMessage)
			return
		}
		log.Printf("error preparing import: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, settingsNoticeURL(week, "import", "1"), http.StatusSeeOther)
}

func removeFile(path string) {
	err := os.Remove(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("error removing %s: %v", path, err)
	}
}

func saveUpload(upload multipart.File, path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}

	_, err = io.Copy(file, upload)
	return errors.Join(err, file.Close())
}

func (h *handler) confirmImport(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := r.PostForm.Get("week")

	backupName := "data-" + h.now().In(intentions.TimeZone).Format("2006-01-02-150405") + ".db.bak"
	err = database.Restore(h.db, h.importPath(), filepath.Join(h.dataDir, backupName))
	if errors.Is(err, database.ErrInvalidFile) {
		log.Printf("import file not usable: %v", err)
		h.renderImportError(w, http.StatusBadRequest, week, lostImportMessage)
		return
	}
	if err != nil {
		log.Printf("error importing data: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	removeFile(h.importPath())

	http.Redirect(w, r, settingsNoticeURL(week, "imported", backupName), http.StatusSeeOther)
}

func (h *handler) importDialog(view *settingsView, closeURL string) {
	summary, err := database.PrepareImport(h.importPath())
	if err != nil {
		log.Printf("import file not usable: %v", err)
		view.ImportError = lostImportMessage
		return
	}

	view.Dialog = &settingsDialog{
		Title:    "Zamienić wszystkie dane?",
		Text:     importSummaryText(summary),
		Action:   "/settings/import/confirm",
		Button:   "Zamień dane",
		Week:     view.Week,
		CloseURL: closeURL,
	}
}

func importSummaryText(s database.Summary) string {
	contents := "brak intencji"
	if s.Intentions > 0 {
		contents = fmt.Sprintf("%s (%s – %s)", intentionsLabel(s.Intentions), s.First.Format("2.1.2006"), s.Last.Format("2.1.2006"))
	}

	return fmt.Sprintf("W pliku: %s, %s. Obecne intencje, święta, ustawienia i porządki mszy zostaną zastąpione danymi z pliku. Przed zamianą program zapisze kopię obecnych danych (plik .bak w folderze danych).", s.ParishName, contents)
}

func intentionsLabel(n int64) string {
	switch {
	case n == 1:
		return "1 intencja"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d intencje", n)
	default:
		return fmt.Sprintf("%d intencji", n)
	}
}

func settingsNoticeURL(week, key, value string) string {
	query := url.Values{}
	if week != "" {
		query.Set("week", week)
	}
	query.Set(key, value)

	return "/settings?" + query.Encode()
}

func (h *handler) renderImportError(w http.ResponseWriter, status int, week, message string) {
	h.renderSettingsWith(w, status, week, func(view *settingsView) {
		view.ImportError = message
	})
}

func (h *handler) renderSettingsWith(w http.ResponseWriter, status int, week string, edit func(view *settingsView)) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}

	view, ok := h.settingsView(w, setting, week)
	if !ok {
		return
	}
	edit(&view)

	render(w, status, settingsTemplate, view)
}

func (h *handler) runBackup(w http.ResponseWriter, r *http.Request) {
	back, week := refererPage(r)

	_, err := h.backups.Run()
	switch {
	case errors.Is(err, backup.ErrNoDir):
		http.Redirect(w, r, settingsNoticeURL(week, "backup_error", "nodir"), http.StatusSeeOther)
	case err != nil:
		log.Printf("error making backup: %v", err)
		http.Redirect(w, r, settingsNoticeURL(week, "backup_error", "failed"), http.StatusSeeOther)
	default:
		http.Redirect(w, r, back, http.StatusSeeOther)
	}
}

func refererPage(r *http.Request) (string, string) {
	ref, err := url.Parse(r.Referer())
	if err != nil || !strings.HasPrefix(ref.Path, "/") || (ref.Host != "" && ref.Host != r.Host) {
		return "/", ""
	}

	page := ref.Path
	if ref.RawQuery != "" {
		page += "?" + ref.RawQuery
	}

	return page, normalizeWeek(ref.Query().Get("week"))
}

func (h *handler) setBackupDir(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := r.PostForm.Get("week")
	dir := strings.TrimSpace(r.PostForm.Get("dir"))

	switch {
	case r.PostForm.Get("off") == "1":
		dir = ""
	case h.chooseDir != nil:
		dir, err = h.chooseDir(h.backups.State().Dir)
		if err != nil {
			log.Printf("error choosing backup folder: %v", err)
			http.Error(w, "internal server error", http.StatusInternalServerError)
			return
		}
		if dir == "" {
			http.Redirect(w, r, settingsURL(week, false), http.StatusSeeOther)
			return
		}
	case dir == "":
		h.renderBackupError(w, week, "", "Wpisz ścieżkę folderu.")
		return
	}

	err = h.backups.SetDir(dir)
	if errors.Is(err, backup.ErrNotFolder) {
		h.renderBackupError(w, week, dir, "Nie ma takiego folderu: "+dir)
		return
	}
	if err != nil {
		log.Printf("error saving backup folder: %v", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	state := "set"
	if dir == "" {
		state = "off"
	}
	http.Redirect(w, r, settingsNoticeURL(week, "backup_dir", state), http.StatusSeeOther)
}

func (h *handler) renderBackupError(w http.ResponseWriter, week, typed, message string) {
	h.renderSettingsWith(w, http.StatusUnprocessableEntity, week, func(view *settingsView) {
		view.BackupError = message
		view.BackupDir = typed
	})
}

func backupErrorMessage(code, dir string) string {
	switch code {
	case "nodir":
		return "Najpierw wybierz folder kopii w sekcji „Kopia danych”."
	case "failed":
		return fmt.Sprintf("Nie udało się zapisać kopii w folderze %s. Sprawdź, czy folder jest dostępny (np. czy dysk w chmurze jest podłączony).", dir)
	default:
		return ""
	}
}

func backupDirNotice(state string) string {
	switch state {
	case "set":
		return "Zapisano folder kopii."
	case "off":
		return "Wyłączono automatyczną kopię."
	default:
		return ""
	}
}

func importedNotice(backupName, dataDir string) string {
	return fmt.Sprintf("Dane zaimportowane. Poprzednie dane zapisano w pliku %s (folder: %s).", backupName, dataDir)
}

func exportedNotice(path string) string {
	return "Zapisano kopię danych: " + path
}
