package web

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"go-parafia/internal/intentions"
)

const (
	paymentsWeek  = "week"
	paymentsMonth = "month"
	paymentsYear  = "year"

	monthLayout = "2006-01"
)

var errInvalidPayments = errors.New("invalid payments query")

type paymentsQuery struct {
	tab           string
	month         time.Time
	year          int
	withGregorian bool
}

func parsePaymentsQuery(query url.Values, start time.Time) (paymentsQuery, error) {
	p := paymentsQuery{
		tab:   query.Get("payments"),
		month: time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, intentions.TimeZone),
		year:  start.Year(),

		withGregorian: query.Get("greg") == "1",
	}

	switch p.tab {
	case paymentsWeek:
	case paymentsMonth:
		if param := query.Get("month"); param != "" {
			month, err := time.ParseInLocation(monthLayout, param, intentions.TimeZone)
			if err != nil || month.Year() < 1000 {
				return paymentsQuery{}, errInvalidPayments
			}
			p.month = month
		}
	case paymentsYear:
		if param := query.Get("year"); param != "" {
			year, err := strconv.Atoi(param)
			if err != nil || year < 1000 || year > 9999 {
				return paymentsQuery{}, errInvalidPayments
			}
			p.year = year
		}
	default:
		return paymentsQuery{}, errInvalidPayments
	}

	return p, nil
}

func (p paymentsQuery) span(start time.Time) (time.Time, time.Time) {
	switch p.tab {
	case paymentsMonth:
		return p.month, p.month.AddDate(0, 1, 0)
	case paymentsYear:
		first := time.Date(p.year, time.January, 1, 0, 0, 0, 0, intentions.TimeZone)
		return first, first.AddDate(1, 0, 0)
	default:
		return start, start.AddDate(0, 0, 7)
	}
}

type paymentsView struct {
	Tab         string
	Title       string
	FirstColumn string
	PrevURL     string
	PrevLabel   string
	NextURL     string
	NextLabel   string
	WeekTabURL  string
	MonthTabURL string
	YearTabURL  string
	CloseURL    string
	OpenWeekURL string
	UnpaidURL   string

	Week          string
	Month         string
	Year          string
	WithGregorian bool

	Total  intentions.Grosze
	Count  int
	Unpaid int
	Paid   int
	Rows   []paymentsRow

	GregorianAmount intentions.Grosze
	GregorianPaid   int
}

type paymentsRow struct {
	Label  string
	URL    string
	Amount intentions.Grosze
	Count  int
	Unpaid int
	Paid   int
	Width  string

	GregorianAmount intentions.Grosze
	GregorianPaid   int

	IsCurrent bool
}

func (r paymentsRow) Title() string {
	title := r.Label + ": " + r.Amount.String() + ", " + intentionsCount(r.Count)
	if r.GregorianAmount > 0 {
		title += ", w tym gregorianki " + r.GregorianAmount.String()
	}

	return title
}

func (v paymentsView) Average() intentions.Grosze {
	return v.average(v.WithGregorian)
}

func (v paymentsView) OtherAverage() intentions.Grosze {
	return v.average(!v.WithGregorian)
}

func (v paymentsView) average(withGregorian bool) intentions.Grosze {
	total, paid := v.Total, v.Paid
	if !withGregorian {
		total -= v.GregorianAmount
		paid -= v.GregorianPaid
	}

	return roundedAverage(total, paid)
}

func roundedAverage(total intentions.Grosze, count int) intentions.Grosze {
	paid := int64(count)
	if paid <= 0 || total <= 0 {
		return 0
	}

	zloty := (int64(total)*2 + paid*100) / (paid * 200)

	return intentions.Grosze(zloty * 100)
}

func (v paymentsView) GregorianLabel() string {
	n := v.GregorianPaid
	switch {
	case n == 1:
		return "1 seria"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d serie", n)
	default:
		return fmt.Sprintf("%d serii", n)
	}
}

