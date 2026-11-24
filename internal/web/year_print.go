package web

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/signintech/gopdf"

	"go-parafia/internal/calendar"
	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/safefile"
	"go-parafia/internal/settings"
)

const yearFootSize = 7.5

func yearSpan(year int) (time.Time, time.Time) {
	first := time.Date(year, time.January, 1, 0, 0, 0, 0, intentions.TimeZone)

	return first, first.AddDate(1, 0, 0)
}

func yearPrintURL(year int, week string) string {
	query := url.Values{"year": {strconv.Itoa(year)}}
	if week != "" {
		query.Set("week", week)
	}

	return "/print/year?" + query.Encode()
}

func splitWeeks(year int, list []intentions.Intention) []weekView {
	from, to := yearSpan(year)

	var weeks []weekView
	next := 0
	for start := intentions.WeekStart(from); start.Before(to); start = start.AddDate(0, 0, 7) {
		end := start.AddDate(0, 0, 7)
		first := next
		for next < len(list) && list[next].MassAt.Before(end) {
			next++
		}
		weeks = append(weeks, newWeekView(start, list[first:next]))
	}

	return weeks
}

func layoutYear(year int, weeks []weekView, opts pdfOptions, m textMeasurer) []pdfPage {
	var pages []pdfPage
	for _, view := range weeks {
		days := slices.DeleteFunc(printableDays(view, opts), func(d printedDay) bool {
			return d.date.Year() != year
		})
		if !slices.ContainsFunc(days, func(d printedDay) bool { return len(d.masses) > 0 }) {
			continue
		}
		days[len(days)-1].ruleBelow = false

		header := layoutHeaderRange(view, rangeInWords(days[0].date, days[len(days)-1].date), m)
		pages = append(pages, layoutPrintedDays(header, days, m)...)
	}

	if len(pages) == 0 {
		size := 14.0
		pages = append(pages, pdfPage{Runs: []pdfRun{centered(fmt.Sprintf("Brak intencji w roku %d.", year), fontItalic, size, 0, marginTop+baseline(size, 1.3), m)}})
	}

	for i := range pages {
		label := fmt.Sprintf("Intencje mszy świętych – rok %d · str. %d z %d", year, i+1, len(pages))
		pages[i].Runs = append(pages[i].Runs, centered(label, fontRegular, yearFootSize, 0, pageHeight-5*mm, m))
	}

	return pages
}

func writeYearPDF(w io.Writer, year int, weeks []weekView, opts pdfOptions) error {
	return writePDF(w, fmt.Sprintf("Intencje mszy świętych – rok %d", year), gopdf.PageSizeA4, func(m textMeasurer) []pdfPage {
		return layoutYear(year, weeks, opts, m)
	})
}

func (h *handler) printYear(w http.ResponseWriter, query url.Values) (int, bool) {
	year, err := strconv.Atoi(query.Get("year"))
	if err != nil || year < 1000 || year >= h.now().In(intentions.TimeZone).Year() {
		http.Error(w, "nieprawidłowy rok, oczekiwano RRRR – rok, który już minął", http.StatusBadRequest)
		return 0, false
	}

	return year, true
}

func (h *handler) yearWeeks(year int) ([]weekView, error) {
	from, to := yearSpan(year)
	list, err := intentions.ForRange(h.db, from, to)
	if err != nil {
		return nil, fmt.Errorf("loading intentions: %w", err)
	}

	feasts, err := calendar.FeastsBetween(h.db, from, to)
	if err != nil {
		return nil, fmt.Errorf("loading feasts: %w", err)
	}

	steps, err := gregorian.Steps(h.db, seriesIDs(list))
	if err != nil {
		return nil, fmt.Errorf("loading gregorian steps: %w", err)
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		return nil, fmt.Errorf("loading settings: %w", err)
	}

	weeks := splitWeeks(year, list)
	for i := range weeks {
		setFeasts(weeks[i].Days, feasts)
		setSeriesSteps(weeks[i].Days, steps)
		weeks[i].ParishName = setting.ParishName
		weeks[i].Address = setting.Address
	}

	return weeks, nil
}

