package web

import (
	"time"

	"go-parafia/internal/intentions"
)

type printView struct {
	weekView

	HiddenCount int
	ShowHidden  bool
}

func newPrintView(start time.Time, list []intentions.Intention, showHidden bool) printView {
	view := printView{weekView: newWeekView(start, nil), ShowHidden: showHidden}
	for _, intention := range list {
		if intention.IsHidden {
			view.HiddenCount++
		}
	}

	return view
}

func (v printView) PDFURL() string {
	if v.ShowHidden {
		return v.weekView.PDFURL() + "&hidden=1"
	}

	return v.weekView.PDFURL()
}
