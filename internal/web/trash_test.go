package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/trash"
)

var trashNow = time.Date(2026, 11, 19, 14, 32, 0, 0, intentions.TimeZone)

func deleteByHandler(t *testing.T, h *handler, id int) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/intentions/"+strconv.Itoa(id)+"/delete", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", strconv.Itoa(id))
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.delete(rec, req)

	return rec
}

func postRestore(t *testing.T, h *handler, batch string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(http.MethodPost, "/trash/"+batch+"/restore", nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("batch", batch)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec := httptest.NewRecorder()
	h.restoreTrash(rec, req)

	return rec
}

func batchFrom(t *testing.T, location string) string {
	t.Helper()

	u, err := url.Parse(location)
	if err != nil {
		t.Fatalf("parsing %q: %v", location, err)
	}
	batch := u.Query().Get("deleted")
	if batch == "" {
		t.Fatalf("location %q has no deleted=", location)
	}

	return batch
}

func TestDeleteShowsUndoAndRestores(t *testing.T) {
	h := newTestHandler(t, trashNow)
	intention := newIntention(t, 0, "2026-12-01 18:00", false, "+ Jan Kowalski")
	intention.Amount = 10000
	h.db.Create(&intention)

	rec := deleteByHandler(t, h, intention.ID)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	location := rec.Header().Get("Location")
	batch := batchFrom(t, location)
	if countIntentions(t, h.db) != 0 {
		t.Fatal("intention not deleted")
	}

	body := getWeek(t, h, location).Body.String()
	checkBody(t, body,
		`<form class="week-notice undo-bar" method="post" action="/trash/`+batch+`/restore" role="status">`,
		`<span>Usunięto intencję „&#43; Jan Kowalski” (Wtorek 1.12., godz. 18:00).</span>`,
		`<button class="button" type="submit" autofocus>Cofnij</button>`,
		`<a href="/settings#kosz">Kosz</a>`,
	)

	rec = postRestore(t, h, batch)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("restore status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	id := strconv.Itoa(intention.ID)
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-30&saved="+id+"#i-"+id; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	var restored intentions.Intention
	h.db.First(&restored, intention.ID)
	if restored.Content != "+ Jan Kowalski" || restored.Amount != 10000 {
		t.Errorf("restored = %+v", restored)
	}

	if body := getWeek(t, h, location).Body.String(); strings.Contains(body, "undo-bar") {
		t.Error("undo bar still shown after restoring")
	}
}

func TestClearWeekCanBeUndone(t *testing.T) {
	h := newTestHandler(t, trashNow)
	seedIntention(t, h.db, "2026-11-30 07:00", false, "A")
	seedIntention(t, h.db, "2026-12-01 07:00", false, "B")
	seedPlaceholder(t, h.db, "2026-12-01 18:00")
	err := masses.MarkLoaded(h.db, time.Date(2026, 11, 30, 0, 0, 0, 0, intentions.TimeZone), 1)
	if err != nil {
		t.Fatalf("marking week: %v", err)
	}

	rec := postClear(t, h, url.Values{"week": {"2026-11-30"}})
	location := rec.Header().Get("Location")
	batch := batchFrom(t, location)

	body := getWeek(t, h, location).Body.String()
	checkBody(t, body, `<span>Wyczyszczono tydzień: 2 intencje i 1 puste miejsce.</span>`)

	rec = postRestore(t, h, batch)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("restore status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := countIntentions(t, h.db); got != 3 {
		t.Errorf("intentions after undo = %d, want 3", got)
	}
}

func TestRestoreUnknownBatch(t *testing.T) {
	h := newTestHandler(t, trashNow)

	if rec := postRestore(t, h, "123"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown batch: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if rec := postRestore(t, h, "abc"); rec.Code != http.StatusBadRequest {
		t.Errorf("bad batch: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestWeekIgnoresUnknownDeletedParam(t *testing.T) {
	h := newTestHandler(t, trashNow)

	for _, target := range []string{"/?week=2026-11-30&deleted=999", "/?week=2026-11-30&deleted=x"} {
		rec := getWeek(t, h, target)
		if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "undo-bar") {
			t.Errorf("%s: status %d, undo bar shown = %v", target, rec.Code, strings.Contains(rec.Body.String(), "undo-bar"))
		}
	}
}

func TestSettingsListTrash(t *testing.T) {
	h := newTestHandler(t, trashNow)

	body := getSettings(t, h, "/settings").Body.String()
	checkBody(t, body, `<li><a href="#kosz">Kosz</a></li>`, `<p class="empty">Kosz jest pusty.</p>`)

	single := newIntention(t, 0, "2026-12-01 18:00", false, "+ Jan Kowalski")
	h.db.Create(&single)
	rec := deleteByHandler(t, h, single.ID)
	singleBatch := batchFrom(t, rec.Header().Get("Location"))

	for i := range 5 {
		seedIntention(t, h.db, "2026-12-0"+strconv.Itoa(i+1)+" 07:00", false, "Intencja "+strconv.Itoa(i+1))
	}
	rec = postClear(t, h, url.Values{"week": {"2026-11-30"}})
	weekBatch := batchFrom(t, rec.Header().Get("Location"))

	body = getSettings(t, h, "/settings").Body.String()
	checkBody(t, body,
		`<form class="trash-row" method="post" action="/trash/`+singleBatch+`/restore">`,
		`<p class="trash-title">Wtorek 1.12.2026, 18:00 – &#43; Jan Kowalski</p>`,
		`<form class="trash-row" method="post" action="/trash/`+weekBatch+`/restore">`,
		`<p class="trash-title">Wyczyszczony tydzień 30 listopada – 6 grudnia 2026</p>`,
		`<p class="trash-item">Wtorek 1.12.2026, 7:00 – Intencja 1</p>`,
		`<p class="trash-item">i 2 więcej</p>`,
		`<p class="trash-when">usunięto dziś, 14:32</p>`,
		`przez 30 dni od usunięcia`,
	)
	if strings.Index(body, weekBatch) > strings.Index(body, singleBatch) {
		t.Error("newest deletion is not listed first")
	}
	if strings.Contains(body, "Kosz jest pusty.") {
		t.Error("trash shown as empty")
	}
}

func TestDeleteDialogMentionsTrash(t *testing.T) {
	h := newTestHandler(t, trashNow)
	intention := newIntention(t, 0, "2026-12-01 18:00", false, "+ Jan Kowalski")
	h.db.Create(&intention)

	body := getWeek(t, h, "/?week=2026-11-30&delete="+strconv.Itoa(intention.ID)).Body.String()
	checkBody(t, body, `<p class="hint">Intencja trafi do kosza – przez 30 dni można ją przywrócić.</p>`)
}

func TestTrashSummaries(t *testing.T) {
	tests := []struct {
		batch trash.Batch
		want  string
	}{
		{batch: trash.Batch{Week: "2026-11-30", Empty: 3}, want: "Wyczyszczono tydzień: 3 puste miejsca."},
		{batch: trash.Batch{Week: "2026-11-30", Intentions: make([]intentions.Intention, 5)}, want: "Wyczyszczono tydzień: 5 intencji."},
		{batch: trash.Batch{Empty: 1}, want: "Usunięto puste miejsce na intencję."},
	}

	for _, tt := range tests {
		if got := batchSummary(tt.batch); got != tt.want {
			t.Errorf("batchSummary(%+v) = %q, want %q", tt.batch, got, tt.want)
		}
	}
}