func newPaymentsView(p paymentsQuery, current, today time.Time, counts map[string]intentions.DayCount) paymentsView {
	week := current.Format(intentions.DateLayout)
	view := paymentsView{
		Tab:           p.tab,
		WeekTabURL:    paymentsURL(week, paymentsWeek, ""),
		MonthTabURL:   paymentsURL(week, paymentsMonth, ""),
		YearTabURL:    paymentsURL(week, paymentsYear, ""),
		CloseURL:      weekURL(current),
		Week:          week,
		WithGregorian: p.withGregorian,
	}

	switch p.tab {
	case paymentsMonth:
		view.Title = monthNames[p.month.Month()-1] + " " + strconv.Itoa(p.month.Year())
		view.FirstColumn = "Tydzień"
		view.PrevURL = paymentsURL(week, paymentsMonth, p.month.AddDate(0, -1, 0).Format(monthLayout))
		view.PrevLabel = "Poprzedni miesiąc"
		view.NextURL = paymentsURL(week, paymentsMonth, p.month.AddDate(0, 1, 0).Format(monthLayout))
		view.NextLabel = "Następny miesiąc"
		view.Rows = monthRows(p.month, current, counts)
		view.Month = p.month.Format(monthLayout)
	case paymentsYear:
		view.Title = strconv.Itoa(p.year)
		view.FirstColumn = "Miesiąc"
		view.PrevURL = paymentsURL(week, paymentsYear, strconv.Itoa(p.year-1))
		view.PrevLabel = "Poprzedni rok"
		view.NextURL = paymentsURL(week, paymentsYear, strconv.Itoa(p.year+1))
		view.NextLabel = "Następny rok"
		view.Rows = yearRows(p.year, current, counts)
		view.Year = strconv.Itoa(p.year)
	default:
		view.Title = weekInWords(current)
		view.FirstColumn = "Dzień"
		view.PrevURL = paymentsURL(current.AddDate(0, 0, -7).Format(intentions.DateLayout), paymentsWeek, "")
		view.PrevLabel = "Poprzedni tydzień"
		view.NextURL = paymentsURL(current.AddDate(0, 0, 7).Format(intentions.DateLayout), paymentsWeek, "")
		view.NextLabel = "Następny tydzień"
		view.OpenWeekURL = weekURL(current)
		view.Rows = weekRows(current, today, counts)
	}

	var top intentions.Grosze
	for _, row := range view.Rows {
		view.Total += row.Amount
		view.Count += row.Count
		view.Unpaid += row.Unpaid
		view.Paid += row.Paid
		view.GregorianAmount += row.GregorianAmount
		view.GregorianPaid += row.GregorianPaid
		top = max(top, row.Amount)
	}
	for i := range view.Rows {
		view.Rows[i].Width = barWidth(view.Rows[i].Amount, top)
	}

	if p.withGregorian {
		for _, link := range []*string{&view.WeekTabURL, &view.MonthTabURL, &view.YearTabURL, &view.PrevURL, &view.NextURL} {
			*link += "&greg=1"
		}
		for i := range view.Rows {
			if view.Rows[i].URL != "" {
				view.Rows[i].URL += "&greg=1"
			}
		}
	}

	return view
}

func weekRows(start, today time.Time, counts map[string]intentions.DayCount) []paymentsRow {
	todayDate := today.In(intentions.TimeZone).Format(intentions.DateLayout)

	rows := make([]paymentsRow, 0, 7)
	for i := range 7 {
		day := start.AddDate(0, 0, i)
		row := sumDays(day, day.AddDate(0, 0, 1), counts)
		row.Label = weekdayNames[day.Weekday()] + " " + day.Format("2.1")
		row.IsCurrent = day.Format(intentions.DateLayout) == todayDate
		rows = append(rows, row)
	}

	return rows
}

func monthRows(first, current time.Time, counts map[string]intentions.DayCount) []paymentsRow {
	next := first.AddDate(0, 1, 0)

	var rows []paymentsRow
	for weekStart := intentions.WeekStart(first); weekStart.Before(next); weekStart = weekStart.AddDate(0, 0, 7) {
		from := weekStart
		if from.Before(first) {
			from = first
		}
		to := weekStart.AddDate(0, 0, 7)
		if to.After(next) {
			to = next
		}

		row := sumDays(from, to, counts)
		row.Label = rangeLabel(from, to.AddDate(0, 0, -1))
		row.URL = paymentsURL(weekStart.Format(intentions.DateLayout), paymentsWeek, "")
		row.IsCurrent = weekStart.Equal(current)
		rows = append(rows, row)
	}

	return rows
}

