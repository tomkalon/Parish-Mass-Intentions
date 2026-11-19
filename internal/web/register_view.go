package web

import (
	"bytes"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

func (p paymentsQuery) period() string {
	switch p.tab {
	case paymentsMonth:
		return p.month.Format(monthLayout)
	case paymentsYear:
		return strconv.Itoa(p.year)
	default:
		return ""
	}
}

func (p paymentsQuery) label(start time.Time) string {
	switch p.tab {
	case paymentsMonth:
		return strings.ToLower(monthNames[p.month.Month()-1]) + " " + strconv.Itoa(p.month.Year())
	case paymentsYear:
		return "rok " + strconv.Itoa(p.year)
	default:
		return weekInWords(start)
	}
}

func (p paymentsQuery) fileSuffix(start time.Time) string {
	switch p.tab {
	case paymentsMonth, paymentsYear:
		return p.period()
	default:
		return start.Format(intentions.DateLayout)
	}
}

func registerURL(week, tab, period string) string {
	return "/register" + strings.TrimPrefix(paymentsURL(week, tab, period), "/")
}

func (h *handler) registerPeriod(w http.ResponseWriter, query url.Values) (time.Time, paymentsQuery, bool) {
	start, err := h.weekStart(query)
	if err != nil {
		http.Error(w, "nieprawidłowa data tygodnia, oczekiwano RRRR-MM-DD", http.StatusBadRequest)
		return time.Time{}, paymentsQuery{}, false
	}

	p, err := parsePaymentsQuery(query, start)
	if err != nil {
		http.Error(w, "nieprawidłowy okres księgi, oczekiwano payments=week|month|year, month=RRRR-MM, year=RRRR", http.StatusBadRequest)
		return time.Time{}, paymentsQuery{}, false
	}

	return start, p, true
}

func (h *handler) registerPage(w http.ResponseWriter, r *http.Request) {
	start, p, ok := h.registerPeriod(w, r.URL.Query())
	if !ok {
		return
	}

	week := start.Format(intentions.DateLayout)
	label := p.label(start)
	renderPreview(w, previewView{
		Title:     "Księga intencji – " + label,
		BackURL:   paymentsURL(week, p.tab, p.period()),
		BackLabel: "Wróć do wpłat",
		Note:      "KSIĘGA INTENCJI MSZALNYCH – " + label + " · A4 poziomo",
		PDFURL:    "/register.pdf" + strings.TrimPrefix(paymentsURL(week, p.tab, p.period()), "/"),
	})
}

func (h *handler) registerPDF(w http.ResponseWriter, r *http.Request) {
	start, p, ok := h.registerPeriod(w, r.URL.Query())
	if !ok {
		return
	}

	from, to := p.span(start)
	list, err := intentions.ForRange(h.db, from, to)
	if err != nil {
		log.Printf("error loading intentions for the register: %v", err)
		serverError(w)
		return
	}

	steps, err := gregorian.Steps(h.db, seriesIDs(list))
	if err != nil {
		log.Printf("error loading gregorian series steps: %v", err)
		serverError(w)
		return
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		serverError(w)
		return
	}

	view := newRegisterView(p.label(start), list, steps)
	view.ParishName = setting.ParishName
	view.Address = setting.Address

	var buf bytes.Buffer
	err = writeRegisterPDF(&buf, view)
	if err != nil {
		log.Printf("error generating the register pdf: %v", err)
		serverError(w)
		return
	}

	writePDFResponse(w, &buf, "ksiega-intencji-"+p.fileSuffix(start)+".pdf")
}
