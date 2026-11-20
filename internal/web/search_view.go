package web

import (
	"log"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/calendar"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

const (
	searchPageDays = 30
	maxSearchDays  = 400
	manyIntentions = 3
)

type searchPeriod struct {
	ID     string
	Label  string
	months int
}

var searchPeriods = []searchPeriod{
	{ID: "m1", Label: "Miesiąc", months: 1},
	{ID: "m3", Label: "3 miesiące", months: 3},
	{ID: "m6", Label: "Pół roku", months: 6},
	{ID: "y1", Label: "Rok", months: 12},
}

const (
	defaultSearchPeriod = "m3"
	customPeriod        = "date"
	maxSearchMonths     = 24
	maxSearchText       = 100
)

var firstSearchDay = time.Date(2000, 1, 1, 0, 0, 0, 0, intentions.TimeZone)

var countLabels = [...]string{"Wolna – bez intencji", "1 intencja", "2 intencje", "3 i więcej"}

var countShortLabels = [...]string{"Wolne", "1 intencja", "2 intencje", "3+ intencji"}

var countKinds = [...]string{"free", "one", "two", "many"}

var weekdayOrder = [...]time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

var weekdayFacetNames = [...]string{"Niedz.", "Pon.", "Wt.", "Śr.", "Czw.", "Pt.", "Sob."}

type searchQuery struct {
	Week          string
	Period        string
	Counts        []int
	Weekdays      []int
	Times         []string
	SkipGregorian bool
	Planned       bool
	ScheduleID    int
	Show          int
	Calendar      bool
	Day           string
	Text          string
	From          string
	Until         string
}

func parseSearchQuery(query url.Values, now time.Time, selectedSchedule int, schedules []masses.Schedule) searchQuery {
	q := searchQuery{
		Week:   intentions.NextWeekStart(now).Format(intentions.DateLayout),
		Period: defaultSearchPeriod,
		Show:   searchPageDays,
	}

	if day, err := time.ParseInLocation(intentions.DateLayout, query.Get("week"), intentions.TimeZone); err == nil {
		q.Week = intentions.WeekStart(day).Format(intentions.DateLayout)
	}
	if slices.ContainsFunc(searchPeriods, func(p searchPeriod) bool { return p.ID == query.Get("period") }) {
		q.Period = query.Get("period")
	}
	today := startOfDay(now)
	if from, err := time.ParseInLocation(intentions.DateLayout, query.Get("from"), intentions.TimeZone); err == nil {
		from = clampDay(from, firstSearchDay, today.AddDate(0, maxSearchMonths, -1))
		if !from.Equal(today) {
			q.From = from.Format(intentions.DateLayout)
		}
	}
	if until, err := time.ParseInLocation(intentions.DateLayout, query.Get("until"), intentions.TimeZone); err == nil && query.Get("period") == customPeriod {
		start := q.start(today)
		q.Period = customPeriod
		q.Until = clampDay(until, start, start.AddDate(0, maxSearchMonths, -1)).Format(intentions.DateLayout)
	}
	if text := []rune(strings.TrimSpace(query.Get("q"))); len(text) > 0 {
		q.Text = string(text[:min(len(text), maxSearchText)])
	}

	for _, value := range query["n"] {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 && n <= manyIntentions && !slices.Contains(q.Counts, n) {
			q.Counts = append(q.Counts, n)
		}
	}
	for _, value := range query["dow"] {
		if n, err := strconv.Atoi(value); err == nil && n >= 0 && n <= 6 && !slices.Contains(q.Weekdays, n) {
			q.Weekdays = append(q.Weekdays, n)
		}
	}
	for _, value := range query["time"] {
		if at, err := time.Parse("15:04", value); err == nil && !slices.Contains(q.Times, at.Format("15:04")) {
			q.Times = append(q.Times, at.Format("15:04"))
		}
	}
	slices.Sort(q.Counts)
	slices.Sort(q.Weekdays)
	slices.Sort(q.Times)

	explicit := false
	for key := range query {
		if key != "week" && key != "view" && key != "day" {
			explicit = true
		}
	}
	if explicit {
		q.SkipGregorian = query.Get("skipgreg") == "1"
		q.Planned = query.Get("planned") == "1"
	} else {
		q.Counts = []int{0}
		q.SkipGregorian = true
	}

	if show, err := strconv.Atoi(query.Get("show")); err == nil {
		q.Show = min(max(show, searchPageDays), maxSearchDays)
	}
	q.Calendar = query.Get("view") == "calendar"
	if _, err := time.ParseInLocation(intentions.DateLayout, query.Get("day"), intentions.TimeZone); err == nil && q.Calendar {
		q.Day = query.Get("day")
	}

	q.ScheduleID = chooseSchedule(query.Get("schedule"), selectedSchedule, schedules)

	return q
}

func chooseSchedule(param string, selected int, schedules []masses.Schedule) int {
	has := func(id int) bool {
		return slices.ContainsFunc(schedules, func(s masses.Schedule) bool { return s.ID == id })
	}

	if id, err := strconv.Atoi(param); err == nil && has(id) {
		return id
	}
	if has(selected) {
		return selected
	}
	if len(schedules) > 0 {
		return schedules[0].ID
	}

	return 0
}

func (q searchQuery) values() url.Values {
	values := url.Values{}
	values.Set("week", q.Week)
	values.Set("period", q.Period)
	for _, n := range q.Counts {
		values.Add("n", strconv.Itoa(n))
	}
	for _, d := range q.Weekdays {
		values.Add("dow", strconv.Itoa(d))
	}
	for _, t := range q.Times {
		values.Add("time", t)
	}
	if q.SkipGregorian {
		values.Set("skipgreg", "1")
	}
	if q.Planned {
		values.Set("planned", "1")
	}
	if q.ScheduleID > 0 {
		values.Set("schedule", strconv.Itoa(q.ScheduleID))
	}
	if q.Show != searchPageDays {
		values.Set("show", strconv.Itoa(q.Show))
	}
	if q.Calendar {
		values.Set("view", "calendar")
		if q.Day != "" {
			values.Set("day", q.Day)
		}
	}
	if q.From != "" {
		values.Set("from", q.From)
	}
	if q.Until != "" {
		values.Set("until", q.Until)
	}
	if q.Text != "" {
		values.Set("q", q.Text)
	}

	return values
}

func (q searchQuery) url(edit func(q *searchQuery)) string {
	next := q
	next.Counts = slices.Clone(q.Counts)
	next.Weekdays = slices.Clone(q.Weekdays)
	next.Times = slices.Clone(q.Times)
	next.Show = searchPageDays
	next.Day = ""
	if edit != nil {
		edit(&next)
	}

	return "/search?" + next.values().Encode()
}

func (q searchQuery) start(today time.Time) time.Time {
	if from, err := time.ParseInLocation(intentions.DateLayout, q.From, intentions.TimeZone); err == nil {
		return from
	}

	return today
}

func (q searchQuery) end(today time.Time) time.Time {
	if until, err := time.ParseInLocation(intentions.DateLayout, q.Until, intentions.TimeZone); err == nil {
		return until.AddDate(0, 0, 1)
	}
	months := searchPeriods[1].months
	for _, p := range searchPeriods {
		if p.ID == q.Period {
			months = p.months
		}
	}

	return q.start(today).AddDate(0, months, 0)
}

func clampDay(day, first, last time.Time) time.Time {
	if day.Before(first) {
		return first
	}
	if day.After(last) {
		return last
	}
	return day
}

func startOfDay(t time.Time) time.Time {
	t = t.In(intentions.TimeZone)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, intentions.TimeZone)
}

