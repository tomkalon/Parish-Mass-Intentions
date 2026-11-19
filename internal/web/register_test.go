package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

func registerIntention(t *testing.T, massAt, content string) intentions.Intention {
	t.Helper()

	at, err := time.ParseInLocation(intentions.MassAtLayout, massAt, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing %q: %v", massAt, err)
	}

	return intentions.Intention{MassAt: at, Content: content}
}

func TestNewRegisterView(t *testing.T) {
	first := registerIntention(t, "2026-12-03 18:00", "Jan Kowalski")
	first.ID = 1
	first.IsDeceased = true
	first.AcceptedOn = "2026-11-19"
	first.Amount = 10000

	outside := registerIntention(t, "2026-12-04 07:00", "Anna Nowak")
	outside.ID = 2
	outside.IsOutsideParish = true

	opening := registerIntention(t, "2026-12-05 06:30", "+ Piotr")
	opening.ID = 3
	opening.IsGregorian, opening.IsDeceased, opening.SeriesID = true, true, 7
	opening.Amount = 300000

	next := registerIntention(t, "2026-12-06 06:30", "+ Piotr")
	next.ID = 4
	next.IsGregorian, next.IsDeceased, next.SeriesID = true, true, 7

	view := newRegisterView("grudzień 2026", []intentions.Intention{first, outside, opening, next}, map[int]int{3: 1, 4: 2})

	want := []registerRow{
		{Number: 1, Accepted: "19.11.2026", Mass: "Cz 3.12.2026, 18:00", Content: "+ Jan Kowalski", Amount: "100 zł"},
		{Number: 2, Accepted: "—", Mass: "Pt 4.12.2026, 7:00", Content: "Anna Nowak", Outside: true, Amount: "—"},
		{Number: 3, Accepted: "—", Mass: "So 5.12.2026, 6:30", Content: "+ Piotr (greg. 1/30)", Amount: "3 000 zł"},
		{Number: 4, Accepted: "—", Mass: "Nd 6.12.2026, 6:30", Content: "+ Piotr (greg. 2/30)", Amount: "w serii", InSeries: true},
	}
	if !slices.Equal(view.Rows, want) {
		t.Errorf("rows = %+v,\nwant %+v", view.Rows, want)
	}
	if got, want := view.Summary(), "Razem: 4 intencje (w tym 1 poza parafią) · ofiary: 3 100 zł."; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestRegisterSummaryWithoutIntentions(t *testing.T) {
	if got, want := (registerView{}).Summary(), "Brak intencji w tym okresie."; got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

func TestLayoutRegisterPaginates(t *testing.T) {
	view := registerView{Period: "rok 2026", ParishName: "Parafia św. Jana"}
	for i := range 80 {
		view.Rows = append(view.Rows, registerRow{Number: i + 1, Accepted: "—", Mass: "Pn 2.11.2026, 18:00", Content: strings.Repeat("intencja ", 1+i%12), Amount: "—"})
	}

	pages := layoutRegister(view, fakeMeasurer{})

	if len(pages) < 3 {
		t.Fatalf("got %d pages, want at least 3 for 80 rows", len(pages))
	}

	footerTop := registerHeight - registerBottom - registerFootSize*1.4
	numbers := 0
	for i, page := range pages {
		texts := pageTexts(page)
		if !slices.Contains(texts, "Podpis celebransa") {
			t.Errorf("page %d has no table header", i+1)
		}
		if label := "str. " + strconv.Itoa(i+1) + " z " + strconv.Itoa(len(pages)); !slices.Contains(texts, label) {
			t.Errorf("page %d has no %q", i+1, label)
		}
		if got := slices.Contains(texts, "KSIĘGA INTENCJI MSZALNYCH"); got != (i == 0) {
			t.Errorf("page %d: big title shown = %v, want %v", i+1, got, i == 0)
		}
		for _, run := range page.Runs {
			if strings.HasSuffix(run.Text, ".") && run.Font == fontRegular && run.X < registerSide+30 && run.Baseline < footerTop {
				numbers++
			}
			if run.Baseline > registerHeight-registerBottom {
				t.Errorf("page %d: %q below the bottom margin (%.1f)", i+1, run.Text, run.Baseline)
			}
		}
	}
	if numbers != 80 {
		t.Errorf("printed %d row numbers, want 80", numbers)
	}

	last := pageTexts(pages[len(pages)-1])
	if !slices.Contains(last, "Razem: 80 intencji.") {
		t.Errorf("last page has no summary, texts: %q", last)
	}
}

func TestLayoutRegisterEmpty(t *testing.T) {
	pages := layoutRegister(registerView{Period: "listopad 2026"}, fakeMeasurer{})

	if len(pages) != 1 {
		t.Fatalf("got %d pages, want 1", len(pages))
	}
	texts := pageTexts(pages[0])
	for _, want := range []string{"listopad 2026", "Lp.", "Brak intencji w tym okresie.", "str. 1 z 1"} {
		if !slices.Contains(texts, want) {
			t.Errorf("page has no %q", want)
		}
	}
}

func getRegister(t *testing.T, h *handler, target string, pdf bool) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, target, nil)
	rec := httptest.NewRecorder()
	if pdf {
		h.registerPDF(rec, req)
	} else {
		h.registerPage(rec, req)
	}

	return rec
}

