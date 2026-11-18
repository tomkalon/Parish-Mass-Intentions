package web

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"go-parafia/internal/database"
	"go-parafia/internal/intentions"
	"go-parafia/internal/update"
)

const updateCheckTimeout = 20 * time.Second

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
	Installing string
}

func newUpdateView(u *update.Updater, now time.Time) updateView {
	status := u.Status()
	view := updateView{
		Current:    status.Current,
		Error:      status.Err,
		Available:  status.Available(),
		CanInstall: u.CanInstall(),
	}
	if progress := u.Progress(); progress.Active() {
		view.Installing = progress.Version
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

func progressURL(week string) string {
	if week == "" {
		return "/update"
	}

	return "/update?week=" + url.QueryEscape(normalizeWeek(week))
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
	week := r.PostForm.Get("week")
	version := r.PostForm.Get("version")

	err = h.updater.Start(version, h.backupBeforeUpdate)
	if err != nil {
		log.Printf("error starting update %s: %v", version, err)
		h.updater.SetError(update.Message(err))
		http.Redirect(w, r, updatesURL(week), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, progressURL(week), http.StatusSeeOther)
}

func (h *handler) backupBeforeUpdate() error {
	name := "data-" + h.now().In(intentions.TimeZone).Format("2006-01-02-150405") + "-przed-aktualizacja.db.bak"
	err := database.Export(h.db, filepath.Join(h.dataDir, name))
	if err != nil {
		log.Printf("error backing up data before the update: %v", err)
		return err
	}

	err = h.backups.RunIfPending()
	if err != nil {
		log.Printf("error making backup before the update: %v", err)
	}

	return nil
}

var updateSteps = []struct {
	stage string
	label string
}{
	{update.StageBackup, "Kopia danych"},
	{update.StageDownload, "Pobieranie"},
	{update.StageVerify, "Sprawdzanie pliku"},
	{update.StageInstall, "Instalacja"},
	{update.StageRestart, "Ponowne uruchomienie"},
}

type progressView struct {
	Version string `json:"version"`
	Stage   string `json:"stage"`
	Label   string `json:"label"`
	Percent int    `json:"percent"`
	Error   string `json:"error"`
	Failed  bool   `json:"failed"`
	Steps   []step `json:"steps"`

	Week    string `json:"-"`
	BackURL string `json:"-"`
}

type step struct {
	Stage string `json:"stage"`
	Label string `json:"label"`
	State string `json:"state"`
}

func newProgressView(p update.Progress) progressView {
	view := progressView{
		Version: p.Version,
		Stage:   p.Stage,
		Error:   p.Err,
		Failed:  p.Stage == update.StageError,
		Label:   progressLabel(p),
		Percent: progressPercent(p),
	}

	current := -1
	for i, s := range updateSteps {
		if s.stage == p.Stage {
			current = i
		}
	}
	for i, s := range updateSteps {
		state := "todo"
		switch {
		case i < current:
			state = "done"
		case i == current:
			state = "current"
		}
		view.Steps = append(view.Steps, step{Stage: s.stage, Label: s.label, State: state})
	}

	return view
}

func progressLabel(p update.Progress) string {
	switch p.Stage {
	case update.StageBackup:
		return "Zapisuję kopię danych…"
	case update.StageDownload:
		if p.Total > 0 {
			return fmt.Sprintf("Pobieram nową wersję… %s z %s", megabytes(p.Done), megabytes(p.Total))
		}
		return "Pobieram nową wersję… " + megabytes(p.Done)
	case update.StageVerify:
		return "Sprawdzam pobrany plik…"
	case update.StageInstall:
		return "Instaluję nową wersję…"
	case update.StageRestart:
		return "Gotowe. Program za chwilę uruchomi się ponownie."
	case update.StageError:
		return "Aktualizacja nie powiodła się."
	default:
		return ""
	}
}

func progressPercent(p update.Progress) int {
	switch p.Stage {
	case update.StageDownload:
		if p.Total > 0 {
			return int(min(p.Done*100/p.Total, 100))
		}
		return 0
	case update.StageVerify, update.StageInstall, update.StageRestart:
		return 100
	default:
		return 0
	}
}

func megabytes(n int64) string {
	return strings.Replace(fmt.Sprintf("%.1f MB", float64(n)/(1<<20)), ".", ",", 1)
}

func (h *handler) updateProgress(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week")
	progress := h.updater.Progress()
	if progress.Stage == update.StageIdle {
		http.Redirect(w, r, updatesURL(week), http.StatusSeeOther)
		return
	}

	view := newProgressView(progress)
	view.Week = normalizeWeek(week)
	view.BackURL = updatesURL(week)

	render(w, http.StatusOK, updatingTemplate, view)
}

func (h *handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")

	err := json.NewEncoder(w).Encode(newProgressView(h.updater.Progress()))
	if err != nil {
		log.Printf("error writing update status: %v", err)
	}
}
