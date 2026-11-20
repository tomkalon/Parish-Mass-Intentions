package web

import (
	"bytes"
	"cmp"
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"net/url"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/go-playground/validator/v10"
	"gorm.io/gorm"

	"go-parafia/internal/backup"
	"go-parafia/internal/calendar"
	"go-parafia/internal/draft"
	"go-parafia/internal/gcatholic"
	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
	"go-parafia/internal/trash"
	"go-parafia/internal/update"
)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var weekTemplate = template.Must(template.ParseFS(templateFS, "templates/week.html", "templates/panel.html", "templates/topbar.html", "templates/daystats.html", "templates/badges.html"))

var searchTemplate = template.Must(template.ParseFS(templateFS, "templates/search.html", "templates/topbar.html", "templates/daystats.html", "templates/badges.html"))

var printTemplate = template.Must(template.ParseFS(templateFS, "templates/print.html"))

var settingsTemplate = template.Must(template.ParseFS(templateFS, "templates/settings.html", "templates/topbar.html"))

var scheduleTemplate = template.Must(template.ParseFS(templateFS, "templates/schedule.html", "templates/topbar.html"))

var startTemplate = template.Must(template.ParseFS(templateFS, "templates/start.html", "templates/topbar.html"))

type handler struct {
	now               func() time.Time
	db                *gorm.DB
	validate          *validator.Validate
	slotsMu           sync.Mutex
	fetchCelebrations func(ctx context.Context, year int) ([]gcatholic.Celebration, error)
	dataDir           string
	saveFile          func(defaultName string) (string, error)
	backups           *backup.Backups
	chooseDir         func(current string) (string, error)
	drafts            *draft.Store
	dbWarning         string
	version           string
	updater           *update.Updater
	logFile           string
	searches          searchMemory
}

type Options struct {
	DataDir         string
	LogFile         string
	SaveFile        func(defaultName string) (string, error)
	Backups         *backup.Backups
	ChooseDir       func(current string) (string, error)
	DatabaseWarning string
	Version         string
	Updater         *update.Updater
}

func RegisterRoutes(router chi.Router, db *gorm.DB, opts Options) {
	client := &http.Client{Timeout: 20 * time.Second}
	h := &handler{
		now:      time.Now,
		db:       db,
		validate: intentions.NewValidator(),
		fetchCelebrations: func(ctx context.Context, year int) ([]gcatholic.Celebration, error) {
			return gcatholic.Fetch(ctx, client, gcatholic.BaseURL, year)
		},
		dataDir:   opts.DataDir,
		logFile:   opts.LogFile,
		saveFile:  opts.SaveFile,
		backups:   opts.Backups,
		chooseDir: opts.ChooseDir,
		drafts:    draft.NewStore(draftPath(opts.DataDir)),
		dbWarning: opts.DatabaseWarning,
		version:   opts.Version,
		updater:   opts.Updater,
	}
	if h.updater == nil {
		h.updater = update.New(opts.Version, "", nil)
	}
	if h.backups == nil {
		h.backups, _ = backup.Load(db, "")
	}

	router.Group(func(r chi.Router) {
		r.Use(h.backups.TrackChanges)

		r.Post("/intentions", h.create)
		r.Post("/intentions/{id}", h.update)
		r.Post("/intentions/{id}/delete", h.delete)
		r.Post("/slots", h.createSlots)
		r.Post("/clear", h.clearWeek)
		r.Post("/trash/{batch}/restore", h.restoreTrash)
		r.Post("/series/{id}/stop", h.stopSeries)
		r.Post("/days/{date}", h.saveFeast)
		r.Post("/feasts/import", h.importFeasts)
		r.Post("/settings", h.saveSettings)
		r.Post("/settings/schedules", h.saveSchedule)
		r.Post("/settings/schedules/restore", h.restoreSchedules)
		r.Post("/settings/schedules/{id}", h.saveSchedule)
		r.Post("/settings/schedules/{id}/delete", h.deleteSchedule)
		r.Post("/settings/import/confirm", h.confirmImport)
	})

	router.Post("/update/check", h.checkUpdate)
	router.Post("/update/install", h.installUpdate)
	router.Get("/update", h.updateProgress)
	router.Get("/update/status", h.updateStatus)
	router.Post("/draft", h.saveDraft)
	router.Post("/draft/discard", h.discardDraft)
	router.Get("/", h.home)
	router.Get("/search", h.search)
	router.Get("/settings", h.settingsPage)
	router.Get("/settings/schedules/new", h.scheduleForm)
	router.Get("/settings/schedules/{id}", h.scheduleForm)
	router.Post("/settings/export", h.exportData)
	router.Post("/settings/import", h.importData)
	router.Post("/settings/backup/dir", h.setBackupDir)
	router.Post("/settings/backup/days", h.setBackupDays)
	router.Post("/backup", h.runBackup)
	router.Get("/pdf", h.pdf)
	router.Get("/intentions/{id}/receipt", h.receiptPage)
	router.Get("/intentions/{id}/receipt.pdf", h.receiptPDF)
	router.Get("/print/year", h.yearPrintPage)
	router.Get("/pdf/year", h.yearPDF)
	router.Post("/print/year/save", h.saveYearPDF)
	router.Get("/register", h.registerPage)
	router.Get("/register.pdf", h.registerPDF)
	router.Get("/print", h.printPreview)
	router.Handle("/static/*", http.FileServerFS(staticFS))
}