func countBucket(parish int) int {
	return min(parish, manyIntentions)
}

func (q searchQuery) matches(m masses.Mass, skip string) bool {
	if m.Planned && !q.Planned {
		return false
	}
	if q.SkipGregorian && m.Gregorian {
		return false
	}
	if skip != "n" && len(q.Counts) > 0 && !slices.Contains(q.Counts, countBucket(m.Parish)) {
		return false
	}
	if skip != "dow" && len(q.Weekdays) > 0 && !slices.Contains(q.Weekdays, int(m.At.Weekday())) {
		return false
	}
	if skip != "time" && len(q.Times) > 0 && !slices.Contains(q.Times, m.At.Format("15:04")) {
		return false
	}

	return true
}

type searchView struct {
	Topbar      topbarView
	BackURL     string
	DefaultsURL string
	Saved       *searchSaved
	Week        string
	RangeText   string

	Periods       []searchOption
	Counts        []facetOption
	Weekdays      []facetOption
	Times         []facetOption
	SkipGregorian bool
	Planned       bool
	Schedules     []searchOption

	Total    string
	Subtotal string
	Active   []activeFilter
	ClearURL string
	Months   []searchMonth
	MoreURL  string
	More     string
	Empty    bool

	IsCalendar  bool
	ListURL     string
	CalendarURL string
	CalMonths   []searchCalMonth
	Preview     *searchPreview

	CustomPeriod bool
	Today        string
	From         string
	FromMin      string
	FromMax      string
	FromLabel    string
	FromRangeEnd string
	FromTodayURL string
	Until        string
	UntilMin     string
	UntilMax     string
	UntilLabel   string

	Text         string
	TextHint     string
	TextMode     bool
	TextFields   []hiddenField
	TextClearURL string
	HitSections  []textHitSection
}

