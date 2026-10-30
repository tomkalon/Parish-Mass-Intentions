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
	{value: "other-days", label: "Niedziele i dni powszednie", hint: "np. „XXXII Niedziela Zwykła”, „Wtorek XXXI tygodnia zwykłego”", kind: gcatholic.Other, skip: func(f *settings.FeastImport) *bool { return &f.SkipOtherDays }},
}

func skipped(f settings.FeastImport, kind gcatholic.Kind) bool {
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
	Saved      bool
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

func newSettingsView(setting settings.Setting, week string) settingsView {
	view := settingsView{
		BackURL:    "/",
		ParishName: setting.ParishName,
		Address:    setting.Address,
	}
	if day, err := time.ParseInLocation(intentions.DateLayout, week, intentions.TimeZone); err == nil {
		view.Week = intentions.WeekStart(day).Format(intentions.DateLayout)
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

	return view
}

func settingsFromForm(form url.Values) settings.Setting {
	setting := settings.Setting{
		ParishName: strings.TrimSpace(form.Get("parish_name")),
		Address:    strings.TrimSpace(form.Get("address")),
	}

	for _, k := range feastKinds {
		*k.skip(&setting.FeastImport) = !slices.Contains(form["import"], k.value)
	}

	return setting
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