func (h *handler) home(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" {
		h.week(w, r)
		return
	}

	h.start(w, r)
}

func (h *handler) start(w http.ResponseWriter, r *http.Request) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}

	render(w, http.StatusOK, startTemplate, startView{
		ParishName: setting.ParishName,
		Address:    setting.Address,
		WeekURL:    weekURL(intentions.NextWeekStart(h.now())),
		Draft:      h.draftNotice(),
		Warning:    h.dbWarning,
		Version:    h.version,
		Update:     h.availableUpdate(),
	})
}

func draftPath(dataDir string) string {
	if dataDir == "" {
		return ""
	}

	return filepath.Join(dataDir, draft.FileName)
}

func (h *handler) week(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	start, err := h.weekStart(query)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	var state weekState
	switch {
	case query.Get("draft") == "1":
		form, ok := h.restoredForm(w, start)
		if !ok {
			return
		}
		if form == nil {
			http.Redirect(w, r, weekURL(start), http.StatusSeeOther)
			return
		}
		state.form = form
	case query.Get("delete") != "":
		intention, ok := h.findIntention(w, query.Get("delete"))
		if !ok {
			return
		}

		d := newDeleteView(start, intention)
		state.delete = &d
	case query.Get("edit") != "":
		intention, ok := h.findIntention(w, query.Get("edit"))
		if !ok {
			return
		}

		at := intention.MassAt.In(intentions.TimeZone)
		day := cmp.Or(query.Get("day"), at.Format(intentions.DateLayout))
		massTime := cmp.Or(query.Get("time"), at.Format("15:04"))

		f := newFormView(start, day, massTime)
		f.EditID = intention.ID
		f.Content = intention.Content
		f.IsOutsideParish = intention.IsOutsideParish
		f.IsGregorian = intention.IsGregorian
		f.IsDeceased = intention.IsDeceased
		f.IsHidden = intention.IsHidden
		f.Amount = intention.Amount.FormValue()
		f.SeriesID = intention.SeriesID
		f.AcceptedOn = intention.AcceptedOn
		f.placeholder = intention.IsPlaceholder
		if intention.IsPlaceholder {
			f.AcceptedOn = h.today()
		}
		state.form = &f
	case query.Get("new") == "1":
		f := newFormView(start, query.Get("day"), query.Get("time"))
		if id, err := strconv.Atoi(query.Get("series")); err == nil && id > 0 {
			f.SeriesID = id
		}
		if query.Get("from") == fromSearch {
			h.openedFromSearch(&f)
		}
		state.form = &f
	case query.Get("clear") == "1":
		state.clear = true
	case query.Get("feast") != "":
		day, err := time.ParseInLocation(intentions.DateLayout, query.Get("feast"), intentions.TimeZone)
		if err != nil {
			http.Error(w, "nieprawidłowa data dnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
			return
		}

		f := newFeastView(day)
		f.Feast, err = calendar.Feast(h.db, day.Format(intentions.DateLayout))
		if err != nil {
			log.Printf("error loading feast for %s: %v", day.Format(intentions.DateLayout), err)
			serverError(w)
			return
		}
		state.feast = &f
	case query.Get("calendar") != "":
		year, err := strconv.Atoi(query.Get("calendar"))
		if err != nil || year < 1000 || year > 9999 {
			http.Error(w, "nieprawidłowy rok kalendarza, oczekiwano RRRR", http.StatusBadRequest)
			return
		}

		state.calendarYear = year
	case query.Get("payments") != "":
		payments, err := parsePaymentsQuery(query, start)
		if err != nil {
			http.Error(w, "nieprawidłowy okres wpłat, oczekiwano payments=week|month|year, month=RRRR-MM, year=RRRR", http.StatusBadRequest)
			return
		}

		state.payments = &payments
	case query.Get("gregorian") != "":
		g, err := parseGregorianQuery(query)
		if err != nil {
			http.Error(w, "nieprawidłowe parametry okna mszy gregoriańskich, oczekiwano gregorian=active|done[&series=ID]", http.StatusBadRequest)
			return
		}

		state.gregorian = &g
	}

	state.saved, _ = strconv.Atoi(query.Get("saved"))
	state.undo = h.undoNotice(query.Get("deleted"))
	state.unpaid = query.Get("unpaid") == "1"

	h.renderWeek(w, http.StatusOK, start, state)
}

func (h *handler) today() string {
	return h.now().In(intentions.TimeZone).Format(intentions.DateLayout)
}

func (h *handler) weekStart(query url.Values) (time.Time, error) {
	param := query.Get("week")
	if param == "" {
		return intentions.NextWeekStart(h.now()), nil
	}

	day, err := time.ParseInLocation(intentions.DateLayout, param, intentions.TimeZone)
	if err != nil {
		return time.Time{}, err
	}

	return intentions.WeekStart(day), nil
}

func (h *handler) printPreview(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	start, err := h.weekStart(query)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		serverError(w)
		return
	}

	opts, schedules, ok := h.printOptions(w, query)
	if !ok {
		return
	}

	render(w, http.StatusOK, printTemplate, newPrintView(start, list, opts, schedules))
}

func (h *handler) printOptions(w http.ResponseWriter, query url.Values) (printOptions, []masses.Schedule, bool) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}

	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		serverError(w)
		return printOptions{}, nil, false
	}

	return parsePrintOptions(query, setting.SelectedScheduleID, schedules), schedules, true
}

