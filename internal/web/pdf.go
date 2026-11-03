package web

import (
	_ "embed"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/signintech/gopdf"
)

//go:embed fonts/LiberationSerif-Regular.ttf
var serifRegular []byte

//go:embed fonts/LiberationSerif-Bold.ttf
var serifBold []byte

//go:embed fonts/LiberationSerif-Italic.ttf
var serifItalic []byte

var monthsGenitive = [...]string{
	"stycznia", "lutego", "marca", "kwietnia", "maja", "czerwca",
	"lipca", "sierpnia", "września", "października", "listopada", "grudnia",
}

const (
	mm = 72.0 / 25.4
	px = 0.75

	pageWidth    = 595.0
	pageHeight   = 842.0
	marginTop    = 12 * mm
	marginSide   = 15 * mm
	marginBottom = 10 * mm
	contentWidth = pageWidth - 2*marginSide
	pageContent  = pageHeight - marginTop - marginBottom

	lineAscent     = 0.891
	lineDescent    = 0.216
	smallCapsScale = 0.78
	groupGap       = 3 * px

	pdfFamily = "serif"
)

type pdfFont int

const (
	fontRegular pdfFont = iota
	fontBold
	fontItalic
)

var pdfFontStyles = [...]int{
	fontRegular: gopdf.Regular,
	fontBold:    gopdf.Bold,
	fontItalic:  gopdf.Italic,
}

type pdfStyle struct {
	textSize   float64
	lineHeight float64
	padTop     float64
	padBottom  float64
}

var pdfStyles = []pdfStyle{
	{textSize: 13.5 * px, lineHeight: 1.42, padTop: 9 * px, padBottom: 8 * px},
	{textSize: 13.5 * px, lineHeight: 1.42, padTop: 6 * px, padBottom: 6 * px},
	{textSize: 13.5 * px, lineHeight: 1.32, padTop: 6 * px, padBottom: 6 * px},
	{textSize: 13 * px, lineHeight: 1.32, padTop: 6 * px, padBottom: 6 * px},
	{textSize: 12.5 * px, lineHeight: 1.32, padTop: 6 * px, padBottom: 6 * px},
}

type textMeasurer interface {
	width(text string, font pdfFont, size, spacing float64) float64
}

type pdfRun struct {
	X, Baseline float64
	Font        pdfFont
	Size        float64
	Spacing     float64
	Text        string
}

type pdfRule struct {
	X1, X2, Y, Width float64
}

type pdfBlock struct {
	Height float64
	Runs   []pdfRun
	Rules  []pdfRule
}

type pdfPage struct {
	Runs  []pdfRun
	Rules []pdfRule
}

func (p *pdfPage) place(block pdfBlock, top float64) {
	for _, run := range block.Runs {
		run.Baseline += top
		p.Runs = append(p.Runs, run)
	}
	for _, rule := range block.Rules {
		rule.Y += top
		p.Rules = append(p.Rules, rule)
	}
}

func writeWeekPDF(w io.Writer, view weekView) error {
	pdf, err := newPDFDocument()
	if err != nil {
		return err
	}

	pdf.SetInfo(gopdf.PdfInfo{Title: "Intencje mszy świętych – " + weekInWords(view.Start), Creator: "Go Parafia"})

	measurer := &gopdfMeasurer{pdf: pdf}
	pages := layoutWeek(view, measurer)
	if measurer.err != nil {
		return fmt.Errorf("measuring text: %w", measurer.err)
	}

	for _, page := range pages {
		pdf.AddPage()
		for _, run := range page.Runs {
			err = setPDFFont(pdf, run.Font, run.Size, run.Spacing)
			if err != nil {
				return err
			}
			pdf.SetXY(run.X, run.Baseline)
			err = pdf.Text(run.Text)
			if err != nil {
				return fmt.Errorf("writing text %q: %w", run.Text, err)
			}
		}
		for _, rule := range page.Rules {
			pdf.SetLineWidth(rule.Width)
			pdf.Line(rule.X1, rule.Y, rule.X2, rule.Y)
		}
	}

	_, err = pdf.WriteTo(w)
	return err
}