func yearRows(year int, current time.Time, counts map[string]intentions.DayCount) []paymentsRow {
	week := current.Format(intentions.DateLayout)

	rows := make([]paymentsRow, 0, 12)
	for month := time.January; month <= time.December; month++ {
		first := time.Date(year, month, 1, 0, 0, 0, 0, intentions.TimeZone)
		row := sumDays(first, first.AddDate(0, 1, 0), counts)
		row.Label = monthNames[month-1]
		row.URL = paymentsURL(week, paymentsMonth, first.Format(monthLayout))
		row.IsCurrent = year == current.Year() && month == current.Month()
		rows = append(rows, row)
	}

	return rows
}

func sumDays(from, to time.Time, counts map[string]intentions.DayCount) paymentsRow {
	var row paymentsRow
	for day := from; day.Before(to); day = day.AddDate(0, 0, 1) {
		count := counts[day.Format(intentions.DateLayout)]
		row.Amount += count.Amount
		row.Count += count.Filled
		row.Unpaid += count.Unpaid
		row.Paid += count.Paid
		row.GregorianAmount += count.GregorianAmount
		row.GregorianPaid += count.GregorianPaid
	}

	return row
}

func rangeLabel(from, last time.Time) string {
	if from.Equal(last) {
		return from.Format("2.1")
	}

	return from.Format("2") + " – " + last.Format("2.1")
}

func barWidth(amount, top intentions.Grosze) string {
	if top <= 0 || amount <= 0 {
		return "0%"
	}

	return strconv.FormatFloat(float64(amount)*100/float64(top), 'f', 1, 64) + "%"
}

func paymentsURL(week, tab, period string) string {
	target := "/?week=" + week + "&payments=" + tab
	switch {
	case period == "":
		return target
	case tab == paymentsMonth:
		return target + "&month=" + period
	case tab == paymentsYear:
		return target + "&year=" + period
	default:
		return target
	}
}

type weekPayments struct {
	Total      intentions.Grosze
	Count      int
	Unpaid     int
	URL        string
	UnpaidURL  string
	ShowUnpaid bool

	firstUnpaid int
}

func (p weekPayments) Title() string {
	return fmt.Sprintf("Podsumowanie wpłat · kwota wpisana przy %d z %d intencji", p.Count-p.Unpaid, p.Count)
}

func (p weekPayments) UnpaidLabel() string {
	return fmt.Sprintf("%d bez kwoty", p.Unpaid)
}

func (v weekView) WeekPayments() *weekPayments {
	p := weekPayments{
		URL:        v.PaymentsURL(),
		ShowUnpaid: v.ShowUnpaid,
	}
	forEachIntention(v.Days, func(intention *intentionView) {
		if intention.IsPlaceholder {
			return
		}
		p.Count++
		p.Total += intention.Amount
		if intention.Amount == 0 && intention.SeriesStep <= 1 {
			p.Unpaid++
			if p.firstUnpaid == 0 {
				p.firstUnpaid = intention.ID
			}
		}
	})
	if p.Count == 0 {
		return nil
	}

	p.UnpaidURL = unpaidURL(v.Week(), p.firstUnpaid)
	if v.ShowUnpaid {
		p.UnpaidURL = v.WeekURL()
	}

	return &p
}

func (v weekView) PaymentsURL() string {
	return paymentsURL(v.Week(), paymentsWeek, "")
}

func unpaidURL(week string, firstID int) string {
	target := "/?week=" + week + "&unpaid=1"
	if firstID > 0 {
		target += "#i-" + strconv.Itoa(firstID)
	}

	return target
}

func markUnpaid(days []dayView) {
	forEachIntention(days, func(intention *intentionView) {
		intention.IsUnpaid = !intention.IsPlaceholder && intention.Amount == 0 && intention.SeriesStep <= 1
	})
}
