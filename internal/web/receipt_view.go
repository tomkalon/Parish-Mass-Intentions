package web

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

func receiptURL(id int) string {
	return "/intentions/" + strconv.Itoa(id) + "/receipt"
}

func newReceiptLines(intention intentions.Intention, series *gregorian.Summary, showAmount bool) []receiptLine {
	at := intention.MassAt.In(intentions.TimeZone)
	content := intention.Content
	amount := intention.Amount

	var lines []receiptLine
	if series != nil && len(series.Masses) > 0 {
		first := series.Masses[0].MassAt.In(intentions.TimeZone)
		content = series.Series.Content
		amount = series.Amount()
		lines = append(lines, receiptLine{Label: "Msze gregoriańskie:", Value: strconv.Itoa(gregorian.Length) + " Mszy, od " + longDate(first) + ", godz. " + clockLabel(first)})
	} else {
		lines = append(lines, receiptLine{Label: "Msza Święta:", Value: massInWords(at)})
	}
	if intention.IsOutsideParish {
		lines = append(lines, receiptLine{Value: "Msza zostanie odprawiona poza parafią.", Italic: true})
	}

	lines = append(lines, receiptLine{Label: "Intencja:", Value: printedContent(intentionView{Content: content, IsDeceased: intention.IsDeceased})})

	if showAmount && amount > 0 {
		lines = append(lines, receiptLine{Label: "Ofiara:", Value: amount.String()})
	}
	if day, err := time.ParseInLocation(intentions.DateLayout, intention.AcceptedOn, intentions.TimeZone); err == nil {
		lines = append(lines, receiptLine{Label: "Przyjęto dnia:", Value: longDate(day)})
	}

	return lines
}

type receiptData struct {
	intention  intentions.Intention
	series     *gregorian.Summary
	amount     intentions.Grosze
	showAmount bool
}

func (h *handler) loadReceipt(w http.ResponseWriter, r *http.Request) (receiptData, bool) {
	intention, ok := h.findIntention(w, chi.URLParam(r, "id"))
	if !ok {
		return receiptData{}, false
	}
	if intention.IsPlaceholder {
		http.Error(w, "nie znaleziono intencji", http.StatusNotFound)
		return receiptData{}, false
	}

	query := r.URL.Query()
	data := receiptData{
		intention:  intention,
		amount:     intention.Amount,
		showAmount: query.Get("set") != "1" || query.Get("amount") == "1",
	}

	if intention.SeriesID > 0 {
		summary, err := gregorian.Load(h.db, intention.SeriesID, h.now())
		switch {
		case errors.Is(err, gregorian.ErrSeriesNotFound):
		case err != nil:
			log.Printf("error loading gregorian series %d: %v", intention.SeriesID, err)
			serverError(w)
			return receiptData{}, false
		default:
			data.series = &summary
			data.amount = summary.Amount()
		}
	}

	return data, true
}

func (h *handler) receiptPage(w http.ResponseWriter, r *http.Request) {
	data, ok := h.loadReceipt(w, r)
	if !ok {
		return
	}

	id := data.intention.ID
	week := intentions.WeekStart(data.intention.MassAt).Format(intentions.DateLayout)
	pdfQuery := url.Values{"set": {"1"}}
	if data.showAmount {
		pdfQuery.Set("amount", "1")
	}

	amountLabel := "Pokaż ofiarę"
	if data.amount > 0 {
		amountLabel += " (" + data.amount.String() + ")"
	} else {
		amountLabel += " (brak kwoty)"
	}

	renderPreview(w, previewView{
		Title:     "Potwierdzenie przyjęcia intencji",
		BackURL:   editURL(week, id),
		BackLabel: "Wróć do intencji",
		Note:      "POTWIERDZENIE DLA WIERNEGO · górna połowa A4, do odcięcia",
		PDFURL:    receiptURL(id) + ".pdf?" + pdfQuery.Encode(),
		Action:    receiptURL(id),
		Hidden:    []previewField{{Name: "set", Value: "1"}},
		Checks: []previewCheck{{
			Name:     "amount",
			Label:    amountLabel,
			Checked:  data.showAmount && data.amount > 0,
			Disabled: data.amount == 0,
		}},
	})
}

func (h *handler) receiptPDF(w http.ResponseWriter, r *http.Request) {
	data, ok := h.loadReceipt(w, r)
	if !ok {
		return
	}

	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
		serverError(w)
		return
	}

	view := receiptView{
		ParishName: setting.ParishName,
		Address:    setting.Address,
		Lines:      newReceiptLines(data.intention, data.series, data.showAmount),
	}

	var buf bytes.Buffer
	err = writeReceiptPDF(&buf, view)
	if err != nil {
		log.Printf("error generating the receipt for intention %d: %v", data.intention.ID, err)
		serverError(w)
		return
	}

	day := data.intention.MassAt.In(intentions.TimeZone).Format(intentions.DateLayout)
	writePDFResponse(w, &buf, "potwierdzenie-intencji-"+day+".pdf")
}
