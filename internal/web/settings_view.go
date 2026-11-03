package web

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"go-parafia/internal/gcatholic"
	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
	"go-parafia/internal/settings"
)

const maxSettingLength = 200

type feastKind struct {
	value string
	label string
	hint  string
	kind  gcatholic.Kind
	skip  func(f *settings.FeastImport) *bool
}

var feastKinds = []feastKind{
	{value: "solemnities", label: "Uroczystości", kind: gcatholic.Solemnity, skip: func(f *settings.FeastImport) *bool { return &f.SkipSolemnities }},
	{value: "feasts", label: "Święta", kind: gcatholic.Feast, skip: func(f *settings.FeastImport) *bool { return &f.SkipFeasts }},
	{value: "memorials", label: "Wspomnienia obowiązkowe", kind: gcatholic.Memorial, skip: func(f *settings.FeastImport) *bool { return &f.SkipMemorials }},
	{value: "optional-memorials", label: "Wspomnienia dowolne", kind: gcatholic.OptionalMemorial, skip: func(f *settings.FeastImport) *bool { return &f.SkipOptionalMemorials }},
	{value: "other-days", label: "Niedziele i dni szczególne", hint: "np. „XXXII Niedziela Zwykła”, „Środa Popielcowa”, „Wielki Czwartek”", kind: gcatholic.Other, skip: func(f *settings.FeastImport) *bool { return &f.SkipOtherDays }},
}

func skipped(f settings.FeastImport, kind gcatholic.Kind) bool {
	if kind == gcatholic.Weekday {
		return true
	}

	for _, k := range feastKinds {
		if k.kind == kind {
			return *k.skip(&f)
		}
	}

	return false
}

type settingsView struct {
	Week       string
	BackURL    string
	ParishName string
	Address    string
	Kinds      []kindOption
	Errors     map[string]string

	QuickAmounts string
	QuickPreview []string

	Saved   bool
	Notice  string
	DataDir string

	ImportError string

	BackupDir    string
	BackupLabel  string
	BackupError  string
	CanChooseDir bool

	Schedules      []scheduleItem
	NewScheduleURL string
	RestoreURL     string
	Dialog         *settingsDialog
	Topbar         topbarView
}

type kindOption struct {
	Value   string
	Label   string
	Hint    string
	Checked bool
}

func (v settingsView) MaxLength() int {
	return maxSettingLength
}

func (v settingsView) MaxQuickAmounts() int {
	return settings.MaxQuickAmounts
}

func (v settingsView) DefaultQuickAmounts() string {
	values := make([]string, 0, len(settings.DefaultQuickAmounts))
	for _, amount := range settings.DefaultQuickAmounts {
		values = append(values, amount.FormValue())
	}

	return strings.Join(values, ", ")
}

func newSettingsView(setting settings.Setting, schedules []masses.Schedule, week string) settingsView {
	view := settingsView{
		Week:       normalizeWeek(week),
		BackURL:    "/",
		ParishName: setting.ParishName,
		Address:    setting.Address,
	}
	if view.Week != "" {
		view.BackURL = "/?week=" + view.Week
	}

	for _, k := range feastKinds {
		view.Kinds = append(view.Kinds, kindOption{
			Value:   k.value,
			Label:   k.label,
			Hint:    k.hint,
			Checked: !*k.skip(&setting.FeastImport),
		})
	}

	var preview []string
	for _, amount := range setting.QuickAmounts {
		preview = append(preview, amount.FormValue())
	}
	view.QuickAmounts = strings.Join(preview, ", ")
	view.QuickPreview = preview

	view.Schedules = scheduleItems(schedules, view.Week)
	view.NewScheduleURL = withWeek("/settings/schedules/new", view.Week)
	view.RestoreURL = withWeek("/settings?restore=1", view.Week)

	return view
}

func normalizeWeek(week string) string {
	day, err := time.ParseInLocation(intentions.DateLayout, week, intentions.TimeZone)
	if err != nil {
		return ""
	}

	return intentions.WeekStart(day).Format(intentions.DateLayout)
}

func settingsFromForm(form url.Values) (settings.Setting, map[string]string) {
	setting := settings.Setting{
		ParishName: strings.TrimSpace(form.Get("parish_name")),
		Address:    strings.TrimSpace(form.Get("address")),
	}

	for _, k := range feastKinds {
		*k.skip(&setting.FeastImport) = !slices.Contains(form["import"], k.value)
	}

	errs := settingsErrors(setting)
	amounts, message := parseQuickAmounts(form.Get("quick_amounts"))
	if message != "" {
		errs["quick_amounts"] = message
	}
	setting.QuickAmounts = amounts

	return setting, errs
}

const quickAmountsMessage = "Wpisz kwoty w pełnych złotych, oddzielone przecinkami, np. 50, 100, 150."

func parseQuickAmounts(text string) ([]intentions.Grosze, string) {
	var amounts []intentions.Grosze
	for part := range strings.FieldsFuncSeq(text, func(r rune) bool { return r == ',' || r == ';' }) {
		if strings.TrimSpace(part) == "" {
			continue
		}

		amount, err := intentions.ParseGrosze(part)
		if err != nil || amount <= 0 || amount%100 != 0 {
			return nil, quickAmountsMessage
		}
		if !slices.Contains(amounts, amount) {
			amounts = append(amounts, amount)
		}
	}
	if len(amounts) > settings.MaxQuickAmounts {
		return nil, fmt.Sprintf("Podaj najwyżej %d kwot.", settings.MaxQuickAmounts)
	}

	return amounts, ""
}

func settingsErrors(setting settings.Setting) map[string]string {
	errs := make(map[string]string)

	if setting.ParishName == "" {
		errs["parish_name"] = intentions.RequiredMessage
	}
	tooLong := fmt.Sprintf("Maksymalnie %d znaków.", maxSettingLength)
	if utf8.RuneCountInString(setting.ParishName) > maxSettingLength {
		errs["parish_name"] = tooLong
	}
	if utf8.RuneCountInString(setting.Address) > maxSettingLength {
		errs["address"] = tooLong
	}

	return errs
}

func settingsURL(week string, saved bool) string {
	query := url.Values{}
	if week != "" {
		query.Set("week", week)
	}
	if saved {
		query.Set("saved", "1")
	}
	if len(query) == 0 {
		return "/settings"
	}

	return "/settings?" + query.Encode()
}
