package web

import (
	"strconv"
	"time"

	"go-parafia/internal/intentions"
)

type deleteView struct {
	Action   string
	When     string
	Content  string
	CloseURL string

	IsPlaceholder bool
}

func newDeleteView(start time.Time, intention intentions.Intention) deleteView {
	at := intention.MassAt.In(intentions.TimeZone)

	when := dayLabel(at) + ", godz. " + clockLabel(at)
	if intention.IsOutsideParish {
		when += " · poza parafią"
	}
	if intention.IsDeceased {
		when += " · za zmarłych"
	}
	if intention.IsGregorian {
		when += " · gregorianka"
	}

	return deleteView{
		Action:   "/intentions/" + strconv.Itoa(intention.ID) + "/delete",
		When:     when,
		Content:  intention.Content,
		CloseURL: "/?week=" + start.Format(intentions.DateLayout),

		IsPlaceholder: intention.IsPlaceholder,
	}
}