func TestRegisterPDF(t *testing.T) {
	tests := []struct {
		name         string
		target       string
		wantFilename string
	}{
		{name: "week", target: "/register.pdf?week=2026-11-11&payments=week", wantFilename: "ksiega-intencji-2026-11-09.pdf"},
		{name: "month", target: "/register.pdf?week=2026-11-09&payments=month&month=2026-12", wantFilename: "ksiega-intencji-2026-12.pdf"},
		{name: "year", target: "/register.pdf?week=2026-11-09&payments=year&year=2026", wantFilename: "ksiega-intencji-2026.pdf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			seedIntention(t, h.db, "2026-11-10 18:00", false, "+ Anna Kutyba")
			seedIntention(t, h.db, "2026-12-10 18:00", true, "Jan Nowak")

			rec := getRegister(t, h, tt.target, true)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d, body %q", rec.Code, http.StatusOK, rec.Body.String())
			}
			if got, want := rec.Header().Get("Content-Type"), "application/pdf"; got != want {
				t.Errorf("Content-Type = %q, want %q", got, want)
			}
			if got, want := rec.Header().Get("Content-Disposition"), `inline; filename="`+tt.wantFilename+`"`; got != want {
				t.Errorf("Content-Disposition = %q, want %q", got, want)
			}
			if !strings.HasPrefix(rec.Body.String(), "%PDF-") {
				t.Errorf("body does not start with %%PDF-")
			}
		})
	}
}

func TestRegisterRejectsInvalidPeriod(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	for _, target := range []string{
		"/register.pdf?week=2026-11-09",
		"/register.pdf?week=2026-11-09&payments=month&month=13",
		"/register.pdf?week=zly&payments=week",
		"/register?week=2026-11-09&payments=day",
	} {
		rec := getRegister(t, h, target, strings.HasPrefix(target, "/register.pdf"))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestRegisterPreviewPage(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := getRegister(t, h, "/register?week=2026-11-09&payments=month&month=2026-12", false)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<title>Księga intencji – grudzień 2026 – Go Parafia</title>`,
		`<a class="button" href="/?week=2026-11-09&amp;payments=month&amp;month=2026-12" data-close>← Wróć do wpłat</a>`,
		`KSIĘGA INTENCJI MSZALNYCH – grudzień 2026 · A4 poziomo`,
		`<iframe class="print-preview" src="/register.pdf?week=2026-11-09&amp;payments=month&amp;month=2026-12"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func TestPaymentsDialogLinksToRegister(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	for target, want := range map[string]string{
		"/?week=2026-11-09&payments=week":                `href="/register?week=2026-11-09&amp;payments=week"`,
		"/?week=2026-11-09&payments=month&month=2026-12": `href="/register?week=2026-11-09&amp;payments=month&amp;month=2026-12"`,
		"/?week=2026-11-09&payments=year":                `href="/register?week=2026-11-09&amp;payments=year&amp;year=2026"`,
	} {
		body := getWeek(t, h, target).Body.String()
		if !strings.Contains(body, want) {
			t.Errorf("%s: body does not contain %q", target, want)
		}
	}
}

func acceptedOf(t *testing.T, h *handler, id int) string {
	t.Helper()

	var intention intentions.Intention
	err := h.db.First(&intention, id).Error
	if err != nil {
		t.Fatalf("loading intention %d: %v", id, err)
	}

	return intention.AcceptedOn
}

func lastIntentionID(t *testing.T, h *handler) int {
	t.Helper()

	var intention intentions.Intention
	err := h.db.Order("id DESC").First(&intention).Error
	if err != nil {
		t.Fatalf("loading the last intention: %v", err)
	}

	return intention.ID
}

func TestCreateIntentionSetsAcceptedDate(t *testing.T) {
	now := time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone)

	t.Run("today by default", func(t *testing.T) {
		h := newTestHandler(t, now)
		rec := postIntention(t, h, validForm())
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		if got := acceptedOf(t, h, lastIntentionID(t, h)); got != "2026-11-19" {
			t.Errorf("accepted = %q, want 2026-11-19", got)
		}
	})

	t.Run("chosen date", func(t *testing.T) {
		h := newTestHandler(t, now)
		form := validForm()
		form.Set("accepted", "2026-11-02")
		rec := postIntention(t, h, form)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
		}
		if got := acceptedOf(t, h, lastIntentionID(t, h)); got != "2026-11-02" {
			t.Errorf("accepted = %q, want 2026-11-02", got)
		}
	})

	for name, tc := range map[string]struct {
		value string
		want  string
	}{
		"future date":  {value: "2026-11-20", want: "Data przyjęcia nie może być późniejsza niż dzisiaj."},
		"wrong format": {value: "19.11.2026", want: "Nieprawidłowy format, oczekiwano RRRR-MM-DD."},
	} {
		t.Run(name, func(t *testing.T) {
			h := newTestHandler(t, now)
			form := validForm()
			form.Set("accepted", tc.value)
			rec := postIntention(t, h, form)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			body := rec.Body.String()
			if !strings.Contains(body, `<p class="field-error" id="accepted-error">`+tc.want+`</p>`) {
				t.Errorf("body has no error %q", tc.want)
			}
			if countIntentions(t, h.db) != 0 {
				t.Error("intention saved despite the error")
			}
		})
	}
}

func postEdit(t *testing.T, h *handler, id int, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/intentions/"+strconv.Itoa(id), strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.Itoa(id))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.update(rec, req)

	return rec
}

func TestUpdateIntentionKeepsAcceptedDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	legacy := newIntention(t, 0, "2026-11-03 18:00", false, "Stara intencja")
	accepted := newIntention(t, 0, "2026-11-03 18:00", false, "Przyjęta")
	accepted.AcceptedOn = "2026-10-01"
	placeholder := newIntention(t, 0, "2026-11-04 18:00", false, "")
	placeholder.IsPlaceholder = true
	for _, intention := range []*intentions.Intention{&legacy, &accepted, &placeholder} {
		err := h.db.Create(intention).Error
		if err != nil {
			t.Fatalf("creating intention: %v", err)
		}
	}

	for _, tc := range []struct {
		id   int
		day  string
		want string
	}{
		{id: legacy.ID, day: "2026-11-03", want: ""},
		{id: accepted.ID, day: "2026-11-03", want: "2026-10-01"},
		{id: placeholder.ID, day: "2026-11-04", want: "2026-11-19"},
	} {
		form := validForm()
		form.Set("day", tc.day)
		form.Set("content", "Nowa treść")
		rec := postEdit(t, h, tc.id, form)
		if rec.Code != http.StatusSeeOther {
			t.Fatalf("intention %d: status = %d, want %d", tc.id, rec.Code, http.StatusSeeOther)
		}
		if got := acceptedOf(t, h, tc.id); got != tc.want {
			t.Errorf("intention %d: accepted = %q, want %q", tc.id, got, tc.want)
		}
	}
}

func TestGregorianSeriesSharesAcceptedDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	form := url.Values{
		"week":      {"2026-11-23"},
		"day":       {"2026-11-23"},
		"time":      {"06:30"},
		"content":   {"+ Piotr Wiśniewski"},
		"gregorian": {"on"},
		"accepted":  {"2026-11-18"},
	}
	rec := postIntention(t, h, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	firstID := lastIntentionID(t, h)
	var first intentions.Intention
	h.db.First(&first, firstID)

	next := url.Values{
		"week":     {"2026-11-23"},
		"day":      {"2026-11-24"},
		"time":     {"06:30"},
		"series":   {strconv.Itoa(first.SeriesID)},
		"accepted": {"2026-11-19"},
	}
	rec = postIntention(t, h, next)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("next mass: status = %d, want %d, body %q", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	nextID := lastIntentionID(t, h)
	if got := acceptedOf(t, h, nextID); got != "2026-11-18" {
		t.Errorf("next mass accepted = %q, want the series date 2026-11-18", got)
	}

	edit := url.Values{
		"week":     {"2026-11-23"},
		"day":      {"2026-11-23"},
		"time":     {"06:30"},
		"content":  {"+ Piotr Wiśniewski"},
		"accepted": {"2026-11-17"},
	}
	rec = postEdit(t, h, firstID, edit)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("editing 1/30: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	for _, id := range []int{firstID, nextID} {
		if got := acceptedOf(t, h, id); got != "2026-11-17" {
			t.Errorf("intention %d accepted = %q, want 2026-11-17 after editing 1/30", id, got)
		}
	}

	summary, err := gregorian.Load(h.db, first.SeriesID, h.now())
	if err != nil {
		t.Fatalf("loading series: %v", err)
	}
	if got := summary.Masses[0].AcceptedOn; got != "2026-11-17" {
		t.Errorf("series mass accepted = %q, want 2026-11-17", got)
	}
}

func TestPanelShowsAcceptedDate(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 19, 10, 0, 0, 0, intentions.TimeZone))
	accepted := newIntention(t, 0, "2026-11-24 18:00", false, "Przyjęta")
	accepted.AcceptedOn = "2026-10-01"
	h.db.Create(&accepted)

	body := getWeek(t, h, "/?week=2026-11-23&new=1").Body.String()
	if want := `value="2026-11-19" min="2000-01-01" max="2026-11-19" aria-label="Data przyjęcia"`; !strings.Contains(body, want) {
		t.Errorf("new form does not contain %q", want)
	}
	if want := `data-date-picker="19.11.2026"`; !strings.Contains(body, want) {
		t.Errorf("new form does not contain %q", want)
	}

	body = getWeek(t, h, "/?week=2026-11-23&edit="+strconv.Itoa(accepted.ID)).Body.String()
	if want := `data-date-picker="1.10.2026"`; !strings.Contains(body, want) {
		t.Errorf("edit form does not contain %q", want)
	}
}
