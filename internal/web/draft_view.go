package web

import (
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"go-parafia/internal/draft"
	"go-parafia/internal/intentions"
)

const (
	maxDraftBody     = 64 << 10
	maxDraftToken    = 64
	draftPreviewSize = 160
)

type draftNotice struct {
	SavedAt    string
	Content    string
	When       string
	RestoreURL string
	IsEdit     bool

	token string
}

func newDraftNotice(d draft.Draft, now time.Time) *draftNotice {
	notice := &draftNotice{
		SavedAt:    backupTime(d.SavedAt, now),
		Content:    shorten(strings.TrimSpace(d.Content), draftPreviewSize),
		When:       draftWhen(d),
		RestoreURL: "/?week=" + draftWeek(d, now).Format(intentions.DateLayout) + "&draft=1",
		IsEdit:     d.EditID > 0,
		token:      d.Token,
	}

	return notice
}

func draftWhen(d draft.Draft) string {
	var parts []string

	if day, err := time.ParseInLocation(intentions.DateLayout, d.Day, intentions.TimeZone); err == nil {
		when := weekdayNames[day.Weekday()] + " " + day.Format("2.1")
		if at, err := time.Parse("15:04", d.Time); err == nil {
			when += ", godz. " + clockLabel(at)
		}
		parts = append(parts, when)
	}

	if amount, err := intentions.ParseGrosze(d.Amount); err == nil && amount > 0 {
		parts = append(parts, "wpłata "+amount.String())
	}

	return strings.Join(parts, " · ")
}

func draftWeek(d draft.Draft, now time.Time) time.Time {
	for _, value := range []string{d.Week, d.Day} {
		day, err := time.ParseInLocation(intentions.DateLayout, value, intentions.TimeZone)
		if err == nil {
			return intentions.WeekStart(day)
		}
	}

	return intentions.NextWeekStart(now)
}

func shorten(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}

	runes := []rune(text)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func (h *handler) draftNotice() *draftNotice {
	d, ok, err := h.drafts.Load()
	if err != nil {
		log.Printf("error loading intention draft: %v", err)
	}
	if !ok {
		return nil
	}

	return newDraftNotice(d, h.now())
}

func (h *handler) otherDraft(token string) *draftNotice {
	notice := h.draftNotice()
	if notice == nil || (token != "" && notice.token == token) {
		return nil
	}

	return notice
}

func (h *handler) restoredForm(w http.ResponseWriter, start time.Time) (*formView, bool) {
	d, ok, err := h.drafts.Load()
	if err != nil {
		log.Printf("error loading intention draft: %v", err)
	}
	if !ok {
		return nil, true
	}

	form := newFormView(start, d.Day, d.Time)
	form.Content = d.Content
	form.IsOutsideParish = d.Outside
	form.IsDeceased = d.Deceased
	form.IsGregorian = d.Gregorian
	form.IsHidden = d.Hidden
	form.Amount = d.Amount
	form.SeriesID = d.SeriesID
	form.AcceptedOn = d.Accepted
	form.DraftToken = d.Token
	form.Restored = "Przywrócono niezapisaną intencję z " + backupTime(d.SavedAt, h.now()) + ". Sprawdź ją i kliknij „Zapisz”."

	if d.EditID > 0 {
		var intention intentions.Intention
		err = h.db.First(&intention, d.EditID).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			form.SeriesID = 0
			form.Restored += " Zmieniana intencja została w międzyczasie usunięta, więc zapisze się jako nowa."
		case err != nil:
			log.Printf("error loading intention %d for its draft: %v", d.EditID, err)
			serverError(w)
			return nil, false
		default:
			form.EditID = intention.ID
			form.SeriesID = intention.SeriesID
		}
	}

	return &form, true
}

func (h *handler) saveDraft(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxDraftBody)
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	d, ok := draftFromForm(r.PostForm)
	if !ok {
		http.Error(w, "nieprawidłowy szkic", http.StatusBadRequest)
		return
	}

	d.SavedAt = h.now()
	err = h.drafts.Save(d)
	if err != nil {
		log.Printf("error saving intention draft: %v", err)
		serverError(w)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func draftFromForm(form url.Values) (draft.Draft, bool) {
	token := form.Get("draft_token")
	if token == "" || len(token) > maxDraftToken {
		return draft.Draft{}, false
	}

	return draft.Draft{
		Token:     token,
		EditID:    positiveInt(form.Get("edit")),
		Week:      form.Get("week"),
		Day:       form.Get("day"),
		Time:      form.Get("time"),
		Content:   form.Get("content"),
		Outside:   form.Get("outside") == "on",
		Deceased:  form.Get("deceased") == "on",
		Gregorian: form.Get("gregorian") == "on",
		Hidden:    form.Get("hidden") == "on",
		Amount:    form.Get("amount"),
		SeriesID:  positiveInt(form.Get("series")),
		Accepted:  form.Get("accepted"),
	}, true
}

func positiveInt(text string) int {
	n, err := strconv.Atoi(text)
	if err != nil || n < 0 {
		return 0
	}

	return n
}

func (h *handler) discardDraft(w http.ResponseWriter, r *http.Request) {
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "nieprawidłowe dane formularza", http.StatusBadRequest)
		return
	}

	token := r.PostForm.Get("draft_token")
	if token != "" {
		h.finishDraft(token)
		w.WriteHeader(http.StatusNoContent)
		return
	}

	err = h.drafts.Discard()
	if err != nil {
		log.Printf("error discarding intention draft: %v", err)
	}

	back, _ := refererPage(r)
	http.Redirect(w, r, back, http.StatusSeeOther)
}

func (h *handler) finishDraft(token string) {
	err := h.drafts.Finish(token)
	if err != nil {
		log.Printf("error removing intention draft: %v", err)
	}
}