func (h *handler) plannedForPrint(start time.Time, opts printOptions, schedules []masses.Schedule) (map[string][]time.Time, error) {
	if !opts.Blanks || !opts.Planned {
		return nil, nil
	}

	for i := range schedules {
		if schedules[i].ID != opts.ScheduleID {
			continue
		}

		list, err := masses.InRange(h.db, start, start.AddDate(0, 0, 7), &schedules[i])
		if err != nil {
			return nil, err
		}

		planned := make(map[string][]time.Time)
		for _, m := range list {
			if m.Planned {
				date := m.At.Format(intentions.DateLayout)
				planned[date] = append(planned[date], m.At)
			}
		}

		return planned, nil
	}

	return nil, nil
}

func (h *handler) pdf(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()

	start, err := h.weekStart(query)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		serverError(w)
		return
	}

	options, schedules, ok := h.printOptions(w, query)
	if !ok {
		return
	}
	planned, err := h.plannedForPrint(start, options, schedules)
	if err != nil {
		log.Printf("error loading planned masses for week %s: %v", start.Format(intentions.DateLayout), err)
		serverError(w)
		return
	}
	opts := pdfOptions{showHidden: options.ShowHidden, planned: planned}
	if options.Blanks {
		opts.blanks = options.Lines
		opts.extra = options.Extra
	}

	feasts, err := calendar.FeastsForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week feasts: %v", err)
		serverError(w)
		return
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		serverError(w)
		return
	}

	steps, err := gregorian.Steps(h.db, seriesIDs(list))
	if err != nil {
		log.Printf("error loading gregorian series steps: %v", err)
		serverError(w)
		return
	}

	view := newWeekView(start, list)
	setFeasts(view.Days, feasts)
	setSeriesSteps(view.Days, steps)
	view.ParishName = setting.ParishName
	view.Address = setting.Address

	var buf bytes.Buffer
	err = writeWeekPDF(&buf, view, opts)
	if err != nil {
		log.Printf("error generating pdf for week %s: %v", start.Format(intentions.DateLayout), err)
		serverError(w)
		return
	}

	filename := "intencje-" + start.Format(intentions.DateLayout) + ".pdf"
	if options.Blanks {
		filename = "intencje-" + start.Format(intentions.DateLayout) + "-do-wpisania.pdf"
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `inline; filename="`+filename+`"`)

	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func (h *handler) create(w http.ResponseWriter, r *http.Request) {
	h.save(w, r, intentions.Intention{})
}

func (h *handler) update(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	h.save(w, r, intention)
}

func (h *handler) delete(w http.ResponseWriter, r *http.Request) {
	intention, ok := h.findIntention(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var batch int64
	err := h.db.Transaction(func(tx *gorm.DB) error {
		var err error
		batch, err = trash.Move(tx, []intentions.Intention{intention}, time.Time{}, h.now())
		if err != nil {
			return err
		}
		err = tx.Delete(&intention).Error
		if err != nil {
			return err
		}

		return gregorian.Release(tx, []intentions.Intention{intention})
	})
	if err != nil {
		log.Printf("error deleting intention %d: %v", intention.ID, err)
		serverError(w)
		return
	}
	err = h.drafts.DiscardEdit(intention.ID)
	if err != nil {
		log.Printf("error removing draft of deleted intention %d: %v", intention.ID, err)
	}

	http.Redirect(w, r, deletedURL(intentions.WeekStart(intention.MassAt), batch), http.StatusSeeOther)
}

func (h *handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		serverError(w)
		return
	}

	query := r.URL.Query()
	view, ok := h.settingsView(w, setting, query.Get("week"))
	if !ok {
		return
	}
	view.Saved = query.Get("saved") == "1"
	if path := query.Get("exported"); path != "" {
		view.Notice = exportedNotice(path)
	}
	if backupName := query.Get("imported"); backupName != "" {
		view.Notice = importedNotice(backupName, h.dataDir)
	}
	if notice := backupDirNotice(query.Get("backup_dir")); notice != "" {
		view.Notice = notice
	}
	if n, err := strconv.Atoi(query.Get("backup_days")); err == nil {
		view.Notice = backupDaysNotice(n)
	}
	view.BackupError = backupErrorMessage(query.Get("backup_error"), view.BackupDir)

	closeURL := settingsURL(view.Week, false) + "#porzadki"
	switch {
	case query.Get("delete_schedule") != "":
		id, err := strconv.Atoi(query.Get("delete_schedule"))
		if err != nil {
			http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
			return
		}

		schedule, err := masses.Find(h.db, id)
		if errors.Is(err, masses.ErrScheduleNotFound) {
			http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
			return
		}
		if err != nil {
			log.Printf("error loading schedule %d: %v", id, err)
			serverError(w)
			return
		}

		view.Dialog = &settingsDialog{
			Title:    "Usunąć porządek mszy?",
			Text:     fmt.Sprintf("„%s” (%s). Intencje i godziny już wczytane do tygodni zostaną bez zmian.", schedule.Name, scheduleSummary(schedule)),
			Action:   "/settings/schedules/" + strconv.Itoa(id) + "/delete",
			Button:   "Usuń porządek",
			Week:     view.Week,
			CloseURL: closeURL,
		}
	case query.Get("restore") == "1":
		view.Dialog = &settingsDialog{
			Title:    "Przywrócić domyślne porządki?",
			Text:     "Wszystkie porządki mszy zostaną zastąpione domyślnymi z programu. Własne zmiany w porządkach przepadną.",
			Action:   "/settings/schedules/restore",
			Button:   "Przywróć domyślne",
			Week:     view.Week,
			CloseURL: closeURL,
		}
	case query.Get("import") == "1":
		h.importDialog(&view, settingsURL(view.Week, false))
	case query.Get("trash") == "1":
		view.openTrash(query.Get("restore"))
	}

	render(w, http.StatusOK, settingsTemplate, view)
}

