package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/backup"
	"go-parafia/internal/intentions"
)

var draftNow = time.Date(2026, 11, 17, 14, 32, 0, 0, intentions.TimeZone)

func draftValues(token string) url.Values {
	return url.Values{
		"draft_token": {token},
		"week":        {"2026-11-30"},
		"day":         {"2026-12-01"},
		"time":        {"18:00"},
		"content":     {"+ Jan Kowalski"},
		"deceased":    {"on"},
		"amount":      {"100"},
	}
}

func postDraft(t *testing.T, h *handler, form url.Values) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/draft", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.saveDraft(rec, req)

	if rec.Code != http.StatusNoContent && rec.Code != http.StatusBadRequest {
		t.Fatalf("POST /draft status = %d", rec.Code)
	}

	return rec
}

func postDiscard(t *testing.T, h *handler, form url.Values, referer string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/draft/discard", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	rec := httptest.NewRecorder()
	h.discardDraft(rec, req)

	return rec
}

func getStart(t *testing.T, h *handler) string {
	t.Helper()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.home(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET / status = %d", rec.Code)
	}

	return rec.Body.String()
}

func hasDraft(t *testing.T, h *handler) bool {
	t.Helper()

	_, ok, err := h.drafts.Load()
	if err != nil {
		t.Fatalf("loading draft: %v", err)
	}

	return ok
}

func TestSaveDraftShowsBannerOnStartPage(t *testing.T) {
	h := newTestHandler(t, draftNow)

	body := getStart(t, h)
	if strings.Contains(body, "draftbar") {
		t.Fatal("start page shows a draft banner without a draft")
	}

	rec := postDraft(t, h, draftValues("tok-1"))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	body = getStart(t, h)
	checkBody(t, body,
		`<div class="draftbar" role="alert">`,
		`Niezapisana intencja`,
		`Wpisano dziś, 14:32, ale nie kliknięto „Zapisz”.`,
		"„&#43; Jan Kowalski”<span class=\"draftbar-when\">Wtorek 1.12, godz. 18:00 · wpłata 100\u00a0zł</span></p>",
		`href="/?week=2026-11-30&amp;draft=1">Przywróć</a>`,
		`<form class="draftbar-actions" method="post" action="/draft/discard">`,
	)
}

func TestSaveDraftRejectsMissingOrLongToken(t *testing.T) {
	h := newTestHandler(t, draftNow)

	for _, token := range []string{"", strings.Repeat("x", maxDraftToken+1)} {
		rec := postDraft(t, h, draftValues(token))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("token of %d chars: status = %d, want %d", len(token), rec.Code, http.StatusBadRequest)
		}
	}
	if hasDraft(t, h) {
		t.Error("draft stored without a valid token")
	}
}

