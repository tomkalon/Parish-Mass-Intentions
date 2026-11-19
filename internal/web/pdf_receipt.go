package web

import (
	"io"
	"strings"
	"time"

	"github.com/signintech/gopdf"
)

const (
	receiptHeight     = pageHeight / 2
	receiptLabel      = 100.0
	receiptSize       = 12.5
	receiptLineHeight = 1.35
)

type receiptView struct {
	ParishName string
	Address    string
	Lines      []receiptLine
}

type receiptLine struct {
	Label  string
	Value  string
	Italic bool
}

func longDate(day time.Time) string {
	return dateInWords(day) + " " + day.Format("2006") + " r."
}

func massInWords(at time.Time) string {
	return strings.ToLower(weekdayNames[at.Weekday()]) + ", " + longDate(at) + ", godz. " + clockLabel(at)
}

func writeReceiptPDF(w io.Writer, view receiptView) error {
	return writePDF(w, "Potwierdzenie przyjęcia intencji mszalnej", gopdf.PageSizeA4, func(m textMeasurer) []pdfPage {
		return []pdfPage{layoutReceipt(view, m)}
	})
}

func layoutReceipt(view receiptView, m textMeasurer) pdfPage {
	var page pdfPage
	y := 16 * mm

	if view.ParishName != "" {
		size := 14.0
		for _, text := range wrapText(view.ParishName, contentWidth, contentWidth, func(s string) float64 { return m.width(s, fontItalic, size, 0) }) {
			page.Runs = append(page.Runs, centered(text, fontItalic, size, 0, y+baseline(size, 1.3), m))
			y += size * 1.3
		}
	}
	if view.Address != "" {
		size := 10.0
		page.Runs = append(page.Runs, centered(view.Address, fontRegular, size, 0, y+baseline(size, 1.3), m))
		y += size * 1.3
	}
	y += 12

	titleSize := 10.5
	page.Runs = append(page.Runs, centered("POTWIERDZENIE PRZYJĘCIA INTENCJI MSZALNEJ", fontBold, titleSize, 0.3*titleSize, y+baseline(titleSize, 1.3), m))
	y += titleSize*1.3 + 8
	page.Rules = append(page.Rules, pdfRule{X1: marginSide + contentWidth/3, X2: marginSide + 2*contentWidth/3, Y: y, Width: 0.6})
	y += 16

	line := receiptSize * receiptLineHeight
	labelWidth := receiptLabel
	for _, row := range view.Lines {
		labelWidth = max(labelWidth, m.width(row.Label, fontBold, receiptSize, 0)+14)
	}
	valueX := marginSide + labelWidth
	valueWidth := marginSide + contentWidth - valueX
	for _, row := range view.Lines {
		font := fontRegular
		if row.Italic {
			font = fontItalic
		}
		if row.Label != "" {
			page.Runs = append(page.Runs, pdfRun{X: marginSide, Baseline: y + baseline(receiptSize, receiptLineHeight), Font: fontBold, Size: receiptSize, Text: row.Label})
		}
		lines := wrapText(row.Value, valueWidth, valueWidth, func(s string) float64 { return m.width(s, font, receiptSize, 0) })
		for i, text := range lines {
			page.Runs = append(page.Runs, pdfRun{X: valueX, Baseline: y + baseline(receiptSize, receiptLineHeight) + float64(i)*line, Font: font, Size: receiptSize, Text: text})
		}
		y += float64(len(lines))*line + 6
	}

	signY := max(y+40, receiptHeight-22*mm)
	signLeft := marginSide + contentWidth - 70*mm
	page.Rules = append(page.Rules, pdfRule{X1: signLeft, X2: marginSide + contentWidth, Y: signY, Width: 0.7, Dotted: true})
	noteSize := 9.0
	page.Runs = append(page.Runs, centeredIn("pieczęć i podpis", fontItalic, noteSize, 0, signY+4+baseline(noteSize, 1.3), signLeft, 70*mm, m))

	cutY := max(receiptHeight, signY+12*mm)
	page.Rules = append(page.Rules, pdfRule{X1: 8 * mm, X2: pageWidth - 8*mm, Y: cutY, Width: 0.5, Dashed: true})

	return page
}
