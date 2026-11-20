package web

import (
	"net/url"
	"time"

	"go-parafia/internal/calendar"
	"go-parafia/internal/intentions"
)

const maxFeastLength = 300

type feastView struct {
	Action   string
	Day      string
	Feast    string
	Error    string
	CloseURL string
}

func newFeastView(day time.Time) feastView {
	date := day.Format(intentions.DateLayout)
	week := intentions.WeekStart(day).Format(intentions.DateLayout)

	return feastView{
		Action:   "/days/" + date,
		Day:      dayLabel(day),
		CloseURL: "/?week=" + week,
	}
}

func (f feastView) MaxLength() int {
	return maxFeastLength
}

func feastURL(week, date string) string {
	query := url.Values{}
	query.Set("week", week)
	query.Set("feast", date)

	return "/?" + query.Encode()
}

type feastImportView struct {
	Title string
}

func newFeastImportView(last calendar.FeastImport, now time.Time) *feastImportView {
	when := backupTime(last.ImportedAt, now)
	title := "Święta pobrane z GCatholic " + when + ". "
	if last.Added > 0 {
		title += "Dopisano opisy do " + countOf(last.Added, "dnia", "dni", "dni") + "."
	} else {
		title += "Nic nie dopisano – dni miały już opisy albo GCatholic nie ma dla nich obchodów."
	}

	return &feastImportView{Title: title + " Kliknij, aby pobrać ponownie – uzupełni tylko puste dni."}
}
