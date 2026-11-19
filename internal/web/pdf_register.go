package web

import (
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/signintech/gopdf"

	"go-parafia/internal/intentions"
)

const (
	registerWidth   = 842.0
	registerHeight  = 595.0
	registerSide    = 12 * mm
	registerTop     = 11 * mm
	registerBottom  = 9 * mm
	registerContent = registerWidth - 2*registerSide

	registerTextSize = 9.5
	registerLine     = 1.3
	registerHeadSize = 8.5
	registerPad      = 4.0
	registerFootSize = 8.0
)

var registerPageSize = &gopdf.Rect{W: registerWidth, H: registerHeight}

type registerColumn struct {
	title string
	width float64
	right bool
}

var registerColumns = []registerColumn{
	{title: "Lp.", width: 30, right: true},
	{title: "Przyjęto", width: 62},
	{title: "Msza", width: 118},
	{title: "Intencja"},
	{title: "Ofiara", width: 70, right: true},
	{title: "Podpis celebransa", width: 140},
}

const registerContentColumn = 3

type registerView struct {
	Period     string
	ParishName string
	Address    string
	Rows       []registerRow
	Total      intentions.Grosze
}

type registerRow struct {
	Number   int
	Accepted string
	Mass     string
	Content  string
	Outside  bool
	Amount   string
	InSeries bool
}

func newRegisterView(period string, list []intentions.Intention, steps map[int]int) registerView {
	view := registerView{Period: period}

	for i, intention := range list {
		at := intention.MassAt.In(intentions.TimeZone)
		row := registerRow{
			Number:   i + 1,
			Accepted: "—",
			Mass:     weekdayShortNames[at.Weekday()] + " " + at.Format("2.1.2006") + ", " + clockLabel(at),
			Content: printedContent(intentionView{
				Content:     intention.Content,
				IsDeceased:  intention.IsDeceased,
				IsGregorian: intention.IsGregorian,
				SeriesStep:  steps[intention.ID],
			}),
			Outside: intention.IsOutsideParish,
			Amount:  "—",
		}
		if day, err := time.ParseInLocation(intentions.DateLayout, intention.AcceptedOn, intentions.TimeZone); err == nil {
			row.Accepted = day.Format("2.1.2006")
		}
		switch {
		case intention.Amount > 0:
			row.Amount = intention.Amount.String()
			view.Total += intention.Amount
		case intention.SeriesID > 0 && steps[intention.ID] > 1:
			row.Amount = "w serii"
			row.InSeries = true
		}

		view.Rows = append(view.Rows, row)
	}

	return view
}

func (v registerView) Summary() string {
	if len(v.Rows) == 0 {
		return "Brak intencji w tym okresie."
	}

	outside := 0
	for _, row := range v.Rows {
		if row.Outside {
			outside++
		}
	}

	text := "Razem: " + countOf(len(v.Rows), "intencja", "intencje", "intencji")
	if outside > 0 {
		text += " (w tym " + strconv.Itoa(outside) + " poza parafią)"
	}
	if v.Total > 0 {
		text += " · ofiary: " + v.Total.String()
	}

	return text + "."
}

func writeRegisterPDF(w io.Writer, view registerView) error {
	return writePDF(w, "Księga intencji mszalnych – "+view.Period, registerPageSize, func(m textMeasurer) []pdfPage {
		return layoutRegister(view, m)
	})
}

type registerLayout struct {
	x      []float64
	widths []float64
}

func newRegisterLayout() registerLayout {
	fixed := 0.0
	for _, column := range registerColumns {
		fixed += column.width
	}

	layout := registerLayout{}
	x := registerSide
	for _, column := range registerColumns {
		width := column.width
		if width == 0 {
			width = registerContent - fixed
		}
		layout.x = append(layout.x, x)
		layout.widths = append(layout.widths, width)
		x += width
	}

	return layout
}

func layoutRegister(view registerView, m textMeasurer) []pdfPage {
	layout := newRegisterLayout()
	head := layoutRegisterHead(layout, m)
	rows := make([]pdfBlock, 0, len(view.Rows))
	for _, row := range view.Rows {
		rows = append(rows, layoutRegisterRow(row, layout, m))
	}
	summary := layoutRegisterSummary(view.Summary())

	footerTop := registerHeight - registerBottom - registerFootSize*1.4
	var pages []pdfPage
	var page pdfPage
	var y, tableTop float64

	start := func() {
		page = pdfPage{}
		var title pdfBlock
		if len(pages) == 0 {
			title = layoutRegisterTitle(view, m)
		} else {
			title = layoutRegisterRunningTitle(view)
		}
		page.place(title, registerTop)
		tableTop = registerTop + title.Height
		page.place(head, tableTop)
		y = tableTop + head.Height
	}
	finish := func() {
		page.Rules = append(page.Rules, registerGrid(layout, tableTop, y)...)
		pages = append(pages, page)
	}

	start()
	for _, row := range rows {
		if y+row.Height > footerTop && y > tableTop+head.Height {
			finish()
			start()
		}
		page.place(row, y)
		y += row.Height
	}
	tableBottom := y
	page.Rules = append(page.Rules, registerGrid(layout, tableTop, tableBottom)...)
	if tableBottom+summary.Height > footerTop {
		pages = append(pages, page)
		page = pdfPage{}
		running := layoutRegisterRunningTitle(view)
		page.place(running, registerTop)
		tableBottom = registerTop + running.Height
	}
	page.place(summary, tableBottom)
	pages = append(pages, page)

	for i := range pages {
		label := fmt.Sprintf("str. %d z %d", i+1, len(pages))
		pages[i].Runs = append(pages[i].Runs, centeredIn(label, fontRegular, registerFootSize, 0, footerTop+baseline(registerFootSize, 1.4), registerSide, registerContent, m))
	}

	return pages
}