func (h *handler) settingsView(w http.ResponseWriter, setting settings.Setting, week string) (settingsView, bool) {
	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		serverError(w)
		return settingsView{}, false
	}

	view := newSettingsView(setting, schedules, week)
	view.Topbar = h.topbar()

	batches, err := trash.List(h.db, h.now())
	if err != nil {
		log.Printf("error loading the trash: %v", err)
	}
	view.Trash = newTrashRows(batches, h.now(), view.Week)
	view.TrashURL = settingsTrashURL(view.Week, 0)
	view.DataDir = h.dataDir
	view.LogFile = h.logFile

	state := h.backups.State()
	view.BackupDir = state.Dir
	view.BackupLabel = backupLabel(state, h.now())
	view.CanChooseDir = h.chooseDir != nil
	view.DailyCopies = strconv.Itoa(state.DailyCopies)
	view.DailyMax = backup.MaxDailyCopies
	view.DailyList = dailyCopiesLabel(state)
	view.Update = newUpdateView(h.updater, h.now())
	view.Version = h.version

	return view, true
}

func (h *handler) topbar() topbarView {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}

	topbar := newTopbarView(setting, h.backups.State(), h.now())
	topbar.Draft = h.draftNotice()

	return topbar
}

func (h *handler) scheduleForm(w http.ResponseWriter, r *http.Request) {
	week := r.URL.Query().Get("week")

	idParam := chi.URLParam(r, "id")
	if idParam == "" {
		h.renderSchedule(w, http.StatusOK, newScheduleFormView(masses.Schedule{}, week))
		return
	}

	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return
	}

	schedule, err := masses.Find(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error loading schedule %d: %v", id, err)
		serverError(w)
		return
	}

	h.renderSchedule(w, http.StatusOK, newScheduleFormView(schedule, week))
}

func (h *handler) renderSchedule(w http.ResponseWriter, status int, view scheduleFormView) {
	view.Topbar = h.topbar()
	render(w, status, scheduleTemplate, view)
}

const duplicateScheduleMessage = "Porządek o tej nazwie już istnieje."