type searchCalMonth struct {
	Name  string
	Count string
	Cells []searchCalCell
}

type searchCalCell struct {
	Num   int
	URL   string
	Label string
	Tip   *dayTip
	Bars  []searchBar

	IsOut     bool
	HasMasses bool
	Matches   bool
	Selected  bool
	IsToday   bool
}

type searchBar struct {
	Kind string
}

type searchPreview struct {
	Label   string
	Feast   string
	WeekURL string
	Stats   dayStats
	Rows    []searchPreviewRow
}

type searchPreviewRow struct {
	searchMass
	Note    string
	Matched bool
}

type searchOption struct {
	Value    string
	Label    string
	Selected bool
}

type facetOption struct {
	ID      string
	Value   string
	Label   string
	Kind    string
	Count   int
	Checked bool
}

type activeFilter struct {
	Label string
	URL   string
}

type searchMonth struct {
	Name  string
	Count string
	Days  []searchDay
}

type searchDay struct {
	Label   string
	Weekday string
	Feast   string
	URL     string
	Stats   dayStats
	Planned string
	Masses  []searchMass

	IsSunday bool
}

type searchMass struct {
	Time    string
	Status  string
	Outside string
	Kind    string
	URL     string
	Title   string
}

func newSearchFrame(q searchQuery, from, to, today time.Time) searchView {
	last := to.AddDate(0, 0, -1)
	view := searchView{
		IsCalendar:    q.Calendar,
		ListURL:       q.url(func(q *searchQuery) { q.Calendar = false }),
		CalendarURL:   q.url(func(q *searchQuery) { q.Calendar = true }),
		BackURL:       "/?week=" + q.Week,
		Week:          q.Week,
		RangeText:     "od " + from.Format("2.1.2006") + " do " + last.Format("2.1.2006"),
		SkipGregorian: q.SkipGregorian,
		Planned:       q.Planned,
		ClearURL: q.url(func(q *searchQuery) {
			q.Counts, q.Weekdays, q.Times = nil, nil, nil
		}),
		CustomPeriod: q.Period == customPeriod,
		Today:        today.Format(intentions.DateLayout),
		From:         from.Format(intentions.DateLayout),
		FromMin:      firstSearchDay.Format(intentions.DateLayout),
		FromMax:      today.AddDate(0, maxSearchMonths, -1).Format(intentions.DateLayout),
		FromLabel:    from.Format("2.1.2006"),
		Until:        last.Format(intentions.DateLayout),
		UntilMin:     from.Format(intentions.DateLayout),
		UntilMax:     from.AddDate(0, maxSearchMonths, -1).Format(intentions.DateLayout),
		Text:         q.Text,
		TextFields:   textFields(q),
		TextClearURL: q.url(func(q *searchQuery) { q.Text = "" }),
	}
	if view.CustomPeriod {
		view.UntilLabel = last.Format("2.1.2006")
		view.FromRangeEnd = view.Until
	}
	if q.From == "" {
		view.FromLabel = "dziś, " + view.FromLabel
	} else {
		view.FromTodayURL = q.url(func(q *searchQuery) { q.From = "" })
	}
	if q.Text != "" && !intentions.SearchReady(q.Text) {
		view.TextHint = "Wpisz co najmniej " + strconv.Itoa(intentions.MinSearchLength) + " znaki, aby szukać w treści intencji."
	}
	for _, p := range searchPeriods {
		view.Periods = append(view.Periods, searchOption{Value: p.ID, Label: p.Label, Selected: p.ID == q.Period})
	}

	return view
}

