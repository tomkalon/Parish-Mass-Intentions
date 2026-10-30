package web

import (
	"net/url"
	"time"

	"go-parafia/internal/intentions"
)

const maxFeastLength = 150

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

func feastURL(week, date string) string {
	query := url.Values{}
	query.Set("week", week)
	query.Set("feast", date)

	return "/?" + query.Encode()
}