func (h *handler) saveSchedule(w http.ResponseWriter, r *http.Request) {
	id := 0
	if idParam := chi.URLParam(r, "id"); idParam != "" {
		var err error
		id, err = strconv.Atoi(idParam)
		if err != nil {
			http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
			return
		}
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := r.PostForm.Get("week")

	schedule, errs := scheduleFromForm(r.PostForm)
	schedule.ID = id
	if errs["name"] == "" {
		taken, err := masses.NameTaken(h.db, schedule.Name, id)
		if err != nil {
			log.Printf("error checking schedule name: %v", err)
			serverError(w)
			return
		}
		if taken {
			errs["name"] = duplicateScheduleMessage
		}
	}
	if len(errs) == 0 {
		_, err = masses.Save(h.db, schedule)
		switch {
		case errors.Is(err, masses.ErrDuplicateName):
			errs["name"] = duplicateScheduleMessage
		case errors.Is(err, masses.ErrScheduleNotFound):
			http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
			return
		case err != nil:
			log.Printf("error saving schedule %d: %v", id, err)
			serverError(w)
			return
		}
	}

	if len(errs) > 0 {
		view := newScheduleFormView(schedule, week)
		view.Name = r.PostForm.Get("name")
		for i := range view.Days {
			view.Days[i].Value = r.PostForm.Get(view.Days[i].Field)
			view.Days[i].Error = errs[view.Days[i].Field]
		}
		view.Errors = errs
		h.renderSchedule(w, http.StatusUnprocessableEntity, view)
		return
	}

	http.Redirect(w, r, settingsURL(week, true)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) deleteSchedule(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	err = masses.Delete(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error deleting schedule %d: %v", id, err)
		serverError(w)
		return
	}

	http.Redirect(w, r, settingsURL(r.PostForm.Get("week"), false)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) restoreSchedules(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	err = masses.RestoreDefaults(h.db)
	if err != nil {
		log.Printf("error restoring default schedules: %v", err)
		serverError(w)
		return
	}

	http.Redirect(w, r, settingsURL(r.PostForm.Get("week"), false)+"#porzadki", http.StatusSeeOther)
}

func (h *handler) saveSettings(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	setting, errs := settingsFromForm(r.PostForm)
	view, ok := h.settingsView(w, setting, r.PostForm.Get("week"))
	if !ok {
		return
	}

	if len(errs) > 0 {
		view.Errors = errs
		view.QuickAmounts = r.PostForm.Get("quick_amounts")
		view.QuickPreview = nil
		render(w, http.StatusUnprocessableEntity, settingsTemplate, view)
		return
	}

	err = settings.SaveParish(h.db, setting)
	if err != nil {
		log.Printf("error saving settings: %v", err)
		serverError(w)
		return
	}

	http.Redirect(w, r, settingsURL(view.Week, true), http.StatusSeeOther)
}

const importErrorMessage = "Nie udało się pobrać kalendarza z GCatholic. Sprawdź połączenie z internetem i spróbuj ponownie."

func (h *handler) importFeasts(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	years := []int{start.Year()}
	if end := start.AddDate(0, 0, 6); end.Year() != start.Year() {
		years = append(years, end.Year())
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		serverError(w)
		return
	}

	feasts := make(map[string]string)
	for _, year := range years {
		celebrations, err := h.fetchCelebrations(r.Context(), year)
		if err != nil {
			log.Printf("error fetching GCatholic calendar %d: %v", year, err)
			h.renderWeek(w, http.StatusBadGateway, start, weekState{importError: importErrorMessage})
			return
		}

		celebrations = slices.DeleteFunc(celebrations, func(c gcatholic.Celebration) bool {
			return skipped(setting.FeastImport, c.Kind())
		})
		texts := gcatholic.Texts(celebrations)
		for i := range 7 {
			date := start.AddDate(0, 0, i).Format(intentions.DateLayout)
			if text, ok := texts[date]; ok {
				feasts[date] = text
			}
		}
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		added, err := calendar.AddMissing(tx, feasts)
		if err != nil {
			return err
		}

		return calendar.MarkImported(tx, start, added, h.now())
	})
	if err != nil {
		log.Printf("error saving imported feasts for week %s: %v", start.Format(intentions.DateLayout), err)
		serverError(w)
		return
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout), http.StatusSeeOther)
}

func (h *handler) saveFeast(w http.ResponseWriter, r *http.Request) {
	day, err := time.ParseInLocation(intentions.DateLayout, chi.URLParam(r, "date"), intentions.TimeZone)
	if err != nil {
		http.Error(w, "nieprawidłowa data dnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	date := day.Format(intentions.DateLayout)
	start := intentions.WeekStart(day)
	feast := strings.TrimSpace(r.PostForm.Get("feast"))

	if utf8.RuneCountInString(feast) > maxFeastLength {
		f := newFeastView(day)
		f.Feast = feast
		f.Error = fmt.Sprintf("Maksymalnie %d znaków.", maxFeastLength)
		h.renderWeek(w, http.StatusUnprocessableEntity, start, weekState{feast: &f})
		return
	}

	err = calendar.SaveFeast(h.db, date, feast)
	if err != nil {
		log.Printf("error saving feast for %s: %v", date, err)
		serverError(w)
		return
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout)+"#d-"+date, http.StatusSeeOther)
}

func (h *handler) clearWeek(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	var batch int64
	err = h.db.Transaction(func(tx *gorm.DB) error {
		list, err := intentions.ForWeek(tx, start)
		if err != nil {
			return err
		}
		batch, err = trash.Move(tx, list, start, h.now())
		if err != nil {
			return err
		}
		err = intentions.DeleteWeek(tx, start)
		if err != nil {
			return err
		}
		err = masses.Unmark(tx, start)
		if err != nil {
			return err
		}

		return gregorian.Release(tx, list)
	})
	if err != nil {
		log.Printf("error clearing week %s: %v", start.Format(intentions.DateLayout), err)
		serverError(w)
		return
	}

	http.Redirect(w, r, deletedURL(start, batch), http.StatusSeeOther)
}

func postedWeekStart(r *http.Request) (time.Time, error) {
	day, err := time.ParseInLocation(intentions.DateLayout, r.PostForm.Get("week"), intentions.TimeZone)
	if err != nil {
		return time.Time{}, err
	}

	return intentions.WeekStart(day), nil
}

func (h *handler) createSlots(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(r.PostForm.Get("schedule"))
	if err != nil {
		http.Error(w, "nieznany porządek mszy", http.StatusBadRequest)
		return
	}

	schedule, err := masses.Find(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nieznany porządek mszy", http.StatusBadRequest)
		return
	}
	if err != nil {
		log.Printf("error loading schedule %d: %v", id, err)
		serverError(w)
		return
	}

	h.slotsMu.Lock()
	defer h.slotsMu.Unlock()

	existing, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		serverError(w)
		return
	}

	slots := schedule.Slots(start, existing)
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if len(slots) > 0 {
			err := tx.Create(&slots).Error
			if err != nil {
				return err
			}
		}

		return masses.MarkLoaded(tx, start, schedule.ID)
	})
	if err != nil {
		log.Printf("error creating slots for week %s: %v", start.Format(intentions.DateLayout), err)
		serverError(w)
		return
	}

	err = settings.SaveSchedule(h.db, schedule.ID)
	if err != nil {
		log.Printf("error saving selected schedule %d: %v", schedule.ID, err)
	}

	http.Redirect(w, r, "/?week="+start.Format(intentions.DateLayout), http.StatusSeeOther)
}

func (h *handler) findIntention(w http.ResponseWriter, idParam string) (intentions.Intention, bool) {
	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator intencji", http.StatusBadRequest)
		return intentions.Intention{}, false
	}

	var intention intentions.Intention
	err = h.db.First(&intention, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		http.Error(w, "nie znaleziono intencji", http.StatusNotFound)
		return intentions.Intention{}, false
	}
	if err != nil {
		log.Printf("error finding intention %d: %v", id, err)
		serverError(w)
		return intentions.Intention{}, false
	}

	return intention, true
}