func newSearchView(q searchQuery, list []masses.Mass, feasts map[string]string, schedules []masses.Schedule, from, to, today time.Time) searchView {
	view := newSearchFrame(q, from, to, today)

	scheduleName := ""
	for _, s := range schedules {
		view.Schedules = append(view.Schedules, searchOption{Value: strconv.Itoa(s.ID), Label: s.Name, Selected: s.ID == q.ScheduleID})
		if s.ID == q.ScheduleID {
			scheduleName = s.Name
		}
	}
	count := func(skip string, test func(m masses.Mass) bool) int {
		n := 0
		for _, m := range list {
			if q.matches(m, skip) && test(m) {
				n++
			}
		}
		return n
	}

	for n := 0; n <= manyIntentions; n++ {
		view.Counts = append(view.Counts, facetOption{
			ID:      "f-n-" + strconv.Itoa(n),
			Value:   strconv.Itoa(n),
			Label:   countLabels[n],
			Kind:    countKinds[n],
			Count:   count("n", func(m masses.Mass) bool { return countBucket(m.Parish) == n }),
			Checked: slices.Contains(q.Counts, n),
		})
	}
	for _, d := range weekdayOrder {
		view.Weekdays = append(view.Weekdays, facetOption{
			ID:      "f-dow-" + strconv.Itoa(int(d)),
			Value:   strconv.Itoa(int(d)),
			Label:   weekdayFacetNames[d],
			Count:   count("dow", func(m masses.Mass) bool { return m.At.Weekday() == d }),
			Checked: slices.Contains(q.Weekdays, int(d)),
		})
	}
	for _, t := range searchTimes(list, q.Times) {
		at, _ := time.Parse("15:04", t)
		view.Times = append(view.Times, facetOption{
			ID:      "f-time-" + strings.ReplaceAll(t, ":", ""),
			Value:   t,
			Label:   clockLabel(at),
			Count:   count("time", func(m masses.Mass) bool { return m.At.Format("15:04") == t }),
			Checked: slices.Contains(q.Times, t),
		})
	}

	for _, n := range q.Counts {
		view.Active = append(view.Active, activeFilter{Label: countShortLabels[n], URL: q.url(func(q *searchQuery) {
			q.Counts = slices.DeleteFunc(q.Counts, func(v int) bool { return v == n })
		})})
	}
	for _, d := range q.Weekdays {
		view.Active = append(view.Active, activeFilter{Label: weekdayNames[d], URL: q.url(func(q *searchQuery) {
			q.Weekdays = slices.DeleteFunc(q.Weekdays, func(v int) bool { return v == d })
		})})
	}
	for _, t := range q.Times {
		at, _ := time.Parse("15:04", t)
		view.Active = append(view.Active, activeFilter{Label: "godz. " + clockLabel(at), URL: q.url(func(q *searchQuery) {
			q.Times = slices.DeleteFunc(q.Times, func(v string) bool { return v == t })
		})})
	}

	byDay := make(map[string][]masses.Mass)
	var matchedDays []string
	matchedByDay := make(map[string][]masses.Mass)
	total := 0
	for _, m := range list {
		date := m.At.Format(intentions.DateLayout)
		byDay[date] = append(byDay[date], m)
		if !q.matches(m, "") {
			continue
		}
		if _, ok := matchedByDay[date]; !ok {
			matchedDays = append(matchedDays, date)
		}
		matchedByDay[date] = append(matchedByDay[date], m)
		total++
	}

	view.Empty = total == 0
	view.Total = "Znaleziono " + countOf(total, "Mszę", "Msze", "Mszy")
	if len(matchedDays) > 0 {
		view.Subtotal = "w " + countOf(len(matchedDays), "dniu", "dniach", "dniach")
	}

	if q.Calendar {
		view.CalMonths = newSearchCalendar(q, from, to, today, byDay, matchedByDay)
		view.Preview = newSearchPreview(q, byDay, matchedDays, feasts)
		return view
	}

	shown := matchedDays[:min(len(matchedDays), q.Show)]
	for _, date := range shown {
		day := matchedByDay[date][0].At
		name := monthNames[day.Month()-1] + " " + strconv.Itoa(day.Year())
		if len(view.Months) == 0 || view.Months[len(view.Months)-1].Name != name {
			view.Months = append(view.Months, searchMonth{Name: name})
		}
		month := &view.Months[len(view.Months)-1]
		month.Days = append(month.Days, newSearchDay(day, byDay[date], matchedByDay[date], feasts[date], scheduleName))
	}
	for i := range view.Months {
		n := 0
		for _, d := range view.Months[i].Days {
			n += len(d.Masses)
		}
		view.Months[i].Count = countOf(n, "Msza", "Msze", "Mszy")
	}

	if rest := len(matchedDays) - len(shown); rest > 0 {
		view.MoreURL = q.url(func(next *searchQuery) {
			next.Show = min(q.Show+searchPageDays, maxSearchDays)
		})
		view.More = "Pokaż kolejne dni (" + strconv.Itoa(rest) + ")"
		if q.Show >= maxSearchDays {
			view.MoreURL = ""
			view.More = "Pokazano pierwsze " + strconv.Itoa(maxSearchDays) + " dni – zawęź wyszukiwanie, aby zobaczyć pozostałe (" + strconv.Itoa(rest) + ")."
		}
	}

	return view
}

