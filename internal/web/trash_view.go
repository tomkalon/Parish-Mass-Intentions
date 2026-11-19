package web

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"go-parafia/internal/intentions"
	"go-parafia/internal/trash"
)

const trashPreviewSize = 90

func deletedURL(week time.Time, batch int64) string {
	target := weekURL(week)
	if batch > 0 {
		target += "&deleted=" + strconv.FormatInt(batch, 10)
	}

	return target
}

func restoreURL(batch int64) string {
	return "/trash/" + strconv.FormatInt(batch, 10) + "/restore"
}

type undoView struct {
	Text   string
	Action string
}

func newUndoView(b trash.Batch) undoView {
	return undoView{Text: batchSummary(b), Action: restoreURL(b.ID)}
}

func batchSummary(b trash.Batch) string {
	if b.Week != "" {
		parts := []string{}
		if len(b.Intentions) > 0 {
			parts = append(parts, countOf(len(b.Intentions), "intencja", "intencje", "intencji"))
		}
		if b.Empty > 0 {
			parts = append(parts, countOf(b.Empty, "puste miejsce", "puste miejsca", "pustych miejsc"))
		}
		return "Wyczyszczono tydzień: " + strings.Join(parts, " i ") + "."
	}

	if len(b.Intentions) == 0 {
		return "Usunięto puste miejsce na intencję."
	}

	intention := b.Intentions[0]
	at := intention.MassAt.In(intentions.TimeZone)
	return fmt.Sprintf("Usunięto intencję „%s” (%s, godz. %s).", shorten(intention.Content, trashPreviewSize), dayLabel(at), clockLabel(at))
}

func (h *handler) undoNotice(param string) *undoView {
	batch, err := strconv.ParseInt(param, 10, 64)
	if err != nil || batch <= 0 {
		return nil
	}

	b, err := trash.Load(h.db, batch)
	if err != nil {
		if !errors.Is(err, trash.ErrBatchNotFound) {
			log.Printf("error loading trash batch %d: %v", batch, err)
		}
		return nil
	}

	view := newUndoView(b)
	return &view
}

type trashRow struct {
	ID         int64
	DeletedAt  string
	Title      string
	Items      []string
	More       int
	Action     string
	ConfirmURL string
}

const trashRowItems = 3

func settingsTrashURL(week string, restore int64) string {
	query := url.Values{"trash": {"1"}}
	if week != "" {
		query.Set("week", week)
	}
	if restore > 0 {
		query.Set("restore", strconv.FormatInt(restore, 10))
	}

	return "/settings?" + query.Encode()
}

func (v settingsView) TrashCount() string {
	if len(v.Trash) == 0 {
		return "Kosz jest pusty."
	}

	return "W koszu: " + countOf(len(v.Trash), "pozycja", "pozycje", "pozycji") + "."
}

func (v *settingsView) openTrash(restore string) {
	v.TrashOpen = true
	v.TrashCloseURL = settingsURL(v.Week, false) + "#kosz"

	id, err := strconv.ParseInt(restore, 10, 64)
	if err != nil {
		return
	}
	for _, row := range v.Trash {
		if row.ID != id {
			continue
		}

		text := row.Title
		if len(row.Items)+row.More > 0 {
			text += " (razem z: " + strings.Join(row.Items, "; ")
			if row.More > 0 {
				text += " i " + strconv.Itoa(row.More) + " więcej"
			}
			text += ")"
		}
		v.Dialog = &settingsDialog{
			Title:    "Przywrócić z kosza?",
			Text:     text + ". Usunięto " + row.DeletedAt + ".",
			Action:   row.Action,
			Button:   "Przywróć",
			Week:     v.Week,
			CloseURL: settingsTrashURL(v.Week, 0),
			Safe:     true,
		}
	}
}

func newTrashRows(batches []trash.Batch, now time.Time, week string) []trashRow {
	rows := make([]trashRow, 0, len(batches))
	for _, b := range batches {
		row := trashRow{ID: b.ID, DeletedAt: backupTime(b.TrashedAt, now), Action: restoreURL(b.ID), ConfirmURL: settingsTrashURL(week, b.ID)}

		if b.Week != "" {
			start, err := time.ParseInLocation(intentions.DateLayout, b.Week, intentions.TimeZone)
			if err == nil {
				row.Title = "Wyczyszczony tydzień " + weekInWords(start)
			}
		}
		for i, intention := range b.Intentions {
			at := intention.MassAt.In(intentions.TimeZone)
			item := weekdayNames[at.Weekday()] + " " + at.Format("2.1.2006") + ", " + clockLabel(at) + " – " + shorten(intention.Content, trashPreviewSize)
			if b.Week == "" && i == 0 {
				row.Title = item
				continue
			}
			if len(row.Items) < trashRowItems {
				row.Items = append(row.Items, item)
			} else {
				row.More++
			}
		}
		if b.Empty > 0 {
			row.Items = append(row.Items, countOf(b.Empty, "puste miejsce", "puste miejsca", "pustych miejsc"))
		}
		if row.Title == "" {
			row.Title = "Puste miejsce na intencję"
		}

		rows = append(rows, row)
	}

	return rows
}

func (h *handler) restoreTrash(w http.ResponseWriter, r *http.Request) {
	batch, err := strconv.ParseInt(chi.URLParam(r, "batch"), 10, 64)
	if err != nil || batch <= 0 {
		http.Error(w, "nieprawidłowy identyfikator w koszu", http.StatusBadRequest)
		return
	}

	restored, err := trash.Restore(h.db, batch)
	if errors.Is(err, trash.ErrBatchNotFound) {
		http.Error(w, "tej pozycji nie ma już w koszu", http.StatusNotFound)
		return
	}
	if err != nil {
		log.Printf("error restoring trash batch %d: %v", batch, err)
		serverError(w)
		return
	}

	for _, intention := range restored {
		if !intention.IsPlaceholder {
			week := intentions.WeekStart(intention.MassAt).Format(intentions.DateLayout)
			http.Redirect(w, r, fmt.Sprintf("/?week=%s&saved=%d#i-%d", week, intention.ID, intention.ID), http.StatusSeeOther)
			return
		}
	}
	if len(restored) > 0 {
		http.Redirect(w, r, weekURL(intentions.WeekStart(restored[0].MassAt)), http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/settings#kosz", http.StatusSeeOther)
}

func (weekView) TrashDays() int {
	return trash.RetentionDays
}

func (settingsView) TrashDays() int {
	return trash.RetentionDays
}