func (h *handler) save(w http.ResponseWriter, r *http.Request, intention intentions.Intention) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	day := r.PostForm.Get("day")
	massTime := r.PostForm.Get("time")
	content := r.PostForm.Get("content")
	outside := r.PostForm.Get("outside") == "on"
	isGregorian := r.PostForm.Get("gregorian") == "on"
	deceased := r.PostForm.Get("deceased") == "on"
	hidden := r.PostForm.Get("hidden") == "on"
	amountText := r.PostForm.Get("amount")
	amount, amountErr := intentions.ParseGrosze(amountText)
	copying := r.PostForm.Get("copy") == "1"
	acceptedOn := strings.TrimSpace(r.PostForm.Get("accepted"))

	seriesID := intention.SeriesID
	if intention.ID == 0 {
		seriesID = 0
		if param := r.PostForm.Get("series"); param != "" {
			seriesID, err = strconv.Atoi(param)
			if err != nil || seriesID < 0 {
				http.Error(w, "nieprawidłowy identyfikator serii", http.StatusBadRequest)
				return
			}
		}
	}

	var summary gregorian.Summary
	seriesErr := ""
	if seriesID > 0 {
		summary, err = gregorian.Load(h.db, seriesID, h.now())
		switch {
		case errors.Is(err, gregorian.ErrSeriesNotFound):
			if intention.ID == 0 {
				seriesErr = "Ta seria gregoriańska już nie istnieje."
			}
			seriesID = 0
		case err != nil:
			log.Printf("error loading gregorian series %d: %v", seriesID, err)
			serverError(w)
			return
		}
	}

	step := 0
	if seriesID > 0 {
		isGregorian, deceased, outside = true, true, summary.Series.IsOutsideParish
		for i, m := range summary.Masses {
			if m.ID == intention.ID {
				step = i + 1
			}
		}
		if step != 1 {
			content = summary.Series.Content
			amount, amountErr = 0, nil
			if len(summary.Masses) > 0 && summary.Masses[0].AcceptedOn != "" {
				acceptedOn = summary.Masses[0].AcceptedOn
			}
		}
		if other := summary.MassOn(day); other != nil && other.ID != intention.ID {
			seriesErr = "Ta seria ma już Mszę w tym dniu."
		}
		if intention.ID == 0 {
			switch {
			case summary.Status != gregorian.StatusActive:
				seriesErr = "Ta seria jest już zakończona albo przerwana."
			case summary.Count() >= gregorian.Length:
				seriesErr = "Ta seria ma już 30 Mszy."
			}
		}
	} else if copying && isGregorian {
		amount = 0
	}

	today := h.today()
	if acceptedOn == "" && (intention.ID == 0 || intention.IsPlaceholder) {
		acceptedOn = today
	}

	input := intentions.Input{
		MassAt:          day + " " + massTime,
		Content:         strings.TrimSpace(content),
		IsOutsideParish: outside,
		IsGregorian:     isGregorian,
		IsDeceased:      deceased,
		IsHidden:        hidden,
		Amount:          amount,
		AcceptedOn:      acceptedOn,
	}

	err = h.validate.Struct(input)
	messages, ok := intentions.ValidationMessages(err)
	if err != nil && !ok {
		log.Printf("unexpected validation error: %v", err)
		serverError(w)
		return
	}

	errs := formErrors(day, massTime, messages, amountErr)
	if _, bad := errs["accepted"]; !bad && acceptedOn > today {
		errs["accepted"] = "Data przyjęcia nie może być późniejsza niż dzisiaj."
	}
	if seriesErr != "" {
		errs["series"] = seriesErr
	}
	if len(errs) > 0 {
		start := intentions.NextWeekStart(h.now())
		if week, parseErr := time.ParseInLocation(intentions.DateLayout, r.PostForm.Get("week"), intentions.TimeZone); parseErr == nil {
			start = intentions.WeekStart(week)
		}

		form := newFormView(start, day, massTime)
		form.Content = content
		form.IsOutsideParish = outside
		form.IsGregorian = isGregorian
		form.IsDeceased = deceased
		form.IsHidden = hidden
		form.Amount = amountText
		form.AcceptedOn = acceptedOn
		form.Errors = errs
		form.EditID = intention.ID
		form.placeholder = intention.IsPlaceholder
		form.SeriesID = seriesID
		if token := r.PostForm.Get("draft_token"); len(token) <= maxDraftToken {
			form.DraftToken = token
		}
		if r.PostForm.Get("from") == fromSearch {
			h.openedFromSearch(&form)
		}

		h.renderWeek(w, http.StatusUnprocessableEntity, start, weekState{form: &form})
		return
	}

	newSeries := intention.ID == 0 && seriesID == 0 && isGregorian && !copying
	rename := seriesID > 0 && step == 1 && input.Content != summary.Series.Content
	syncAccepted := seriesID > 0 && step == 1 && input.AcceptedOn != "" && input.AcceptedOn != intention.AcceptedOn

	err = h.db.Transaction(func(tx *gorm.DB) error {
		switch {
		case newSeries:
			series, err := gregorian.Create(tx, input.Content, input.IsOutsideParish)
			if err != nil {
				return err
			}
			intention.SeriesID = series.ID
		case seriesID > 0:
			intention.SeriesID = seriesID
		}
		if rename {
			err := gregorian.Rename(tx, seriesID, input.Content)
			if err != nil {
				return err
			}
		}
		if syncAccepted {
			err := gregorian.SetAccepted(tx, seriesID, input.AcceptedOn)
			if err != nil {
				return err
			}
		}

		intention.Apply(input)
		err := intentions.Save(tx, &intention)
		if err != nil {
			return err
		}
		if intention.SeriesID > 0 {
			return gregorian.Normalize(tx, intention.SeriesID)
		}

		return nil
	})
	if err != nil {
		log.Printf("error saving intention %d: %v", intention.ID, err)
		serverError(w)
		return
	}
	h.finishDraft(r.PostForm.Get("draft_token"))

	week := intentions.WeekStart(intention.MassAt).Format(intentions.DateLayout)
	if copying {
		query := url.Values{}
		query.Set("week", week)
		query.Set("edit", strconv.Itoa(intention.ID))
		query.Set("saved", strconv.Itoa(intention.ID))
		http.Redirect(w, r, "/?"+query.Encode(), http.StatusSeeOther)
		return
	}

	if r.PostForm.Get("from") == fromSearch {
		http.Redirect(w, r, h.backToSearchURL(week, intention.ID), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, fmt.Sprintf("/?week=%s&saved=%d#i-%d", week, intention.ID, intention.ID), http.StatusSeeOther)
}