func newPDFDocument() (*gopdf.GoPdf, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{Unit: gopdf.UnitPT, PageSize: *gopdf.PageSizeA4})

	fonts := []struct {
		font pdfFont
		data []byte
	}{
		{fontRegular, serifRegular},
		{fontBold, serifBold},
		{fontItalic, serifItalic},
	}
	for _, f := range fonts {
		err := pdf.AddTTFFontDataWithOption(pdfFamily, f.data, gopdf.TtfOption{Style: pdfFontStyles[f.font]})
		if err != nil {
			return nil, fmt.Errorf("adding font: %w", err)
		}
	}

	return pdf, nil
}

func setPDFFont(pdf *gopdf.GoPdf, font pdfFont, size, spacing float64) error {
	err := pdf.SetFontWithStyle(pdfFamily, pdfFontStyles[font], size)
	if err != nil {
		return fmt.Errorf("setting font: %w", err)
	}

	return pdf.SetCharSpacing(spacing)
}

type gopdfMeasurer struct {
	pdf *gopdf.GoPdf
	err error
}

func (m *gopdfMeasurer) width(text string, font pdfFont, size, spacing float64) float64 {
	if m.err != nil {
		return 0
	}

	err := setPDFFont(m.pdf, font, size, spacing)
	if err != nil {
		m.err = err
		return 0
	}

	width, err := m.pdf.MeasureTextWidth(text)
	if err != nil {
		m.err = err
		return 0
	}

	return width
}

func layoutWeek(view weekView, m textMeasurer) []pdfPage {
	header := layoutHeader(view, m)
	days := printableDays(view)

	style := pdfStyles[len(pdfStyles)-1]
	for _, s := range pdfStyles {
		if header.Height+totalHeight(layoutDays(days, s, m)) <= pageContent {
			style = s
			break
		}
	}

	return paginate(header, days, style, m)
}

func layoutDays(days []printedDay, style pdfStyle, m textMeasurer) []pdfBlock {
	blocks := make([]pdfBlock, 0, len(days))
	for _, day := range days {
		blocks = append(blocks, layoutDay(day, style, m))
	}

	return blocks
}

func totalHeight(blocks []pdfBlock) float64 {
	total := 0.0
	for _, block := range blocks {
		total += block.Height
	}

	return total
}

func paginate(header pdfBlock, days []printedDay, style pdfStyle, m textMeasurer) []pdfPage {
	pages := []pdfPage{{}}
	pages[0].place(header, marginTop)

	y := marginTop + header.Height
	bottom := marginTop + pageContent
	place := func(block pdfBlock) {
		pages[len(pages)-1].place(block, y)
		y += block.Height
	}
	newPage := func() {
		pages = append(pages, pdfPage{})
		y = marginTop
	}

	for _, day := range days {
		for {
			block := layoutDay(day, style, m)
			if y+block.Height <= bottom {
				place(block)
				break
			}
			if block.Height <= pageContent && y > marginTop {
				newPage()
				continue
			}

			first, rest, ok := splitDay(day, bottom-y, style, m)
			if !ok {
				if y > marginTop {
					newPage()
					continue
				}
				place(block)
				break
			}

			place(layoutDay(first, style, m))
			newPage()
			day = rest
		}
	}

	return pages
}

func splitDay(day printedDay, available float64, style pdfStyle, m textMeasurer) (printedDay, printedDay, bool) {
	count := 0
	for _, mass := range day.masses {
		count += len(mass.intentions)
	}

	fit := sort.Search(count, func(n int) bool {
		first, _ := day.split(n + 1)
		return layoutDay(first, style, m).Height > available
	})
	if fit == 0 || fit == count {
		return printedDay{}, printedDay{}, false
	}

	first, rest := day.split(fit)
	return first, rest, true
}

