package web

import (
	"context"
	"errors"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"time"
	"unicode"
	"unicode/utf8"

	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
	"go-parafia/internal/update"
)

const (
	updateCheckTimeout   = 20 * time.Second
	updateInstallTimeout = 5 * time.Minute
)

var updatingTemplate = template.Must(template.ParseFS(templateFS, "templates/updating.html"))

type updateView struct {
	Current    string
	Latest     string
	Published  string
	Notes      string
	Checked    string
	Error      string
	Available  bool
	UpToDate   bool
	CanInstall bool
}

func newUpdateView(u *update.Updater, now time.Time) updateView {
	status := u.Status()
	view := updateView{
		Current:    status.Current,
		Error:      status.Err,
		Available:  status.Available(),
		CanInstall: u.CanInstall(),
	}
	if !status.CheckedAt.IsZero() {
		view.Checked = backupTime(status.CheckedAt, now)
		view.UpToDate = status.Err == "" && !view.Available
	}
	if view.Available {
		view.Latest = status.Latest.Version
		view.Notes = status.Latest.Notes
		if !status.Latest.Published.IsZero() {
			view.Published = status.Latest.Published.In(intentions.TimeZone).Format("2.1.2006")
		}
	}

	return view
}

func (h *handler) availableUpdate() string {
	status := h.updater.Status()
	if !status.Available() {
		return ""
	}

	return status.Latest.Version
}

func updatesURL(week string) string {
	if week == "" {
		return "/settings#aktualizacje"
	}

	return "/settings?week=" + url.QueryEscape(normalizeWeek(week)) + "#aktualizacje"
}

func (h *handler) checkUpdate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), updateCheckTimeout)
	defer cancel()

	_, err = h.updater.Check(ctx)
	if err != nil {
		log.Printf("error checking for updates: %v", err)
	}

	http.Redirect(w, r, updatesURL(r.PostForm.Get("week")), http.StatusSeeOther)
}

func (h *handler) installUpdate(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	back := updatesURL(r.PostForm.Get("week"))
	version := r.PostForm.Get("version")

	if !h.updater.CanInstall() {
		h.updater.SetError(installErrorMessage(update.ErrCannotInstall))
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	backupName := "data-" + h.now().In(intentions.TimeZone).Format("2006-01-02-150405") + "-przed-aktualizacja.db.bak"
	err = database.Export(h.db, filepath.Join(h.dataDir, backupName))
	if err != nil {
		log.Printf("error backing up data before the update: %v", err)
		h.updater.SetError("Nie udało się zapisać kopii danych przed aktualizacją, więc aktualizacja została przerwana.")
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}
	err = h.backups.RunIfPending()
	if err != nil {
		log.Printf("error making backup before the update: %v", err)
	}

	ctx, cancel := context.WithTimeout(r.Context(), updateInstallTimeout)
	defer cancel()

	err = h.updater.Install(ctx, version)
	if err != nil {
		log.Printf("error installing update %s: %v", version, err)
		h.updater.SetError(installErrorMessage(err))
		http.Redirect(w, r, back, http.StatusSeeOther)
		return
	}

	render(w, http.StatusOK, updatingTemplate, struct{ Version string }{version})
}

func installErrorMessage(err error) string {
	for _, known := range []error{update.ErrStart, update.ErrChecksum, update.ErrNotProgram, update.ErrMissingAsset, update.ErrNotAvailable, update.ErrInProgress, update.ErrCannotInstall} {
		if errors.Is(err, known) {
			return capitalize(known.Error()) + "."
		}
	}

	return "Nie udało się zainstalować aktualizacji. Sprawdź połączenie z internetem i spróbuj ponownie – program działa dalej w obecnej wersji."
}

func capitalize(text string) string {
	r, size := utf8.DecodeRuneInString(text)

	return string(unicode.ToUpper(r)) + text[size:]
}