func formErrors(day, massTime string, messages map[string]string, amountErr error) map[string]string {
	errs := make(map[string]string)

	if day == "" {
		errs["day"] = intentions.RequiredMessage
	}
	if massTime == "" {
		errs["time"] = intentions.RequiredMessage
	}
	if message, ok := messages["mass_at"]; ok && day != "" && massTime != "" {
		errs["time"] = message
	}
	if message, ok := messages["content"]; ok {
		errs["content"] = message
	}
	if amountErr != nil {
		errs["amount"] = intentions.AmountMessage(amountErr)
	} else if message, ok := messages["amount"]; ok {
		errs["amount"] = message
	}
	if message, ok := messages["accepted_on"]; ok {
		errs["accepted"] = message
	}

	return errs
}

type weekState struct {
	form         *formView
	delete       *deleteView
	feast        *feastView
	clear        bool
	calendarYear int
	payments     *paymentsQuery
	gregorian    *gregorianQuery
	saved        int
	unpaid       bool
	importError  string
	undo         *undoView
}

func (h *handler) renderWeek(w http.ResponseWriter, status int, start time.Time, state weekState) {
	list, err := intentions.ForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week intention: %v", err)
		serverError(w)
		return
	}

	feasts, err := calendar.FeastsForWeek(h.db, start)
	if err != nil {
		log.Printf("error loading week feasts: %v", err)
		serverError(w)
		return
	}

	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		serverError(w)
		return
	}

	view := newWeekView(start, list)
	view.SearchURL, view.SearchRemembered = h.searches.url(start.Format(intentions.DateLayout))
	setFeasts(view.Days, feasts)
	if last, ok, err := calendar.LastImport(h.db, start); err != nil {
		log.Printf("error loading the feast import of week %s: %v", start.Format(intentions.DateLayout), err)
	} else if ok {
		view.FeastImport = newFeastImportView(last, h.now())
	}
	upcoming := intentions.NextWeekStart(h.now())
	view.IsUpcoming = start.Equal(upcoming)
	view.Relation = newWeekRelation(start, h.now())
	view.UpcomingURL = weekURL(upcoming)
	view.Schedules = schedules

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}
	view.SelectedScheduleID = setting.SelectedScheduleID
	view.Topbar = newTopbarView(setting, h.backups.State(), h.now())
	if state.form == nil {
		view.Topbar.Draft = h.draftNotice()
	}

	active, err := gregorian.Active(h.db, h.now())
	if err != nil {
		log.Printf("error loading gregorian series: %v", err)
		serverError(w)
		return
	}
	steps, err := gregorian.Steps(h.db, seriesIDs(list))
	if err != nil {
		log.Printf("error loading gregorian series steps: %v", err)
		serverError(w)
		return
	}
	setSeriesSteps(view.Days, steps)
	view.ActiveSeries = len(active)
	markSeriesDue(view.Days, active, view.Week())
	view.SeriesNotices = seriesNotices(active, start)

	if state.form != nil {
		state.form.Today = h.today()
		if state.form.EditID == 0 && state.form.AcceptedOn == "" {
			state.form.AcceptedOn = state.form.Today
		}
		err = h.prepareSeriesForm(state.form, active)
		if err != nil {
			log.Printf("error loading gregorian series for the form: %v", err)
			serverError(w)
			return
		}
		state.form.QuickAmounts = quickAmountOptions(setting.QuickAmounts, state.form.Amount)
		state.form.OtherDraft = h.otherDraft(state.form.DraftToken)
		view.Form = state.form
		markSelected(view.Days, state.form.Day, state.form.Time)
		markEditing(view.Days, state.form.EditID)
		state.form.SharedWarning = sharedWarning(view.Days, state.form)
	}
	view.Delete = state.delete
	if state.clear && len(list) > 0 {
		c := newClearView(list)
		view.Clear = &c
	}
	view.Feast = state.feast
	if state.calendarYear != 0 {
		from, to := calendarRange(state.calendarYear)
		counts, err := intentions.CountByDay(h.db, from, to)
		if err != nil {
			log.Printf("error counting intentions for %d: %v", state.calendarYear, err)
			serverError(w)
			return
		}

		yearMasses, err := masses.InRange(h.db, from, to, nil)
		if err != nil {
			log.Printf("error loading masses for %d: %v", state.calendarYear, err)
			serverError(w)
			return
		}

		c := newCalendarView(state.calendarYear, start, h.now(), counts, massesByDay(yearMasses))
		if state.calendarYear < h.now().In(intentions.TimeZone).Year() && yearHasIntentions(state.calendarYear, counts) {
			c.YearPrintURL = yearPrintURL(state.calendarYear, start.Format(intentions.DateLayout))
		}
		view.Calendar = &c
	}
	if state.payments != nil {
		from, to := state.payments.span(start)
		counts, err := intentions.CountByDay(h.db, from, to)
		if err != nil {
			log.Printf("error summing payments from %s to %s: %v", from.Format(intentions.DateLayout), to.Format(intentions.DateLayout), err)
			serverError(w)
			return
		}

		p := newPaymentsView(*state.payments, start, h.now(), counts)
		if week := view.WeekPayments(); p.Tab == paymentsWeek && week != nil && week.Unpaid > 0 {
			p.UnpaidURL = unpaidURL(view.Week(), week.firstUnpaid)
		}
		view.PaymentsDialog = &p
	}
	if state.unpaid {
		view.ShowUnpaid = true
		markUnpaid(view.Days)
	}
	if state.gregorian != nil {
		var closed []gregorian.Summary
		var total int64
		if state.gregorian.tab == seriesClosed || (state.gregorian.open > 0 && !containsSeries(active, state.gregorian.open)) {
			closed, total, err = gregorian.Closed(h.db, h.now(), closedSeriesLimit)
		} else {
			total, err = gregorian.CountClosed(h.db, h.now())
		}
		if err != nil {
			log.Printf("error loading closed gregorian series: %v", err)
			serverError(w)
			return
		}

		g := newGregorianView(*state.gregorian, start, h.now(), active, closed, total)
		view.Gregorian = &g
	}
	view.ImportError = state.importError
	view.Undo = state.undo
	markSaved(view.Days, state.saved)

	render(w, status, weekTemplate, view)
}

