package web

import (
	"cmp"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

const (
	maxFixedContentLength = 500
	fixedMassMessage      = "Wybierz Mszę z listy."
)

type fixedPageView struct {
	ScheduleName string
	Week         string
	BackURL      string
	Days         []fixedDay

	FormTitle       string
	Action          string
	CancelURL       string
	Editing         bool
	Masses          []fixedMassOption
	Content         string
	IsOutsideParish bool
	IsDeceased      bool
	IsHidden        bool
	Errors          map[string]string

	Dialog *settingsDialog
	Topbar topbarView
}

type fixedDay struct {
	Label  string
	Masses []fixedMass
}

type fixedMass struct {
	Time       string
	Anchor     string
	AddURL     string
	Intentions []fixedItem
}

type fixedItem struct {
	Content         string
	IsOutsideParish bool
	IsDeceased      bool
	IsGregorian     bool
	IsHidden        bool
	Editing         bool
	EditURL         string
	DeleteURL       string
}

type fixedMassOption struct {
	Value    string
	Label    string
	Selected bool
}

func (v fixedPageView) MaxLength() int {
	return maxFixedContentLength
}

func fixedPath(scheduleID int) string {
	return "/settings/schedules/" + strconv.Itoa(scheduleID) + "/fixed"
}

func fixedPageURL(scheduleID int, week string) string {
	return withWeek(fixedPath(scheduleID), week)
}

func massValue(day time.Weekday, clock string) string {
	return strconv.Itoa(int(day)) + "-" + clock
}

func massAnchor(day time.Weekday, clock string) string {
	return "msza-" + strconv.Itoa(int(day)) + "-" + strings.ReplaceAll(clock, ":", "")
}

func massLabel(day time.Weekday, clock string) string {
	return weekdayNames[day] + ", " + strings.TrimPrefix(clock, "0")
}

func parseMassValue(value string) (time.Weekday, string, bool) {
	dayText, clock, ok := strings.Cut(value, "-")
	if !ok {
		return 0, "", false
	}

	day, err := strconv.Atoi(dayText)
	if err != nil || day < int(time.Sunday) || day > int(time.Saturday) {
		return 0, "", false
	}

	return time.Weekday(day), clock, true
}

func newFixedPageView(s masses.Schedule, week string, editing masses.FixedIntention) fixedPageView {
	week = normalizeWeek(week)
	base := fixedPath(s.ID)
	view := fixedPageView{
		ScheduleName: s.Name,
		Week:         week,
		BackURL:      settingsURL(week, false) + "#porzadki",
		FormTitle:    "Dodaj stałą intencję",
		Action:       base,
		CancelURL:    fixedPageURL(s.ID, week),
	}
	if editing.ID != 0 {
		view.Editing = true
		view.FormTitle = "Zmień stałą intencję"
		view.Action = base + "/" + strconv.Itoa(editing.ID)
		view.Content = editing.Content
		view.IsOutsideParish = editing.IsOutsideParish
		view.IsDeceased = editing.IsDeceased
		view.IsHidden = editing.IsHidden
	}

	for _, day := range formWeekdays {
		if len(s.Masses[day]) == 0 {
			continue
		}

		row := fixedDay{Label: weekdayNames[day]}
		for _, clock := range s.Masses[day] {
			mass := fixedMass{
				Time:   strings.TrimPrefix(clock, "0"),
				Anchor: massAnchor(day, clock),
				AddURL: withWeek(base+"?mass="+url.QueryEscape(massValue(day, clock)), week) + "#formularz",
			}
			for _, f := range s.FixedAt(day, clock) {
				mass.Intentions = append(mass.Intentions, fixedItem{
					Content:         f.Content,
					IsOutsideParish: f.IsOutsideParish,
					IsDeceased:      f.IsDeceased,
					IsHidden:        f.IsHidden,
					Editing:         f.ID == editing.ID,
					EditURL:         withWeek(base+"?edit="+strconv.Itoa(f.ID), week) + "#formularz",
					DeleteURL:       withWeek(base+"?delete="+strconv.Itoa(f.ID), week),
				})
			}
			row.Masses = append(row.Masses, mass)

			view.Masses = append(view.Masses, fixedMassOption{
				Value: massValue(day, clock),
				Label: massLabel(day, clock),
			})
		}
		view.Days = append(view.Days, row)
	}

	if editing.ID != 0 {
		view.selectMass(massValue(editing.Weekday, editing.Time))
	}

	return view
}

func (v *fixedPageView) selectMass(value string) {
	for i := range v.Masses {
		v.Masses[i].Selected = v.Masses[i].Value == value
	}
}

func fixedFromForm(form url.Values, s masses.Schedule) (masses.FixedIntention, map[string]string) {
	f := masses.FixedIntention{
		ScheduleID:      s.ID,
		Content:         strings.TrimSpace(form.Get("content")),
		IsOutsideParish: form.Get("outside") == "on",
		IsDeceased:      form.Get("deceased") == "on",
		IsHidden:        form.Get("hidden") == "on",
	}
	errs := make(map[string]string)

	day, clock, ok := parseMassValue(form.Get("mass"))
	if ok && s.HasMass(day, clock) {
		f.Weekday, f.Time = day, clock
	} else {
		errs["mass"] = fixedMassMessage
	}

	switch {
	case f.Content == "":
		errs["content"] = intentions.RequiredMessage
	case utf8.RuneCountInString(f.Content) > maxFixedContentLength:
		errs["content"] = fmt.Sprintf("Maksymalnie %d znaków.", maxFixedContentLength)
	}

	return f, errs
}

func inWeekOrder(list []masses.FixedIntention) []masses.FixedIntention {
	sorted := slices.Clone(list)
	slices.SortStableFunc(sorted, func(a, b masses.FixedIntention) int {
		return cmp.Or(
			cmp.Compare((a.Weekday+6)%7, (b.Weekday+6)%7),
			cmp.Compare(a.Time, b.Time),
			cmp.Compare(a.ID, b.ID),
		)
	})

	return sorted
}

func fixedSummary(list []masses.FixedIntention) string {
	parts := make([]string, 0, len(list))
	for _, f := range inWeekOrder(list) {
		parts = append(parts, weekdayShortNames[f.Weekday]+" "+strings.TrimPrefix(f.Time, "0")+" – "+f.Content)
	}

	return strings.Join(parts, " · ")
}

type fixedDrop struct {
	ID    int
	Label string
}

func fixedDrops(list []masses.FixedIntention) []fixedDrop {
	drops := make([]fixedDrop, 0, len(list))
	for _, f := range inWeekOrder(list) {
		drops = append(drops, fixedDrop{ID: f.ID, Label: massLabel(f.Weekday, f.Time) + " – " + f.Content})
	}

	return drops
}

func unconfirmedDrops(orphans []masses.FixedIntention, confirmed []string) bool {
	for _, f := range orphans {
		if !slices.Contains(confirmed, strconv.Itoa(f.ID)) {
			return true
		}
	}

	return false
}

func (h *handler) loadSchedule(w http.ResponseWriter, idParam string) (masses.Schedule, bool) {
	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return masses.Schedule{}, false
	}

	schedule, err := masses.Find(h.db, id)
	if errors.Is(err, masses.ErrScheduleNotFound) {
		http.Error(w, "nie znaleziono porządku mszy", http.StatusNotFound)
		return masses.Schedule{}, false
	}
	if err != nil {
		log.Printf("error loading schedule %d: %v", id, err)
		serverError(w)
		return masses.Schedule{}, false
	}

	return schedule, true
}