func layoutRegisterTitle(view registerView, m textMeasurer) pdfBlock {
	var block pdfBlock
	y := 0.0

	if view.ParishName != "" {
		size := 10.5
		block.Runs = append(block.Runs, centeredIn(view.ParishName, fontItalic, size, 0, y+baseline(size, 1.3), registerSide, registerContent, m))
		y += size * 1.3
	}
	if view.Address != "" {
		size := 8.5
		block.Runs = append(block.Runs, centeredIn(view.Address, fontRegular, size, 0, y+baseline(size, 1.3), registerSide, registerContent, m))
		y += size * 1.3
	}
	if y > 0 {
		y += 4
	}

	titleSize := 9.0
	block.Runs = append(block.Runs, centeredIn("KSIĘGA INTENCJI MSZALNYCH", fontBold, titleSize, 0.32*titleSize, y+baseline(titleSize, 1.3), registerSide, registerContent, m))
	y += titleSize * 1.3

	periodSize := 16.0
	block.Runs = append(block.Runs, centeredIn(view.Period, fontRegular, periodSize, 0, y+baseline(periodSize, 1.2), registerSide, registerContent, m))
	y += periodSize*1.2 + 8

	block.Height = y

	return block
}

func layoutRegisterRunningTitle(view registerView) pdfBlock {
	size := 8.5
	text := "Księga intencji mszalnych – " + view.Period
	if view.ParishName != "" {
		text += " · " + view.ParishName
	}

	return pdfBlock{
		Height: size*1.4 + 4,
		Runs:   []pdfRun{{X: registerSide, Baseline: baseline(size, 1.4), Font: fontItalic, Size: size, Text: text}},
	}
}

func layoutRegisterHead(layout registerLayout, m textMeasurer) pdfBlock {
	line := registerHeadSize * registerLine
	block := pdfBlock{Height: line + 2*registerPad}

	for i, column := range registerColumns {
		run := pdfRun{X: layout.x[i] + registerPad, Baseline: registerPad + baseline(registerHeadSize, registerLine), Font: fontBold, Size: registerHeadSize, Text: column.title}
		if column.right {
			run.X = layout.x[i] + layout.widths[i] - registerPad - m.width(column.title, fontBold, registerHeadSize, 0)
		}
		block.Runs = append(block.Runs, run)
	}

	right := registerSide + registerContent
	block.Rules = append(block.Rules,
		pdfRule{X1: registerSide, X2: right, Y: 0, Width: 0.8},
		pdfRule{X1: registerSide, X2: right, Y: block.Height, Width: 0.8},
	)

	return block
}

func layoutRegisterRow(row registerRow, layout registerLayout, m textMeasurer) pdfBlock {
	line := registerTextSize * registerLine
	textBaseline := registerPad + baseline(registerTextSize, registerLine)
	var block pdfBlock

	cell := func(i int, text string, font pdfFont) {
		run := pdfRun{X: layout.x[i] + registerPad, Baseline: textBaseline, Font: font, Size: registerTextSize, Text: text}
		if registerColumns[i].right {
			run.X = layout.x[i] + layout.widths[i] - registerPad - m.width(text, font, registerTextSize, 0)
		}
		block.Runs = append(block.Runs, run)
	}

	cell(0, strconv.Itoa(row.Number)+".", fontRegular)
	cell(1, row.Accepted, fontRegular)
	cell(2, row.Mass, fontRegular)
	amountFont := fontRegular
	if row.InSeries {
		amountFont = fontItalic
	}
	cell(4, row.Amount, amountFont)

	contentStyle := pdfStyle{textSize: registerTextSize, lineHeight: registerLine}
	contentX := layout.x[registerContentColumn] + registerPad
	contentWidth := layout.widths[registerContentColumn] - 2*registerPad
	lines := layoutIntention(&block, printedIntention{content: row.Content, outside: row.Outside}, contentX, contentWidth, registerPad, contentStyle, m)

	block.Height = float64(max(lines, 1))*line + 2*registerPad
	block.Rules = append(block.Rules, pdfRule{X1: registerSide, X2: registerSide + registerContent, Y: block.Height, Width: 0.4})

	return block
}

func layoutRegisterSummary(text string) pdfBlock {
	size := registerTextSize

	return pdfBlock{
		Height: 8 + size*1.4,
		Runs:   []pdfRun{{X: registerSide, Baseline: 8 + baseline(size, 1.4), Font: fontBold, Size: size, Text: text}},
	}
}

func registerGrid(layout registerLayout, top, bottom float64) []pdfRule {
	var rules []pdfRule
	for _, x := range append(layout.x, registerSide+registerContent) {
		rules = append(rules, pdfRule{X1: x, X2: x, Y: top, Y2: bottom, Width: 0.4})
	}

	return rules
}