func layoutHeader(view weekView, m textMeasurer) pdfBlock {
	var block pdfBlock
	y := 0.0

	if view.ParishName != "" {
		parishSize := 13 * px
		block.Runs = append(block.Runs, centered(view.ParishName, fontItalic, parishSize, 0, y+baseline(parishSize, 1.3), m))
		y += parishSize * 1.3
	}
	if view.Address != "" {
		addressSize := 11 * px
		block.Runs = append(block.Runs, centered(view.Address, fontRegular, addressSize, 0, y+baseline(addressSize, 1.3), m))
		y += addressSize * 1.3
	}
	if y > 0 {
		y += 6 * px
	}

	titleSize := 11 * px
	block.Runs = append(block.Runs, centered("INTENCJE MSZY ŚWIĘTYCH", fontBold, titleSize, 0.32*titleSize, y+baseline(titleSize, 1.3), m))
	y += titleSize * 1.3

	rangeSize := 26 * px
	block.Runs = append(block.Runs, centered(weekInWords(view.Start), fontRegular, rangeSize, 0, y+baseline(rangeSize, 1.2), m))
	y += rangeSize*1.2 + 12*px

	block.Rules = append(block.Rules,
		pdfRule{X1: marginSide, X2: marginSide + contentWidth, Y: y, Width: px},
		pdfRule{X1: marginSide, X2: marginSide + contentWidth, Y: y + 2*px, Width: px},
	)
	block.Height = y + 3*px + 4*px

	return block
}

func layoutDay(day printedDay, style pdfStyle, m textMeasurer) pdfBlock {
	var block pdfBlock
	top := style.padTop
	date := day.date

	nameSize := 14.5 * px
	nameLine := nameSize * 1.25
	name := weekdayNames[date.Weekday()]
	if date.Weekday() == time.Sunday {
		runs, _ := smallCaps(name, fontBold, nameSize, 0.06*nameSize, marginSide, top+baseline(nameSize, 1.25), m)
		block.Runs = append(block.Runs, runs...)
	} else {
		block.Runs = append(block.Runs, pdfRun{X: marginSide, Baseline: top + baseline(nameSize, 1.25), Font: fontBold, Size: nameSize, Text: name})
	}

	dateSize := 12.5 * px
	dateLine := dateSize * 1.3
	block.Runs = append(block.Runs, pdfRun{X: marginSide, Baseline: top + nameLine + baseline(dateSize, 1.3), Font: fontRegular, Size: dateSize, Text: dateInWords(date)})
	leftHeight := nameLine + dateLine
	if day.continued {
		block.Runs = append(block.Runs, pdfRun{X: marginSide, Baseline: top + leftHeight + baseline(dateSize, 1.3), Font: fontItalic, Size: dateSize, Text: "ciąg dalszy"})
		leftHeight += dateLine
	}

	massesX := marginSide + 112*px + 14*px
	massesWidth := marginSide + contentWidth - massesX

	massesTop := top
	if day.feast != "" {
		massesTop += layoutFeast(&block, day.feast, massesX, massesWidth, top, m) + 4*px
	}
	rightHeight := massesTop - top + layoutMasses(&block, day.masses, massesX, massesWidth, massesTop, style, m)

	block.Height = top + max(leftHeight, rightHeight) + style.padBottom
	if day.ruleBelow {
		block.Rules = append(block.Rules, pdfRule{X1: marginSide, X2: marginSide + contentWidth, Y: block.Height, Width: 0.75 * px})
	}

	return block
}

func layoutFeast(block *pdfBlock, feast string, x, width, top float64, m textMeasurer) float64 {
	size := 12.5 * px
	line := size * 1.3

	measure := func(text string) float64 {
		return m.width(text, fontItalic, size, 0)
	}
	lines := wrapText(feast, width, width, measure)
	for i, text := range lines {
		block.Runs = append(block.Runs, pdfRun{X: x, Baseline: top + baseline(size, 1.3) + float64(i)*line, Font: fontItalic, Size: size, Text: text})
	}

	return float64(len(lines)) * line
}

