package web

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

var seriesNow = time.Date(2026, 11, 3, 16, 0, 0, 0, intentions.TimeZone)

func seedSeries(t *testing.T, db *gorm.DB, content, firstMass string, days int, amount intentions.Grosze, skip ...string) gregorian.Series {
	t.Helper()

	series, err := gregorian.Create(db, content, false)
	if err != nil {
		t.Fatalf("creating series: %v", err)
	}
	first, err := time.ParseInLocation(intentions.MassAtLayout, firstMass, intentions.TimeZone)
	if err != nil {
		t.Fatal(err)
	}
	for day := first; days > 0; day = day.AddDate(0, 0, 1) {
		if slices.Contains(skip, day.Format(intentions.DateLayout)) {
			continue
		}
		intention := intentions.Intention{MassAt: day, Content: content, IsGregorian: true, IsDeceased: true, SeriesID: series.ID}
		if day.Equal(first) {
			intention.Amount = amount
		}
		err = db.Create(&intention).Error
		if err != nil {
			t.Fatalf("creating mass: %v", err)
		}
		days--
	}

	return series
}

func loadIntentionByID(t *testing.T, db *gorm.DB, id int) intentions.Intention {
	t.Helper()

	var intention intentions.Intention
	err := db.First(&intention, id).Error
	if err != nil {
		t.Fatalf("loading intention %d: %v", id, err)
	}

	return intention
}

func seriesAmounts(t *testing.T, db *gorm.DB, id int) []intentions.Grosze {
	t.Helper()

	var list []intentions.Grosze
	err := db.Model(&intentions.Intention{}).Where("series_id = ?", id).Order("mass_at, id").Pluck("amount", &list).Error
	if err != nil {
		t.Fatal(err)
	}

	return list
}

func TestCreateGregorianStartsSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	form := url.Values{
		"week":      {"2026-11-09"},
		"day":       {"2026-11-09"},
		"time":      {"18:00"},
		"content":   {"+ Jan Kowalski"},
		"gregorian": {"on"},
		"amount":    {"1500"},
	}

	rec := postToRouter(t, h.db, "/intentions", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	saved := loadIntentionByID(t, h.db, 1)
	if saved.SeriesID != 1 || saved.Amount != 150000 || !saved.IsDeceased {
		t.Errorf("saved = %+v, want series 1, amount 150000, za zmarłych", saved)
	}
	series, err := gregorian.Find(h.db, 1)
	if err != nil || series.Content != "+ Jan Kowalski" {
		t.Errorf("series = %+v, %v", series, err)
	}

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()
	checkBody(t, body,
		`<a class="badge badge-gregorian" href="/?week=2026-11-09&amp;gregorian=active&amp;series=1#s-1" title="Msza gregoriańska 1 z 30 – pokaż serię">`,
		`gregoriańska <span class="series-step">1/30</span></a>`,
		`<span class="amount" title="Ofiara za całą serię (30 Mszy)"><svg class="amount-icon"`,
		`</svg>1`+nbsp+`500`+nbsp+`zł</span>`,
		`<span class="count-badge" aria-hidden="true">1</span>`,
		`<a class="series-due" href="/?day=2026-11-10&amp;new=1&amp;time=18%3A00&amp;week=2026-11-09&amp;series=1" title="Dodaj kolejną Mszę gregoriańską na ten dzień">`,
	)
}

func TestFillingPlaceholdersBuildsSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedPlaceholder(t, h.db, "2026-11-09 18:00")
	seedPlaceholder(t, h.db, "2026-11-10 18:00")

	fill := "/?day=2026-11-09&new=1&time=18%3A00&week=2026-11-09"
	checkBody(t, getWeek(t, h, "/?week=2026-11-09").Body.String(),
		`<a class="action" href="/?day=2026-11-09&amp;new=1&amp;time=18%3A00&amp;week=2026-11-09" aria-label="Uzupełnij intencję z godz. 18:00">Uzupełnij</a>`,
	)
	checkBody(t, getWeek(t, h, fill).Body.String(),
		`<form class="panel-form" method="post" action="/intentions" data-new>`,
	)

	rec := postToRouter(t, h.db, "/intentions", url.Values{
		"week":      {"2026-11-09"},
		"day":       {"2026-11-09"},
		"time":      {"18:00"},
		"content":   {"+ Jan Kowalski"},
		"gregorian": {"on"},
		"amount":    {"1500"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	var saved intentions.Intention
	err := h.db.Where("is_placeholder = ?", false).First(&saved).Error
	if err != nil {
		t.Fatalf("loading the filled intention: %v", err)
	}
	if saved.SeriesID == 0 {
		t.Fatalf("filled intention = %+v, want it to start a series", saved)
	}
	var placeholders int64
	h.db.Model(&intentions.Intention{}).Where("is_placeholder = ?", true).Count(&placeholders)
	if placeholders != 1 {
		t.Errorf("placeholders = %d, want 1 (the one on 10.11 is left)", placeholders)
	}

	checkBody(t, getWeek(t, h, "/?day=2026-11-10&new=1&time=18%3A00&week=2026-11-09").Body.String(),
		`<input type="radio" id="series-1" name="series" value="1"`,
		`<span class="series-step-big">2/30</span>`,
		`<span class="series-meta is-due">na ten dzień · 18:00</span>`,
	)
}

func TestCopyOfOldGregorianDoesNotStartSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	form := url.Values{
		"week": {"2026-11-09"}, "day": {"2026-11-10"}, "time": {"18:00"},
		"content": {"+ Anna"}, "gregorian": {"on"}, "amount": {"100"}, "copy": {"1"},
	}

	rec := postToRouter(t, h.db, "/intentions", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if saved := loadIntentionByID(t, h.db, 1); saved.SeriesID != 0 || saved.Amount != 0 {
		t.Errorf("copy = %+v, want no series and no amount", saved)
	}
}

func TestContinueSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan Kowalski", "2026-11-07 18:00", 2, 150000)
	form := url.Values{
		"week": {"2026-11-09"}, "day": {"2026-11-09"}, "time": {"18:00"},
		"content": {"coś innego"}, "amount": {"200"}, "series": {"1"},
	}

	rec := postToRouter(t, h.db, "/intentions", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	saved := loadIntentionByID(t, h.db, 3)
	if saved.SeriesID != 1 || saved.Content != "+ Jan Kowalski" || saved.Amount != 0 || !saved.IsGregorian || !saved.IsDeceased {
		t.Errorf("continuation = %+v, want the series content, no amount, gregorian and za zmarłych", saved)
	}

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()
	checkBody(t, body,
		`gregoriańska <span class="series-step">3/30</span></a>`,
		`<span class="amount is-missing" title="Ofiara wpisana przy pierwszej Mszy serii">w serii</span>`,
	)
	if strings.Contains(body, `class="week-unpaid"`) {
		t.Error("a later mass of a paid series counts as bez kwoty")
	}
}

func TestContinueSeriesRejected(t *testing.T) {
	tests := []struct {
		name  string
		setup func(t *testing.T, db *gorm.DB)
		day   string
		want  string
	}{
		{
			name:  "series already has a mass that day",
			setup: func(t *testing.T, db *gorm.DB) { seedSeries(t, db, "+ Jan", "2026-11-07 18:00", 3, 0) },
			day:   "2026-11-08",
			want:  "Ta seria ma już Mszę w tym dniu.",
		},
		{
			name: "stopped series",
			setup: func(t *testing.T, db *gorm.DB) {
				s := seedSeries(t, db, "+ Jan", "2026-11-07 18:00", 2, 0)
				if err := gregorian.Stop(db, s.ID, seriesNow); err != nil {
					t.Fatal(err)
				}
			},
			day:  "2026-11-09",
			want: "Ta seria jest już zakończona albo przerwana.",
		},
		{
			name:  "series with 30 masses",
			setup: func(t *testing.T, db *gorm.DB) { seedSeries(t, db, "+ Jan", "2026-10-20 18:00", 30, 0) },
			day:   "2026-11-20",
			want:  "Ta seria ma już 30 Mszy.",
		},
		{
			name:  "missing series",
			setup: func(t *testing.T, db *gorm.DB) {},
			day:   "2026-11-09",
			want:  "Ta seria gregoriańska już nie istnieje.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, seriesNow)
			tt.setup(t, h.db)
			before := countIntentions(t, h.db)
			form := url.Values{"week": {"2026-11-09"}, "day": {tt.day}, "time": {"18:00"}, "content": {"x"}, "series": {"1"}}

			rec := postToRouter(t, h.db, "/intentions", form)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			checkBody(t, rec.Body.String(), `<p class="field-error" id="series-error">`+tt.want+`</p>`)
			if countIntentions(t, h.db) != before {
				t.Error("an intention was saved despite the series error")
			}
		})
	}
}

func TestContinueSeriesInvalidID(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	form := url.Values{"week": {"2026-11-09"}, "day": {"2026-11-09"}, "time": {"18:00"}, "content": {"x"}, "series": {"abc"}}

	if rec := postToRouter(t, h.db, "/intentions", form); rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestCopySeriesMassAddsNextMass(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan Kowalski", "2026-11-07 18:00", 2, 150000)

	edit := getWeek(t, h, "/?week=2026-11-02&edit=2").Body.String()
	checkBody(t, edit,
		`<input type="hidden" name="series" value="1">`,
		`Kolejna Msza tej serii? Wybierz dzień i godzinę i kliknij „Kopiuj”.`,
	)

	form := url.Values{
		"week": {"2026-11-09"}, "day": {"2026-11-09"}, "time": {"18:00"},
		"content": {"+ Jan Kowalski"}, "series": {"1"}, "amount": {""}, "copy": {"1"},
	}
	rec := postToRouter(t, h.db, "/intentions", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if copied := loadIntentionByID(t, h.db, 3); copied.SeriesID != 1 || copied.Amount != 0 {
		t.Errorf("copy = %+v, want the next mass of series 1 without an amount", copied)
	}
	if got := seriesAmounts(t, h.db, 1); !slices.Equal(got, []intentions.Grosze{150000, 0, 0}) {
		t.Errorf("amounts = %v, want the offering only on 1/30", got)
	}
}

func TestEditFirstMassRenamesSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-07 18:00", 3, 150000)
	form := url.Values{
		"week": {"2026-11-02"}, "day": {"2026-11-07"}, "time": {"18:00"},
		"content": {"+ Jan Kowalski"}, "amount": {"1200"},
	}

	rec := postToRouter(t, h.db, "/intentions/1", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	var contents []string
	err := h.db.Model(&intentions.Intention{}).Where("series_id = 1").Pluck("content", &contents).Error
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(contents, []string{"+ Jan Kowalski", "+ Jan Kowalski", "+ Jan Kowalski"}) {
		t.Errorf("contents = %q, want the whole series renamed", contents)
	}
	if got := seriesAmounts(t, h.db, 1); !slices.Equal(got, []intentions.Grosze{120000, 0, 0}) {
		t.Errorf("amounts = %v", got)
	}
}

func TestEditLaterMassKeepsSeriesContentAndOffering(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-07 18:00", 3, 150000)
	form := url.Values{
		"week": {"2026-11-02"}, "day": {"2026-11-08"}, "time": {"10:00"},
		"content": {"coś innego"}, "amount": {"300"},
	}

	rec := postToRouter(t, h.db, "/intentions/2", form)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	saved := loadIntentionByID(t, h.db, 2)
	if saved.Content != "+ Jan" || saved.MassAt.In(intentions.TimeZone).Format("15:04") != "10:00" {
		t.Errorf("saved = %+v, want the series content and the new time", saved)
	}
	if got := seriesAmounts(t, h.db, 1); !slices.Equal(got, []intentions.Grosze{150000, 0, 0}) {
		t.Errorf("amounts = %v, want the offering unchanged on 1/30", got)
	}

	form.Set("day", "2026-11-09")
	if rec := postToRouter(t, h.db, "/intentions/2", form); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("moving 2/30 onto the day of 3/30: status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
}

func TestDeleteFirstMassMovesOffering(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-07 18:00", 2, 150000)

	if rec := postToRouter(t, h.db, "/intentions/1/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := seriesAmounts(t, h.db, 1); !slices.Equal(got, []intentions.Grosze{150000}) {
		t.Errorf("amounts = %v, want the offering moved to the remaining mass", got)
	}

	if rec := postToRouter(t, h.db, "/intentions/2/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if _, err := gregorian.Find(h.db, 1); err == nil {
		t.Error("series still exists after deleting all its masses")
	}
}

func TestClearWeekKeepsSeriesOffering(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-07 18:00", 4, 150000)
	seedSeries(t, h.db, "+ Anna", "2026-11-02 06:30", 2, 50000)

	rec := postToRouter(t, h.db, "/clear", url.Values{"week": {"2026-11-02"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := seriesAmounts(t, h.db, 1); !slices.Equal(got, []intentions.Grosze{150000, 0}) {
		t.Errorf("amounts = %v, want the offering moved to 9.11", got)
	}
	if _, err := gregorian.Find(h.db, 2); err == nil {
		t.Error("series with every mass in the cleared week still exists")
	}
}

func TestPanelShowsSeriesPicker(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan Kowalski", "2026-11-01 18:00", 9, 150000)
	seedSeries(t, h.db, "+ Anna Nowak", "2026-11-03 06:30", 8, 0)

	body := getWeek(t, h, "/?week=2026-11-09&new=1&day=2026-11-10").Body.String()

	checkBody(t, body,
		`<legend class="label">Msze gregoriańskie w trakcie</legend>`,
		`<input type="radio" id="series-1" name="series" value="1" data-content="&#43; Jan Kowalski" data-time="18:00" data-outside="" data-paid="1`+nbsp+`500`+nbsp+`zł" data-days="2026-11-01,2026-11-02,2026-11-03,2026-11-04,2026-11-05,2026-11-06,2026-11-07,2026-11-08,2026-11-09" data-next="2026-11-10" data-first="2026-11-01">`,
		`<span class="series-step-big">10/30</span>`,
		`<span class="series-meta is-due">na ten dzień · 18:00</span>`,
		`<input type="radio" id="series-2" name="series" value="2" disabled`,
		`<span class="series-meta">już dodana na ten dzień</span>`,
		`<input type="radio" id="series-none" name="series" value="" checked>`,
		`<span class="panel-step" id="series-step" hidden></span>`,
	)

	late := getWeek(t, h, "/?week=2026-11-09&new=1&day=2026-11-12").Body.String()
	checkBody(t, late, `<span class="series-meta is-late">zaległa od 10.11</span>`)
}

func TestPanelWithSelectedSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan Kowalski", "2026-11-01 18:00", 9, 150000)

	body := getWeek(t, h, "/?week=2026-11-09&new=1&day=2026-11-10&series=1").Body.String()

	checkBody(t, body,
		`<span class="panel-step" id="series-step">gregoriańska 10/30</span>`,
		`<input type="radio" id="series-1" name="series" value="1" checked data-content=`,
		`<input type="radio" id="series-none" name="series" value="">`,
		`required autofocus readonly>&#43; Jan Kowalski</textarea>`,
		`<p class="hint" id="content-series-hint">Treść pochodzi z serii.`,
		`<option value="18:00" selected>18:00</option>`,
		`<input type="checkbox" name="gregorian" checked disabled> Msza gregoriańska`,
		`<p class="series-paid" id="series-paid">Ofiara za całą serię: <strong>1`+nbsp+`500`+nbsp+`zł</strong> – wpisana przy 1/30.</p>`,
		`<div class="payment-row" id="amount-row" hidden>`,
		`<p class="series-hint" id="series-new-hint" hidden>`,
		`<p class="hint" id="amount-hint" hidden>`,
	)
}

func TestPanelForNewGregorianSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	form := url.Values{"week": {"2026-11-09"}, "day": {"2026-11-10"}, "time": {""}, "content": {"+ Maria"}, "gregorian": {"on"}}

	body := postToRouter(t, h.db, "/intentions", form).Body.String()

	checkBody(t, body,
		`<span class="panel-step" id="series-step">gregoriańska 1/30</span>`,
		`<p class="series-hint" id="series-new-hint">`,
		`<label class="label" for="amount" data-series-label="Ofiara za całą serię (30 Mszy)">Ofiara za całą serię (30 Mszy)</label>`,
		`<p class="hint" id="amount-once">Płatne <strong>jednorazowo</strong>`,
	)
}

func TestPanelEditOfSeriesMasses(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-07 18:00", 3, 150000)

	first := getWeek(t, h, "/?week=2026-11-02&edit=1").Body.String()
	checkBody(t, first,
		`<span class="panel-step" id="series-step">gregoriańska 1/30</span>`,
		`<div class="payment-row" id="amount-row">`,
		`name="amount" value="1500"`,
		`>Ofiara za całą serię (30 Mszy)</label>`,
	)
	if strings.Contains(first, "readonly") {
		t.Error("the first mass of a series has read-only content, want it editable")
	}

	later := getWeek(t, h, "/?week=2026-11-09&edit=3").Body.String()
	checkBody(t, later,
		`<span class="panel-step" id="series-step">gregoriańska 3/30</span>`,
		`readonly>&#43; Jan</textarea>`,
		`<div class="payment-row" id="amount-row" hidden>`,
	)
	if strings.Contains(later, "Msze gregoriańskie w trakcie") {
		t.Error("editing a series mass shows the series picker")
	}
}

func TestWeekShowsSeriesGapNotice(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Henryk Zieliński", "2026-10-19 18:00", 20, 150000, "2026-11-05")

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()

	checkBody(t, body,
		`<p class="week-notice is-series" role="status"><span>Seria gregoriańska <strong>&#43; Henryk Zieliński</strong> ma przerwę 5.11 (po 17/30) – zakończy się ok. 18.11.</span> <a href="/?week=2026-11-09&amp;gregorian=active&amp;series=1">Pokaż serię</a></p>`,
		`&#43; Henryk Zieliński <span class="series-due-step">21/30</span></a>`,
	)
	if got := getWeek(t, h, "/?week=2026-12-07").Body.String(); strings.Contains(got, "week-notice is-series") {
		t.Error("gap notice shown on a week after the series ends")
	}
}

func TestWeekWarnsWhenGregorianSharesMass(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-09 18:00", 1, 0)
	seedIntention(t, h.db, "2026-11-09 18:00", false, "Za parafian")
	seedIntention(t, h.db, "2026-11-10 18:00", false, "+ Anna")

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()
	if got := strings.Count(body, `<p class="mass-warning">Gregorianka powinna być jedyną intencją tej Mszy.</p>`); got != 1 {
		t.Errorf("mass warnings = %d, want 1", got)
	}

	shared := getWeek(t, h, "/?week=2026-11-09&new=1&day=2026-11-10&time=18:00").Body.String()
	checkBody(t, shared, `<p class="mass-warning" id="shared-hint" hidden>`)

	gregorianForm := postToRouter(t, h.db, "/intentions", url.Values{"week": {"2026-11-09"}, "day": {"2026-11-10"}, "time": {"18:00"}, "content": {""}, "gregorian": {"on"}}).Body.String()
	checkBody(t, gregorianForm, `<p class="mass-warning" id="shared-hint">Na tej Mszy będzie gregorianka i inna intencja`)
}

func TestGregorianDialog(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan Kowalski", "2026-10-27 18:00", 17, 150000)
	seedSeries(t, h.db, "+ Henryk", "2026-10-19 18:00", 20, 0, "2026-11-05")
	seedSeries(t, h.db, "+ Maria", "2026-10-02 18:00", 30, 150000)

	rec := getWeek(t, h, "/?week=2026-11-09&gregorian=active&series=1")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	checkBody(t, body,
		`<main inert>`,
		`<a class="button toolbar-gregorian is-open" href="/?week=2026-11-09&amp;gregorian=active" title="Msze gregoriańskie – w trakcie: 2">`,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;gregorian=active" aria-current="page" autofocus>W trakcie · 2</a>`,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;gregorian=done">Zakończone · 1</a>`,
		`<li class="series-row is-open" id="s-1">`,
		`<span class="series-sub">od 27.10.2026 · zwykle 18:00 · ofiara 1`+nbsp+`500`+nbsp+`zł</span>`,
		`<span class="series-count"><strong>7</strong>/30<span>odprawionych</span></span>`,
		`<span class="series-bar" role="img" aria-label="7 odprawionych, 10 zaplanowanych, 13 do dodania">`,
		`<span class="series-next">17 zapisanych · koniec ok. 25.11 · następna: <strong>Pt 13.11 (18/30)</strong></span>`,
		`<a class="button-small" href="/?day=2026-11-13&amp;new=1&amp;time=18%3A00&amp;week=2026-11-09&amp;series=1">Dodaj 18/30</a>`,
		`<a class="series-day done" href="/?week=2026-10-26#i-1" title="1/30 · 27.10 18:00 · odprawiona"><b>1</b><span>27.10</span></a>`,
		`<span class="series-day todo" title="18/30 · 13.11 · do dodania (przewidywana data)"><b>18</b><span>13.11</span></span>`,
		`<a class="series-edit" href="/?edit=1&amp;week=2026-10-26">Zmień treść lub ofiarę</a>`,
		`<a class="delete-link" href="/?week=2026-11-09&amp;gregorian=active&amp;series=1&amp;stop=1">Przerwij serię</a>`,
		`<span class="series-gap">`,
		`Przerwa 5.11 (po 17/30) – seria się wydłuża</span>`,
		`<i class="seg planned gap-before"></i>`,
	)
	if got := strings.Count(body, `<i class="seg `); got != 60 {
		t.Errorf("segments = %d, want 60 (30 per series)", got)
	}

	closed := getWeek(t, h, "/?week=2026-11-09&gregorian=done").Body.String()
	checkBody(t, closed,
		`<span class="series-title">&#43; Maria<span class="series-status">zakończona 31.10</span></span>`,
		`Odprawiono wszystkie 30 Mszy.`,
	)

	jump := getWeek(t, h, "/?week=2026-11-09&gregorian=active&series=3").Body.String()
	checkBody(t, jump, `<li class="series-row is-open" id="s-3">`, `aria-current="page" autofocus>Zakończone · 1</a>`)

	stop := getWeek(t, h, "/?week=2026-11-09&gregorian=active&series=1&stop=1").Body.String()
	checkBody(t, stop,
		`<h2 class="dialog-title" id="dialog-title">Przerwać serię gregoriańską?</h2>`,
		`<form class="dialog-actions" method="post" action="/series/1/stop">`,
		`<input type="hidden" name="week" value="2026-11-09">`,
	)
}

func TestGregorianDialogEmptyAndInvalid(t *testing.T) {
	h := newTestHandler(t, seriesNow)

	checkBody(t, getWeek(t, h, "/?week=2026-11-09&gregorian=active").Body.String(), `<p class="series-empty">Nie ma serii w trakcie.`)

	for _, target := range []string{
		"/?week=2026-11-09&gregorian=all",
		"/?week=2026-11-09&gregorian=active&series=abc",
		"/?week=2026-11-09&gregorian=active&series=0",
		"/?week=2026-11-09&gregorian=active&stop=1",
	} {
		if rec := getWeek(t, h, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestStopSeries(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-01 18:00", 5, 0)

	rec := postToRouter(t, h.db, "/series/1/stop", url.Values{"week": {"2026-11-09"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got, want := rec.Header().Get("Location"), "/?week=2026-11-09&gregorian=done&series=1"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}
	series, err := gregorian.Find(h.db, 1)
	if err != nil || series.StoppedAt == nil {
		t.Errorf("series = %+v, %v, want it stopped", series, err)
	}
	checkBody(t, getWeek(t, h, "/?week=2026-11-09&gregorian=done").Body.String(), `<span class="series-status is-stopped">przerwana po 5/30</span>`)

	if rec := postToRouter(t, h.db, "/series/9/stop", url.Values{"week": {"2026-11-09"}}); rec.Code != http.StatusNotFound {
		t.Errorf("missing series: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if rec := postToRouter(t, h.db, "/series/x/stop", url.Values{"week": {"2026-11-09"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid id: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
	if rec := postToRouter(t, h.db, "/series/1/stop", url.Values{"week": {"bzdura"}}); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid week: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestPDFPrintsSeriesStep(t *testing.T) {
	tests := []struct {
		intention intentionView
		want      string
	}{
		{intention: intentionView{Content: "+ Jan", IsGregorian: true, IsDeceased: true, SeriesStep: 14}, want: "+ Jan (greg. 14/30)"},
		{intention: intentionView{Content: "+ Jan", IsGregorian: true, IsDeceased: true}, want: "+ Jan (greg.)"},
		{intention: intentionView{Content: "+ Jan (greg.)", IsGregorian: true, SeriesStep: 2}, want: "+ Jan (greg.)"},
	}

	for _, tt := range tests {
		if got := printedContent(tt.intention); got != tt.want {
			t.Errorf("printedContent(%+v) = %q, want %q", tt.intention, got, tt.want)
		}
	}

	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-01 18:00", 3, 0)
	if rec := getPDF(t, h, "/pdf?week=2026-10-26"); rec.Code != http.StatusOK {
		t.Errorf("PDF status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestOutsideParishDoesNotShareTheParishMass(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	outside, err := gregorian.Create(h.db, "+ Henryk", true)
	if err != nil {
		t.Fatal(err)
	}
	gregorianOutside := newIntention(t, 0, "2026-11-09 18:00", true, "+ Henryk")
	gregorianOutside.IsGregorian = true
	gregorianOutside.SeriesID = outside.ID
	if err := h.db.Create(&gregorianOutside).Error; err != nil {
		t.Fatal(err)
	}
	seedIntention(t, h.db, "2026-11-09 18:00", false, "Za parafian")
	seedSeries(t, h.db, "+ Jan", "2026-11-10 18:00", 1, 0)
	seedIntention(t, h.db, "2026-11-10 18:00", true, "+ Anna w innej parafii")

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()
	if strings.Contains(body, `<p class="mass-warning">`) {
		t.Error("a gregorian mass shares the time only with an intention poza parafią, want no warning")
	}
	checkBody(t, body,
		`<div class="mass" data-time="18:00" data-count="1" data-gregorian="0">`,
		`<div class="mass" data-time="18:00" data-count="1" data-gregorian="1">`,
	)

	form := url.Values{"week": {"2026-11-09"}, "day": {"2026-11-09"}, "time": {"18:00"}, "content": {""}, "gregorian": {"on"}}
	checkBody(t, postToRouter(t, h.db, "/intentions", form).Body.String(), `<p class="mass-warning" id="shared-hint">`)

	form.Set("outside", "on")
	checkBody(t, postToRouter(t, h.db, "/intentions", form).Body.String(), `<p class="mass-warning" id="shared-hint" hidden>`)
}

func TestPaymentsDialogWithGregorianOffering(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedSeries(t, h.db, "+ Jan", "2026-11-09 18:00", 3, 150000)
	seedPaid(t, h.db, "2026-11-10 06:30", false, "+ Anna", 10000)
	seedPaid(t, h.db, "2026-11-11 06:30", false, "+ Józef", 15000)

	body := getWeek(t, h, "/?week=2026-11-09&payments=week").Body.String()
	checkBody(t, body,
		`<span class="stat-value">1`+nbsp+`750`+nbsp+`zł</span>`,
		`<span class="stat-value">125`+nbsp+`zł</span>`,
		`<span class="stat-note">bez gregorianek</span>`,
		`<span class="stat-label">Gregorianki</span>`,
		`</svg>1`+nbsp+`500`+nbsp+`zł</span>`,
		`<span class="stat-note">1 seria</span>`,
		`<form class="payments-options" method="get" action="/">`,
		`<input type="hidden" name="payments" value="week">`,
		`<input type="checkbox" name="greg" value="1" data-autosubmit> Wliczaj gregorianki do średniej</label>`,
		`<span class="payments-options-note">(z gregoriankami: 583`+nbsp+`zł)</span>`,
		`<span class="greg-mark" title="w tym gregorianki: 1`+nbsp+`500`+nbsp+`zł">`,
	)
	if got := strings.Count(body, `class="greg-mark"`); got != 1 {
		t.Errorf("rows with a candle = %d, want 1 (only Monday has the offering)", got)
	}

	with := getWeek(t, h, "/?week=2026-11-09&payments=month&month=2026-11&greg=1").Body.String()
	checkBody(t, with,
		`<span class="stat-value">583`+nbsp+`zł</span>`,
		`<span class="stat-note">z gregoriankami</span>`,
		`<input type="hidden" name="month" value="2026-11">`,
		`<input type="checkbox" name="greg" value="1" checked data-autosubmit>`,
		`<span class="payments-options-note">(bez gregorianek: 125`+nbsp+`zł)</span>`,
		`href="/?week=2026-11-09&amp;payments=year&amp;greg=1">Rok</a>`,
	)
}

func TestPaymentsDialogWithoutGregorianHasNoCheckbox(t *testing.T) {
	h := newTestHandler(t, seriesNow)
	seedPaid(t, h.db, "2026-11-10 06:30", false, "+ Anna", 10000)

	body := getWeek(t, h, "/?week=2026-11-09&payments=week").Body.String()

	checkBody(t, body, `<span class="stat-label">Gregorianki</span>
					<span class="stat-value">—</span>`)
	for _, unwanted := range []string{"payments-options", "greg-mark", "bez gregorianek"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body contains %q without any gregorian offering", unwanted)
		}
	}
}