func searchTimes(list []masses.Mass, selected []string) []string {
	times := slices.Clone(selected)
	for _, m := range list {
		t := m.At.Format("15:04")
		if !slices.Contains(times, t) {
			times = append(times, t)
		}
	}
	slices.Sort(times)

	return times
}

func newSearchDay(day time.Time, all, matched []masses.Mass, feast, scheduleName string) searchDay {
	week := intentions.WeekStart(day).Format(intentions.DateLayout)
	date := day.Format(intentions.DateLayout)

	result := searchDay{
		Label:    weekdayShortNames[day.Weekday()] + " " + day.Format("2.1"),
		Weekday:  weekdayNames[day.Weekday()],
		Feast:    feast,
		URL:      "/?week=" + week + "#d-" + date,
		Stats:    dayStats{Masses: len(all)},
		IsSunday: day.Weekday() == time.Sunday,
	}

	for _, m := range all {
		result.Stats.Intentions += m.Parish
		result.Stats.Outside += m.Outside
		if m.Free() {
			result.Stats.Free++
		}
		if m.Planned && scheduleName != "" {
			result.Planned = "godziny wg porządku „" + scheduleName + "”"
		}
	}

	for _, m := range matched {
		result.Masses = append(result.Masses, newSearchMass(m, week, date))
	}

	return result
}

func newSearchMass(m masses.Mass, week, date string) searchMass {
	clock := clockLabel(m.At)
	mass := searchMass{
		Time:    clock,
		Outside: outsideLabel(m.Outside, m.Parish > 0),
		URL:     searchMassURL(week, date, m.At.Format("15:04")),
		Title:   "Dodaj intencję: " + strings.ToLower(weekdayNames[m.At.Weekday()]) + " " + m.At.Format("2.1") + ", godz. " + clock,
		Kind:    countKinds[countBucket(m.Parish)],
	}

	switch {
	case m.Planned:
		mass.Status = "planowana"
		mass.Kind = "planned"
	case m.Free():
		mass.Status = "wolna"
	default:
		mass.Status = intentionsCount(m.Parish)
		if m.Gregorian {
			mass.Status += " · gregorianka"
		}
	}

	return mass
}

func (h *handler) search(w http.ResponseWriter, r *http.Request) {
	setting, err := settings.Load(h.db)
	if err != nil {
		log.Printf("error loading settings: %v", err)
	}

	schedules, err := masses.Load(h.db)
	if err != nil {
		log.Printf("error loading schedules: %v", err)
		serverError(w)
		return
	}

	q := parseSearchQuery(r.URL.Query(), h.now(), setting.SelectedScheduleID, schedules)
	h.searches.remember(q)
	defaults := parseSearchQuery(url.Values{"week": {q.Week}}, h.now(), setting.SelectedScheduleID, schedules)
	finish := func(view *searchView) {
		view.Topbar = newTopbarView(setting, h.backups.State(), h.now())
		view.Topbar.Draft = h.draftNotice()
		view.Saved = h.searchSaved(r.URL.Query().Get("saved"))
		if q.values().Encode() != defaults.values().Encode() {
			view.DefaultsURL = "/search?week=" + q.Week
		}
	}

	now := h.now().In(intentions.TimeZone)
	today := startOfDay(now)
	start := q.start(today)
	to := q.end(today)

	if intentions.SearchReady(q.Text) {
		hits, err := intentions.Search(h.db, q.Text)
		if err != nil {
			log.Printf("error searching intentions: %v", err)
			serverError(w)
			return
		}
		upcoming, past := splitTextHits(hits, now)
		loaded, err := intentions.ByIDs(h.db, shownTextHits(q.Show, upcoming, past))
		if err != nil {
			log.Printf("error loading found intentions: %v", err)
			serverError(w)
			return
		}

		view := newSearchFrame(q, start, to, today)
		addTextSearch(&view, q, upcoming, past, loaded)
		finish(&view)
		render(w, http.StatusOK, searchTemplate, view)
		return
	}

	var plan *masses.Schedule
	if q.Planned {
		for i := range schedules {
			if schedules[i].ID == q.ScheduleID {
				plan = &schedules[i]
			}
		}
	}

	list, err := searchMasses(h.db, start, to, now, plan)
	if err != nil {
		log.Printf("error searching masses: %v", err)
		serverError(w)
		return
	}

	feasts, err := calendar.FeastsBetween(h.db, start, to)
	if err != nil {
		log.Printf("error loading feasts: %v", err)
		serverError(w)
		return
	}

	view := newSearchView(q, list, feasts, schedules, start, to, today)
	finish(&view)

	render(w, http.StatusOK, searchTemplate, view)
}