type printedIntention struct {
	content string
	number  int
	outside bool
}

type printedMass struct {
	time       string
	numbered   bool
	intentions []printedIntention
}

type printedDay struct {
	date      time.Time
	feast     string
	masses    []printedMass
	continued bool
	ruleBelow bool
}

func printableDays(view weekView) []printedDay {
	days := make([]printedDay, 0, len(view.Days))
	for i, day := range view.Days {
		days = append(days, printedDay{
			date:      view.Start.AddDate(0, 0, i),
			feast:     day.Feast,
			masses:    printableMasses(day.Masses),
			ruleBelow: i < len(view.Days)-1,
		})
	}

	return days
}

func (d printedDay) split(n int) (printedDay, printedDay) {
	first, rest := d, d
	first.masses, rest.masses = nil, nil
	first.ruleBelow = false
	rest.feast = ""
	rest.continued = true

	for _, mass := range d.masses {
		switch {
		case n >= len(mass.intentions):
			first.masses = append(first.masses, mass)
			n -= len(mass.intentions)
		case n > 0:
			head, tail := mass, mass
			head.intentions, tail.intentions = mass.intentions[:n], mass.intentions[n:]
			first.masses = append(first.masses, head)
			rest.masses = append(rest.masses, tail)
			n = 0
		default:
			rest.masses = append(rest.masses, mass)
		}
	}

	return first, rest
}

func printableMasses(masses []massView) []printedMass {
	var result []printedMass
	for _, mass := range masses {
		parish := printedGroup(mass.Parish, false)
		outside := printedGroup(mass.Outside, true)
		if len(parish) == 0 && len(outside) == 0 {
			continue
		}

		result = append(result, printedMass{
			time:       mass.Time,
			numbered:   len(parish) > 1 || len(outside) > 1,
			intentions: append(parish, outside...),
		})
	}

	return result
}

func printedGroup(list []intentionView, outside bool) []printedIntention {
	var printed []printedIntention
	for _, intention := range list {
		if !intention.IsPlaceholder {
			printed = append(printed, printedIntention{content: printedContent(intention), outside: outside})
		}
	}

	if len(printed) > 1 {
		for i := range printed {
			printed[i].number = i + 1
		}
	}

	return printed
}

func printedContent(intention intentionView) string {
	content := intention.Content
	if intention.IsDeceased && !strings.HasPrefix(content, "+") {
		content = "+ " + content
	}
	if intention.IsGregorian && !strings.Contains(strings.ToLower(content), "(greg.") {
		if intention.SeriesStep > 0 {
			content += fmt.Sprintf(" (greg. %d/30)", intention.SeriesStep)
		} else {
			content += " (greg.)"
		}
	}

	return content
}

func layoutMasses(block *pdfBlock, masses []printedMass, x, width, top float64, style pdfStyle, m textMeasurer) float64 {
	line := style.textSize * style.lineHeight
	textBaseline := baseline(style.textSize, style.lineHeight)

	if len(masses) == 0 {
		block.Runs = append(block.Runs, pdfRun{X: x, Baseline: top + textBaseline, Font: fontItalic, Size: style.textSize, Text: "— brak intencji —"})
		return line
	}

	timeWidth := 46 * px
	textX := x + timeWidth + 8*px
	textWidth := x + width - textX

	y := top
	for i, mass := range masses {
		if i > 0 {
			y += 4 * px
		}

		w := m.width(mass.time, fontBold, style.textSize, 0)
		block.Runs = append(block.Runs, pdfRun{X: x + timeWidth - w, Baseline: y + textBaseline, Font: fontBold, Size: style.textSize, Text: mass.time})

		for n, intention := range mass.intentions {
			if n > 0 && intention.outside && !mass.intentions[n-1].outside {
				y += groupGap
			}

			contentX, available := textX, textWidth
			if mass.numbered {
				if intention.number > 0 {
					block.Runs = append(block.Runs, pdfRun{X: textX, Baseline: y + textBaseline, Font: fontRegular, Size: style.textSize, Text: fmt.Sprintf("%d.", intention.number)})
				}
				contentX += 20 * px
				available -= 20 * px
			}

			lines := layoutIntention(block, intention, contentX, available, y, style, m)
			y += float64(lines) * line
		}
	}

	return y - top
}