func TestSaveDraftRejectsHugeBody(t *testing.T) {
	h := newTestHandler(t, draftNow)
	form := draftValues("tok-1")
	form.Set("content", strings.Repeat("a", maxDraftBody))

	rec := postDraft(t, h, form)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestDraftBannerLongContentIsShortened(t *testing.T) {
	h := newTestHandler(t, draftNow)
	form := draftValues("tok-1")
	form.Set("content", strings.Repeat("ą", 400))
	postDraft(t, h, form)

	body := getStart(t, h)
	checkBody(t, body, strings.Repeat("ą", draftPreviewSize-1)+"…")
	if strings.Contains(body, strings.Repeat("ą", draftPreviewSize)) {
		t.Error("banner shows the whole long content")
	}
}

func TestWeekShowsDraftBannerOrPanelNotice(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	rec := getWeek(t, h, "/?week=2026-11-23")
	checkBody(t, rec.Body.String(), `<div class="draftbar" role="alert">`)

	rec = getWeek(t, h, "/?week=2026-11-23&new=1")
	body := rec.Body.String()
	if strings.Contains(body, `class="draftbar"`) {
		t.Error("banner shown over an open panel")
	}
	checkBody(t, body,
		`<p class="form-other-draft" role="status">Jest niezapisana intencja z dziś, 14:32: „&#43; Jan Kowalski”. <a href="/?week=2026-11-30&amp;draft=1">Przywróć ją</a>`,
	)

	rec = getWeek(t, h, "/?week=2026-11-23&calendar=2026")
	if strings.Contains(rec.Body.String(), `class="draftbar"`) {
		t.Error("banner shown over an open dialog")
	}
}

func TestSettingsShowDraftBanner(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	rec := getSettings(t, h, "/settings")
	checkBody(t, rec.Body.String(), `<div class="draftbar" role="alert">`)
}

func TestRestoreDraftOpensFilledPanel(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	rec := getWeek(t, h, "/?week=2026-11-30&draft=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := rec.Body.String()
	checkBody(t, body,
		`<form class="panel-form" method="post" action="/intentions" data-new data-draft-token="tok-1">`,
		`<p class="form-restored" role="status">Przywrócono niezapisaną intencję z dziś, 14:32. Sprawdź ją i kliknij „Zapisz”.</p>`,
		`value="2026-12-01" checked`,
		`<option value="18:00" selected>`,
		` Jan Kowalski</textarea>`,
		`name="deceased" checked`,
		`name="amount" value="100"`,
	)
	if strings.Contains(body, "form-other-draft") || strings.Contains(body, `class="draftbar"`) {
		t.Error("restored draft is also offered as another draft")
	}
}

func TestRestoreDraftOfEdit(t *testing.T) {
	h := newTestHandler(t, draftNow)
	seedIntention(t, h.db, "2026-12-01 06:30", false, "+ Anna Nowak")
	form := draftValues("tok-1")
	form.Set("edit", "1")
	postDraft(t, h, form)

	body := getStart(t, h)
	checkBody(t, body, `Niezapisana zmiana intencji`)

	rec := getWeek(t, h, "/?week=2026-11-30&draft=1")
	checkBody(t, rec.Body.String(),
		`<form class="panel-form" method="post" action="/intentions/1" data-edit="1" data-draft-token="tok-1">`,
		` Jan Kowalski</textarea>`,
	)

	err := h.db.Delete(&intentions.Intention{}, 1).Error
	if err != nil {
		t.Fatal(err)
	}

	rec = getWeek(t, h, "/?week=2026-11-30&draft=1")
	checkBody(t, rec.Body.String(),
		`<form class="panel-form" method="post" action="/intentions" data-new data-draft-token="tok-1">`,
		`Zmieniana intencja została w międzyczasie usunięta, więc zapisze się jako nowa.`,
	)
}

func TestRestoreWithoutDraftRedirectsToWeek(t *testing.T) {
	h := newTestHandler(t, draftNow)

	rec := getWeek(t, h, "/?week=2026-11-30&draft=1")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-30"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
}

func TestSavingIntentionFinishesItsDraft(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	form := validForm()
	form.Set("draft_token", "tok-1")
	rec := postIntention(t, h, form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if hasDraft(t, h) {
		t.Fatal("draft kept after its intention was saved")
	}

	postDraft(t, h, draftValues("tok-1"))
	if hasDraft(t, h) {
		t.Error("late autosave brought back the draft of a saved intention")
	}
}

func TestSavingAnotherIntentionKeepsDraft(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	form := validForm()
	form.Set("draft_token", "tok-2")
	postIntention(t, h, form)

	if !hasDraft(t, h) {
		t.Error("saving a different panel removed the draft")
	}
}

func TestInvalidSaveKeepsDraftToken(t *testing.T) {
	h := newTestHandler(t, draftNow)
	form := validForm()
	form.Set("content", "")
	form.Set("draft_token", "tok-1")
	postDraft(t, h, url.Values{"draft_token": {"tok-1"}, "week": {"2026-11-02"}, "amount": {"50"}})

	rec := postIntention(t, h, form)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}

	body := rec.Body.String()
	checkBody(t, body, `data-draft-token="tok-1"`)
	if strings.Contains(body, "form-other-draft") {
		t.Error("the panel's own draft is shown as another draft")
	}
	if !hasDraft(t, h) {
		t.Error("draft removed by a failed save")
	}
}

func TestDiscardDraftFromPanel(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	rec := postDiscard(t, h, url.Values{"draft_token": {"tok-2"}}, "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if !hasDraft(t, h) {
		t.Fatal("closing another panel discarded the draft")
	}

	postDiscard(t, h, url.Values{"draft_token": {"tok-1"}}, "")
	if hasDraft(t, h) {
		t.Error("closing the panel did not discard its draft")
	}
}

func TestDiscardDraftFromBanner(t *testing.T) {
	h := newTestHandler(t, draftNow)
	postDraft(t, h, draftValues("tok-1"))

	rec := postDiscard(t, h, url.Values{}, "http://example.com/settings?week=2026-11-30")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/settings?week=2026-11-30"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	if hasDraft(t, h) {
		t.Error("draft kept after Odrzuć")
	}
}

func TestDeletingIntentionDiscardsItsDraft(t *testing.T) {
	h := newTestHandler(t, draftNow)
	seedIntention(t, h.db, "2026-12-01 06:30", false, "+ Anna Nowak")
	form := draftValues("tok-1")
	form.Set("edit", "1")
	postDraft(t, h, form)

	router := chi.NewRouter()
	router.Post("/intentions/{id}/delete", h.delete)
	req := httptest.NewRequest(http.MethodPost, "/intentions/1/delete", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if hasDraft(t, h) {
		t.Error("draft of a deleted intention is still offered")
	}
}

func TestDraftRoutesDoNotMarkDataAsChanged(t *testing.T) {
	h := newTestHandler(t, draftNow)
	backups, err := backup.Load(h.db, filepath.Join(t.TempDir(), "kopia.json"))
	if err != nil {
		t.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterRoutes(router, h.db, Options{DataDir: t.TempDir(), Backups: backups})

	for _, path := range []string{"/draft", "/draft/discard"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(draftValues("tok-1").Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNoContent {
			t.Errorf("POST %s status = %d, want %d", path, rec.Code, http.StatusNoContent)
		}
	}

	if !backups.State().LastChange.IsZero() {
		t.Error("draft autosave marked the data as changed")
	}
}

func TestStartPageShowsVersion(t *testing.T) {
	h := newTestHandler(t, draftNow)

	checkBody(t, getStart(t, h), `<p class="start-credit">autor: Tomasz Kaliński, Rzezawa</p>`)

	h.version = "1.7.1"
	checkBody(t, getStart(t, h), `<p class="start-credit">wersja 1.7.1 · autor: Tomasz Kaliński, Rzezawa</p>`)
}

func TestStartPageShowsDatabaseWarning(t *testing.T) {
	h := newTestHandler(t, draftNow)
	h.dbWarning = "Sprawdzenie bazy danych wykryło błąd."

	checkBody(t, getStart(t, h), `<p class="start-warning" role="alert">Sprawdzenie bazy danych wykryło błąd.</p>`)
}

func TestDraftWhen(t *testing.T) {
	tests := []struct {
		day, time, amount string
		want              string
	}{
		{"2026-12-01", "18:00", "100", "Wtorek 1.12, godz. 18:00 · wpłata 100\u00a0zł"},
		{"2026-12-06", "06:30", "", "Niedziela 6.12, godz. 6:30"},
		{"2026-12-06", "", "", "Niedziela 6.12"},
		{"", "18:00", "abc", ""},
	}
	for _, tt := range tests {
		d := draftValues("t")
		d.Set("day", tt.day)
		d.Set("time", tt.time)
		d.Set("amount", tt.amount)
		got, _ := draftFromForm(d)
		if when := draftWhen(got); !strings.HasPrefix(when, tt.want) || (tt.want == "" && when != "") {
			t.Errorf("draftWhen(%s %s %s) = %q, want prefix %q", tt.day, tt.time, tt.amount, when, tt.want)
		}
	}
}

func TestDraftFromForm(t *testing.T) {
	form := draftValues("tok-1")
	form.Set("edit", "12")
	form.Set("series", "-3")
	form.Set("hidden", "on")

	d, ok := draftFromForm(form)
	if !ok {
		t.Fatal("valid form rejected")
	}
	if d.EditID != 12 || d.SeriesID != 0 || !d.Hidden || !d.Deceased || d.Outside || d.Amount != "100" {
		t.Errorf("draftFromForm = %+v", d)
	}
	if strconv.Itoa(d.EditID) != "12" {
		t.Error("edit id not parsed")
	}
}