func searchMasses(db *gorm.DB, start, to, now time.Time, plan *masses.Schedule) ([]masses.Mass, error) {
	var list []masses.Mass
	if start.Before(startOfDay(now)) {
		past, err := masses.InRange(db, start, minTime(now, to), nil)
		if err != nil {
			return nil, err
		}
		list = past
	}

	from := now
	if start.After(now) {
		from = start
	}
	if from.Before(to) {
		upcoming, err := masses.InRange(db, from, to, plan)
		if err != nil {
			return nil, err
		}
		list = append(list, upcoming...)
	}

	return list, nil
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func newSearchCalendar(q searchQuery, from, to, now time.Time, byDay, matchedByDay map[string][]masses.Mass) []searchCalMonth {
	selected := q.Day
	today := now.Format(intentions.DateLayout)
	last := to.AddDate(0, 0, -1)

	var months []searchCalMonth
	for first := time.Date(from.Year(), from.Month(), 1, 0, 0, 0, 0, intentions.TimeZone); !first.After(last); first = first.AddDate(0, 1, 0) {
		month := searchCalMonth{Name: monthNames[first.Month()-1] + " " + strconv.Itoa(first.Year())}
		for range (int(first.Weekday()) + 6) % 7 {
			month.Cells = append(month.Cells, searchCalCell{IsOut: true})
		}

		matches := 0
		for day := first; day.Month() == first.Month(); day = day.AddDate(0, 0, 1) {
			date := day.Format(intentions.DateLayout)
			all, hits := byDay[date], matchedByDay[date]
			matches += len(hits)

			cell := searchCalCell{
				Num:       day.Day(),
				HasMasses: len(all) > 0,
				Matches:   len(hits) > 0,
				Selected:  date == selected,
				IsToday:   date == today,
				Tip:       newDayTip(day, all),
				URL: q.url(func(next *searchQuery) {
					next.Calendar = true
					next.Day = date
				}) + "#podglad",
				Label: weekdayNames[day.Weekday()] + " " + day.Format("2.1") + ": " + countOf(len(all), "Msza", "Msze", "Mszy") + ", pasuje " + strconv.Itoa(len(hits)),
			}
			for _, m := range all {
				kind := countKinds[countBucket(m.Parish)]
				if m.Planned {
					kind = "planned"
				}
				cell.Bars = append(cell.Bars, searchBar{Kind: kind})
			}
			month.Cells = append(month.Cells, cell)
		}

		month.Count = "brak pasujących Mszy"
		if matches > 0 {
			month.Count = countOf(matches, "pasująca Msza", "pasujące Msze", "pasujących Mszy")
		}
		months = append(months, month)
	}

	return months
}

func newSearchPreview(q searchQuery, byDay map[string][]masses.Mass, matchedDays []string, feasts map[string]string) *searchPreview {
	date := q.Day
	if len(byDay[date]) == 0 {
		if len(matchedDays) == 0 {
			return nil
		}
		date = matchedDays[0]
	}

	all := byDay[date]
	day := all[0].At
	week := intentions.WeekStart(day).Format(intentions.DateLayout)
	preview := &searchPreview{
		Label:   weekdayNames[day.Weekday()] + " " + day.Format("2.1.2006"),
		Feast:   feasts[date],
		WeekURL: "/?week=" + week + "#d-" + date,
		Stats:   massStats(all),
	}

	for _, m := range all {
		row := searchPreviewRow{searchMass: newSearchMass(m, week, date), Matched: q.matches(m, "")}
		switch {
		case m.Gregorian:
			row.Note = "gregorianka – nie dokładać intencji"
		case row.Matched:
			row.Note = "pasuje do filtrów"
		}
		preview.Rows = append(preview.Rows, row)
	}

	return preview
}