func (h *handler) loadFixed(w http.ResponseWriter, scheduleID int, idParam string) (masses.FixedIntention, bool) {
	id, err := strconv.Atoi(idParam)
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator stałej intencji", http.StatusBadRequest)
		return masses.FixedIntention{}, false
	}

	fixed, err := masses.FindFixed(h.db, scheduleID, id)
	if errors.Is(err, masses.ErrFixedNotFound) {
		http.Error(w, "nie znaleziono stałej intencji", http.StatusNotFound)
		return masses.FixedIntention{}, false
	}
	if err != nil {
		log.Printf("error loading fixed intention %d: %v", id, err)
		serverError(w)
		return masses.FixedIntention{}, false
	}

	return fixed, true
}

func (h *handler) fixedPage(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.loadSchedule(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	query := r.URL.Query()
	week := query.Get("week")

	var editing masses.FixedIntention
	if param := query.Get("edit"); param != "" {
		editing, ok = h.loadFixed(w, schedule.ID, param)
		if !ok {
			return
		}
	}

	view := newFixedPageView(schedule, week, editing)
	if editing.ID == 0 {
		view.selectMass(query.Get("mass"))
	}

	if param := query.Get("delete"); param != "" {
		fixed, ok := h.loadFixed(w, schedule.ID, param)
		if !ok {
			return
		}

		view.Dialog = &settingsDialog{
			Title:    "Usunąć stałą intencję?",
			Text:     fmt.Sprintf("%s: „%s”. Intencje już wczytane do tygodni zostaną bez zmian.", massLabel(fixed.Weekday, fixed.Time), fixed.Content),
			Action:   fixedPath(schedule.ID) + "/" + strconv.Itoa(fixed.ID) + "/delete",
			Button:   "Usuń stałą intencję",
			Week:     view.Week,
			CloseURL: fixedPageURL(schedule.ID, view.Week),
		}
	}

	h.renderFixed(w, http.StatusOK, view)
}

func (h *handler) renderFixed(w http.ResponseWriter, status int, view fixedPageView) {
	view.Topbar = h.topbar()
	render(w, status, fixedTemplate, view)
}

func (h *handler) saveFixed(w http.ResponseWriter, r *http.Request) {
	schedule, ok := h.loadSchedule(w, chi.URLParam(r, "id"))
	if !ok {
		return
	}

	var existing masses.FixedIntention
	if param := chi.URLParam(r, "fid"); param != "" {
		existing, ok = h.loadFixed(w, schedule.ID, param)
		if !ok {
			return
		}
	}

	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	week := normalizeWeek(r.PostForm.Get("week"))

	fixed, errs := fixedFromForm(r.PostForm, schedule)
	fixed.ID = existing.ID
	if len(errs) == 0 {
		err = masses.SaveFixed(h.db, &fixed)
		switch {
		case errors.Is(err, masses.ErrNoSuchMass):
			errs["mass"] = fixedMassMessage
		case errors.Is(err, masses.ErrFixedNotFound), errors.Is(err, masses.ErrScheduleNotFound):
			http.Error(w, "nie znaleziono stałej intencji", http.StatusNotFound)
			return
		case err != nil:
			log.Printf("error saving fixed intention for schedule %d: %v", schedule.ID, err)
			serverError(w)
			return
		}
	}

	if len(errs) > 0 {
		view := newFixedPageView(schedule, week, existing)
		view.selectMass(r.PostForm.Get("mass"))
		view.Content = r.PostForm.Get("content")
		view.IsOutsideParish = fixed.IsOutsideParish
		view.IsDeceased = fixed.IsDeceased
		view.IsHidden = fixed.IsHidden
		view.Errors = errs
		h.renderFixed(w, http.StatusUnprocessableEntity, view)
		return
	}

	http.Redirect(w, r, fixedPageURL(schedule.ID, week)+"#"+massAnchor(fixed.Weekday, fixed.Time), http.StatusSeeOther)
}

func (h *handler) deleteFixed(w http.ResponseWriter, r *http.Request) {
	scheduleID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator porządku", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(chi.URLParam(r, "fid"))
	if err != nil {
		http.Error(w, "nieprawidłowy identyfikator stałej intencji", http.StatusBadRequest)
		return
	}

	err = r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	err = masses.DeleteFixed(h.db, scheduleID, id)
	if errors.Is(err, masses.ErrFixedNotFound) {
		http.Error(w, "nie znaleziono stałej intencji", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error deleting fixed intention %d: %v", id, err)
		serverError(w)
		return
	}

	http.Redirect(w, r, fixedPageURL(scheduleID, normalizeWeek(r.PostForm.Get("week"))), http.StatusSeeOther)
}