func (h *handler) yearPDFBytes(year int, showHidden bool) (*bytes.Buffer, error) {
	weeks, err := h.yearWeeks(year)
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	hidden := hiddenSkip
	if showHidden {
		hidden = hiddenInclude
	}
	err = writeYearPDF(&buf, year, weeks, pdfOptions{hidden: hidden})
	if err != nil {
		return nil, err
	}

	return &buf, nil
}

func yearFilename(year int) string {
	return "intencje-" + strconv.Itoa(year) + ".pdf"
}

func (h *handler) yearPrintPage(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	year, ok := h.printYear(w, query)
	if !ok {
		return
	}
	showHidden := query.Get("hidden") == "1"

	from, to := yearSpan(year)
	var hidden int64
	err := h.db.Model(&intentions.Intention{}).
		Where("mass_at >= ? AND mass_at < ? AND is_hidden = ? AND is_placeholder = ?", from, to, true, false).
		Count(&hidden).Error
	if err != nil {
		log.Printf("error counting hidden intentions of %d: %v", year, err)
		serverError(w)
		return
	}

	week := query.Get("week")
	if _, err := time.ParseInLocation(intentions.DateLayout, week, intentions.TimeZone); err != nil {
		week = intentions.NextWeekStart(h.now()).Format(intentions.DateLayout)
	}

	pdfQuery := url.Values{"year": {strconv.Itoa(year)}}
	fields := []previewField{{Name: "year", Value: strconv.Itoa(year)}, {Name: "week", Value: week}}
	if showHidden {
		pdfQuery.Set("hidden", "1")
	}
	saveFields := fields
	if showHidden {
		saveFields = append(slices.Clone(fields), previewField{Name: "hidden", Value: "1"})
	}

	view := previewView{
		Title:     fmt.Sprintf("Wydruk roku %d", year),
		BackURL:   calendarURL(week, year),
		BackLabel: "Wróć do kalendarza",
		Note:      fmt.Sprintf("INTENCJE MSZY ŚWIĘTYCH – ROK %d · A4 pionowo, tydzień od nowej strony", year),
		PDFURL:    "/pdf/year?" + pdfQuery.Encode(),
		Action:    "/print/year",
		Hidden:    fields,
		Checks: []previewCheck{{
			Name:     "hidden",
			Label:    "Pokaż ukryte intencje (" + strconv.FormatInt(hidden, 10) + ")",
			Checked:  showHidden,
			Disabled: hidden == 0,
		}},
		Save: &previewSave{Action: "/print/year/save", Hidden: saveFields},
	}
	if path := query.Get("saved"); path != "" {
		view.Message = "Zapisano plik: " + path
	}

	renderPreview(w, view)
}

func (h *handler) yearPDF(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	year, ok := h.printYear(w, query)
	if !ok {
		return
	}

	buf, err := h.yearPDFBytes(year, query.Get("hidden") == "1")
	if err != nil {
		log.Printf("error generating the pdf of year %d: %v", year, err)
		serverError(w)
		return
	}

	writePDFResponse(w, buf, yearFilename(year))
}

func (h *handler) saveYearPDF(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}
	year, ok := h.printYear(w, r.PostForm)
	if !ok {
		return
	}
	showHidden := r.PostForm.Get("hidden") == "1"

	buf, err := h.yearPDFBytes(year, showHidden)
	if err != nil {
		log.Printf("error generating the pdf of year %d: %v", year, err)
		serverError(w)
		return
	}

	if h.saveFile == nil {
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", `attachment; filename="`+yearFilename(year)+`"`)
		_, err = buf.WriteTo(w)
		if err != nil {
			log.Printf("error writing response: %v", err)
		}
		return
	}

	back := url.Values{"year": {strconv.Itoa(year)}, "week": {r.PostForm.Get("week")}}
	if showHidden {
		back.Set("hidden", "1")
	}

	path, err := h.saveFile(yearFilename(year))
	if err != nil {
		log.Printf("error choosing the year pdf file: %v", err)
		serverError(w)
		return
	}
	if path == "" {
		http.Redirect(w, r, "/print/year?"+back.Encode(), http.StatusSeeOther)
		return
	}

	err = safefile.Write(path, buf.Bytes(), 0o644)
	if err != nil {
		log.Printf("error saving the pdf of year %d to %s: %v", year, path, err)
		serverError(w)
		return
	}

	back.Set("saved", path)
	http.Redirect(w, r, "/print/year?"+back.Encode(), http.StatusSeeOther)
}