func layoutIntention(block *pdfBlock, intention printedIntention, x, width, top float64, style pdfStyle, m textMeasurer) int {
	line := style.textSize * style.lineHeight
	textBaseline := top + baseline(style.textSize, style.lineHeight)

	firstX := x
	if intention.outside {
		prefixSize := 12.5 * px
		runs, end := smallCaps("poza parafią · ", fontRegular, prefixSize, 0.04*prefixSize, x, textBaseline, m)
		block.Runs = append(block.Runs, runs...)
		firstX = end
	}

	measure := func(text string) float64 {
		return m.width(text, fontRegular, style.textSize, 0)
	}
	lines := wrapText(intention.content, x+width-firstX, width, measure)

	for i, text := range lines {
		lineX := x
		if i == 0 {
			lineX = firstX
		}
		block.Runs = append(block.Runs, pdfRun{X: lineX, Baseline: textBaseline + float64(i)*line, Font: fontRegular, Size: style.textSize, Text: text})
	}

	return len(lines)
}

func wrapText(text string, firstWidth, width float64, measure func(string) float64) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{""}
	}

	var lines []string
	line := words[0]
	limit := firstWidth
	for _, word := range words[1:] {
		candidate := line + " " + word
		if measure(candidate) <= limit {
			line = candidate
			continue
		}

		lines = append(lines, line)
		line = word
		limit = width
	}

	return append(lines, line)
}

func smallCaps(text string, font pdfFont, size, spacing, x, baseline float64, m textMeasurer) ([]pdfRun, float64) {
	var runs []pdfRun
	var part []rune
	partSmall := false

	flush := func() {
		if len(part) == 0 {
			return
		}
		runSize := size
		if partSmall {
			runSize = size * smallCapsScale
		}
		runText := string(part)
		runs = append(runs, pdfRun{X: x, Baseline: baseline, Font: font, Size: runSize, Spacing: spacing, Text: runText})
		x += m.width(runText, font, runSize, spacing)
		part = part[:0]
	}

	for _, r := range text {
		small := !unicode.IsUpper(r)
		if small != partSmall {
			flush()
			partSmall = small
		}
		part = append(part, unicode.ToUpper(r))
	}
	flush()

	return runs, x
}

func centered(text string, font pdfFont, size, spacing, baseline float64, m textMeasurer) pdfRun {
	width := m.width(text, font, size, spacing)

	return pdfRun{X: marginSide + (contentWidth-width)/2, Baseline: baseline, Font: font, Size: size, Spacing: spacing, Text: text}
}

func baseline(size, lineHeight float64) float64 {
	return (lineHeight*size-(lineAscent+lineDescent)*size)/2 + lineAscent*size
}

func weekInWords(start time.Time) string {
	end := start.AddDate(0, 0, 6)

	switch {
	case start.Year() != end.Year():
		return fmt.Sprintf("%s %d – %s %d", dateInWords(start), start.Year(), dateInWords(end), end.Year())
	case start.Month() != end.Month():
		return fmt.Sprintf("%s – %s %d", dateInWords(start), dateInWords(end), end.Year())
	default:
		return fmt.Sprintf("%d – %s %d", start.Day(), dateInWords(end), end.Year())
	}
}

func dateInWords(day time.Time) string {
	return fmt.Sprintf("%d %s", day.Day(), monthsGenitive[day.Month()-1])
}
