package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

func TestNewReceiptLines(t *testing.T) {
	intention := registerIntention(t, "2026-12-03 18:00", "Jan Kowalski")
	intention.IsDeceased = true
	intention.Amount = 10000
	intention.AcceptedOn = "2026-11-19"

	got := newReceiptLines(intention, nil, true)
	want := []receiptLine{
		{Label: "Msza Święta:", Value: "czwartek, 3 grudnia 2026 r., godz. 18:00"},
		{Label: "Intencja:", Value: "+ Jan Kowalski"},
		{Label: "Ofiara:", Value: "100 zł"},
		{Label: "Przyjęto dnia:", Value: "19 listopada 2026 r."},
	}
	if !slices.Equal(got, want) {
		t.Errorf("lines = %+v,\nwant %+v", got, want)
	}

	intention.IsOutsideParish = true
	intention.AcceptedOn = ""
	got = newReceiptLines(intention, nil, false)
	want = []receiptLine{
		{Label: "Msza Święta:", Value: "czwartek, 3 grudnia 2026 r., godz. 18:00"},
		{Value: "Msza zostanie odprawiona poza parafią.", Italic: true},
		{Label: "Intencja:", Value: "+ Jan Kowalski"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("outside, no amount: lines = %+v,\nwant %+v", got, want)
	}
}

func TestNewReceiptLinesForGregorianSeries(t *testing.T) {
	intention := registerIntention(t, "2026-12-02 07:00", "+ Piotr")
	intention.IsGregorian, intention.IsDeceased, intention.SeriesID = true, true, 3

	first := time.Date(2026, 12, 1, 7, 0, 0, 0, intentions.TimeZone)
	series := &gregorian.Summary{
		Series: gregorian.Series{ID: 3, Content: "+ Piotr Wiśniewski"},
		Masses: []gregorian.Mass{{ID: 1, MassAt: first, Amount: 300000}, {ID: 2, MassAt: first.AddDate(0, 0, 1)}},
	}

	got := newReceiptLines(intention, series, true)
	want := []receiptLine{
		{Label: "Msze gregoriańskie:", Value: "30 Mszy, od 1 grudnia 2026 r., godz. 7:00"},
		{Label: "Intencja:", Value: "+ Piotr Wiśniewski"},
		{Label: "Ofiara:", Value: "3 000 zł"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("lines = %+v,\nwant %+v", got, want)
	}
}

func TestLayoutReceipt(t *testing.T) {
	view := receiptView{
		ParishName: "Parafia św. Jana",
		Address:    "ul. Kościelna 1",
		Lines: []receiptLine{
			{Label: "Msza Święta:", Value: "czwartek, 3 grudnia 2026 r., godz. 18:00"},
			{Label: "Intencja:", Value: strings.Repeat("za zmarłych z rodziny Kowalskich ", 12)},
		},
	}

	page := layoutReceipt(view, fakeMeasurer{})
	texts := pageTexts(page)
	for _, want := range []string{"Parafia św. Jana", "ul. Kościelna 1", "POTWIERDZENIE PRZYJĘCIA INTENCJI MSZALNEJ", "Msza Święta:", "Intencja:", "pieczęć i podpis"} {
		if !slices.Contains(texts, want) {
			t.Errorf("receipt has no %q", want)
		}
	}

	contentLines := 0
	for _, run := range page.Runs {
		if strings.Contains(run.Text, "Kowalskich") {
			contentLines++
			if run.X+(fakeMeasurer{}).width(run.Text, run.Font, run.Size, 0) > marginSide+contentWidth+0.01 {
				t.Errorf("line %q goes past the right margin", run.Text)
			}
		}
	}
	if contentLines < 2 {
		t.Errorf("long intention printed in %d lines, want it wrapped", contentLines)
	}

	var cut *pdfRule
	for i, rule := range page.Rules {
		if rule.Dashed {
			cut = &page.Rules[i]
		}
	}
	if cut == nil {
		t.Fatal("receipt has no dashed cut line")
	}
	if cut.Y < receiptHeight {
		t.Errorf("cut line at %.1f, want at least the middle of the page (%.1f)", cut.Y, receiptHeight)
	}
	for _, run := range page.Runs {
		if run.Baseline > cut.Y {
			t.Errorf("%q printed below the cut line", run.Text)
		}
	}
}

func getReceipt(t *testing.T, h *handler, id int, query string, pdf bool) *httptest.ResponseRecorder {
	t.Helper()

	target := receiptURL(id)
	if pdf {
		target += ".pdf"
	}
	req := httptest.NewRequest(http.MethodGet, target+query, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.Itoa(id))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	if pdf {
		h.receiptPDF(rec, req)
	} else {
		h.receiptPage(rec, req)
	}

	return rec
}

func TestReceiptPDF(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	intention := newIntention(t, 0, "2026-12-03 18:00", false, "+ Jan Kowalski")
	intention.Amount = 10000
	h.db.Create(&intention)

	rec := getReceipt(t, h, intention.ID, "?set=1&amount=1", true)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="potwierdzenie-intencji-2026-12-03.pdf"`; got != want {
		t.Errorf("Content-Disposition = %q, want %q", got, want)
	}
	if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
		t.Error("body does not start with %PDF-")
	}
}

func TestReceiptNotFound(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	placeholder := newIntention(t, 0, "2026-12-03 18:00", false, "")
	placeholder.IsPlaceholder = true
	h.db.Create(&placeholder)

	for _, id := range []int{placeholder.ID, 999} {
		for _, pdf := range []bool{false, true} {
			rec := getReceipt(t, h, id, "", pdf)
			if rec.Code != http.StatusNotFound {
				t.Errorf("id %d, pdf %v: status = %d, want %d", id, pdf, rec.Code, http.StatusNotFound)
			}
		}
	}
}

func TestReceiptPreviewPage(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	paid := newIntention(t, 0, "2026-12-03 18:00", false, "+ Jan Kowalski")
	paid.Amount = 10000
	unpaid := newIntention(t, 0, "2026-12-04 18:00", false, "Anna")
	h.db.Create(&paid)
	h.db.Create(&unpaid)

	tests := []struct {
		name  string
		id    int
		query string
		want  []string
	}{
		{
			name: "amount shown by default",
			id:   paid.ID,
			want: []string{
				`<a class="button" href="/?edit=` + strconv.Itoa(paid.ID) + `&amp;week=2026-11-30" data-close>← Wróć do intencji</a>`,
				`<input type="checkbox" name="amount" value="1" checked data-autosubmit> Pokaż ofiarę (100` + " " + `zł)`,
				`src="/intentions/` + strconv.Itoa(paid.ID) + `/receipt.pdf?amount=1&amp;set=1"`,
			},
		},
		{
			name:  "amount switched off",
			id:    paid.ID,
			query: "?set=1",
			want: []string{
				`<input type="checkbox" name="amount" value="1" data-autosubmit> Pokaż ofiarę`,
				`src="/intentions/` + strconv.Itoa(paid.ID) + `/receipt.pdf?set=1"`,
			},
		},
		{
			name: "no amount to show",
			id:   unpaid.ID,
			want: []string{`<input type="checkbox" name="amount" value="1" disabled data-autosubmit> Pokaż ofiarę (brak kwoty)`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := getReceipt(t, h, tt.id, tt.query, false)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			checkBody(t, rec.Body.String(), tt.want...)
		})
	}
}

func TestPanelLinksToReceipt(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	saved := newIntention(t, 0, "2026-12-03 18:00", false, "+ Jan Kowalski")
	placeholder := newIntention(t, 0, "2026-12-04 18:00", false, "")
	placeholder.IsPlaceholder = true
	h.db.Create(&saved)
	h.db.Create(&placeholder)

	link := `href="/intentions/` + strconv.Itoa(saved.ID) + `/receipt"`
	if body := getWeek(t, h, "/?week=2026-11-30&edit="+strconv.Itoa(saved.ID)).Body.String(); !strings.Contains(body, link) {
		t.Errorf("edit panel has no receipt link %q", link)
	}
	for _, target := range []string{"/?week=2026-11-30&new=1", "/?week=2026-11-30&edit=" + strconv.Itoa(placeholder.ID)} {
		if body := getWeek(t, h, target).Body.String(); strings.Contains(body, "/receipt") {
			t.Errorf("%s: panel links to a receipt", target)
		}
	}
}
