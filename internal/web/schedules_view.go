package web

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

const maxScheduleNameLength = 100

var formWeekdays = []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}

type scheduleItem struct {
	Name      string
	Summary   string
	EditURL   string
	DeleteURL string
}

type settingsDialog struct {
	Title    string
	Text     string
	Action   string
	Button   string
	Week     string
	CloseURL string
}

func scheduleItems(schedules []masses.Schedule, week string) []scheduleItem {
	items := make([]scheduleItem, 0, len(schedules))
	for _, s := range schedules {
		items = append(items, scheduleItem{
			Name:      s.Name,
			Summary:   scheduleSummary(s),
			EditURL:   withWeek("/settings/schedules/"+strconv.Itoa(s.ID), week),
			DeleteURL: withWeek("/settings?delete_schedule="+strconv.Itoa(s.ID), week),
		})
	}

	return items
}

func scheduleSummary(s masses.Schedule) string {
	var parts []string
	for i := 0; i < len(formWeekdays); {
		times := s.Masses[formWeekdays[i]]
		j := i + 1
		for j < len(formWeekdays) && slices.Equal(s.Masses[formWeekdays[j]], times) {
			j++
		}

		if len(times) > 0 {
			label := weekdayShortNames[formWeekdays[i]]
			if j-i > 1 {
				label += "–" + weekdayShortNames[formWeekdays[j-1]]
			}
			parts = append(parts, label+": "+displayTimes(times))
		}
		i = j
	}

	if len(parts) == 0 {
		return "bez mszy"
	}

	return strings.Join(parts, " · ")
}

func displayTimes(times []string) string {
	shown := make([]string, 0, len(times))
	for _, t := range times {
		shown = append(shown, strings.TrimPrefix(t, "0"))
	}

	return strings.Join(shown, ", ")
}

type scheduleFormView struct {
	Title   string
	Action  string
	Week    string
	BackURL string
	Name    string
	Days    []scheduleDay
	Errors  map[string]string
}

type scheduleDay struct {
	Field string
	Label string
	Value string
	Error string
}

func (v scheduleFormView) MaxLength() int {
	return maxScheduleNameLength
}

func newScheduleFormView(s masses.Schedule, week string) scheduleFormView {
	week = normalizeWeek(week)
	view := scheduleFormView{
		Title:   "Nowy porządek mszy",
		Action:  "/settings/schedules",
		Week:    week,
		BackURL: settingsURL(week, false) + "#porzadki",
		Name:    s.Name,
	}
	if s.ID != 0 {
		view.Title = "Zmień porządek mszy"
		view.Action = "/settings/schedules/" + strconv.Itoa(s.ID)
	}

	for _, day := range formWeekdays {
		view.Days = append(view.Days, scheduleDay{
			Field: dayField(day),
			Label: weekdayNames[day],
			Value: displayTimes(s.Masses[day]),
		})
	}

	return view
}

func scheduleFromForm(form url.Values) (masses.Schedule, map[string]string) {
	s := masses.Schedule{Name: strings.TrimSpace(form.Get("name"))}
	errs := make(map[string]string)

	switch {
	case s.Name == "":
		errs["name"] = intentions.RequiredMessage
	case utf8.RuneCountInString(s.Name) > maxScheduleNameLength:
		errs["name"] = fmt.Sprintf("Maksymalnie %d znaków.", maxScheduleNameLength)
	}

	for _, day := range formWeekdays {
		times, err := masses.ParseDay(form.Get(dayField(day)))
		if err != nil {
			errs[dayField(day)] = timeErrorMessage(err)
			continue
		}
		s.Masses[day] = times
	}

	return s, errs
}

func timeErrorMessage(err error) string {
	var timeErr *masses.TimeError
	if !errors.As(err, &timeErr) {
		return "Nieprawidłowe godziny."
	}

	switch {
	case errors.Is(err, masses.ErrNotQuarterHour):
		return fmt.Sprintf("Godzina „%s” nie jest pełnym kwadransem (:00, :15, :30, :45).", timeErr.Value)
	case errors.Is(err, masses.ErrDuplicateTime):
		return fmt.Sprintf("Godzina „%s” jest wpisana dwa razy.", timeErr.Value)
	default:
		return fmt.Sprintf("Nieprawidłowa godzina „%s” – wpisz np. 6:30.", timeErr.Value)
	}
}

func dayField(day time.Weekday) string {
	return "day-" + strconv.Itoa(int(day))
}

func withWeek(path, week string) string {
	if week == "" {
		return path
	}

	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}

	return path + separator + "week=" + url.QueryEscape(week)
}