func render(w http.ResponseWriter, status int, tmpl *template.Template, data any) {
	var buf bytes.Buffer

	err := tmpl.Execute(&buf, data)
	if err != nil {
		log.Printf("error rendering template %s: %v", tmpl.Name(), err)
		serverError(w)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)

	_, err = buf.WriteTo(w)
	if err != nil {
		log.Printf("error writing response: %v", err)
	}
}

func (h *handler) prepareSeriesForm(form *formView, active []gregorian.Summary) error {
	if form.EditID > 0 {
		if form.SeriesID == 0 {
			return nil
		}

		summary, err := gregorian.Load(h.db, form.SeriesID, h.now())
		if errors.Is(err, gregorian.ErrSeriesNotFound) {
			form.SeriesID = 0
			return nil
		}
		if err != nil {
			return err
		}

		form.SeriesLocked = true
		for i, m := range summary.Masses {
			if m.ID == form.EditID {
				form.SeriesStep = i + 1
			}
		}
		form.SeriesPaid = paidLabel(summary)
		form.IsGregorian = true
		form.IsDeceased = true
		form.IsOutsideParish = summary.Series.IsOutsideParish

		return nil
	}

	form.SeriesOptions = newSeriesOptions(active, form.Day, form.SeriesID)
	selected := form.SeriesID
	form.SeriesID = 0
	for _, option := range form.SeriesOptions {
		if option.ID != selected || !option.Checked {
			continue
		}

		form.SeriesID = option.ID
		form.SeriesStep = option.Step
		form.SeriesPaid = option.Paid
		form.Content = option.Content
		form.IsGregorian = true
		form.IsDeceased = true
		form.IsOutsideParish = option.Outside
		if form.Time == "" {
			form.Time = option.Time
			form.Times = timeOptions(form.Time)
		}
	}

	return nil
}

func sharedWarning(days []dayView, form *formView) bool {
	for _, day := range days {
		if day.Date != form.Day {
			continue
		}
		for _, mass := range day.Masses {
			if mass.TimeValue != form.Time {
				continue
			}

			if form.IsOutsideParish {
				return false
			}

			others, gregorians := mass.ParishFilled(), mass.ParishGregorians()
			for _, intention := range mass.Parish {
				if intention.ID != form.EditID || intention.IsPlaceholder {
					continue
				}
				others--
				if intention.IsGregorian {
					gregorians--
				}
			}

			return others > 0 && (form.IsGregorian || form.SeriesID > 0 || gregorians > 0)
		}
	}

	return false
}

func (h *handler) stopSeries(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator serii", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	start, err := postedWeekStart(r)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return
	}

	err = gregorian.Stop(h.db, id, h.now())
	if errors.Is(err, gregorian.ErrSeriesNotFound) {
		http.Error(w, "nie znaleziono serii gregoriańskiej", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error stopping gregorian series %d: %v", id, err)
		serverError(w)
		return
	}

	http.Redirect(w, r, gregorianURL(start.Format(intentions.DateLayout), seriesClosed, id), http.StatusSeeOther)
}
