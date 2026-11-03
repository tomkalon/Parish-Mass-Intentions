package web

import (
	"bytes"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/intentions"
	"go-parafia/internal/settings"
)

const nbsp = " "

func seedPaid(t *testing.T, db *gorm.DB, massAt string, outside bool, content string, amount intentions.Grosze) intentions.Intention {
	t.Helper()

	intention := newIntention(t, 0, massAt, outside, content)
	intention.Amount = amount
	err := db.Create(&intention).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}

	return intention
}

func loadAmount(t *testing.T, db *gorm.DB, id int) intentions.Grosze {
	t.Helper()

	var intention intentions.Intention
	err := db.First(&intention, id).Error
	if err != nil {
		t.Fatalf("loading intention %d: %v", id, err)
	}

	return intention.Amount
}

func checkBody(t *testing.T, body string, wants ...string) {
	t.Helper()

	for _, want := range wants {
		if !strings.Contains(body, want) {
			t.Errorf("body does not contain %q", want)
		}
	}
}

func seedPaymentsWeek(t *testing.T, db *gorm.DB) {
	t.Helper()

	seedPaid(t, db, "2026-11-09 18:00", false, "+ Jan Kowalski", 15000)
	seedPaid(t, db, "2026-11-09 18:00", false, "Za parafian", 0)
	seedPaid(t, db, "2026-11-10 06:30", true, "+ Anna Nowak", 10050)
	seedPlaceholder(t, db, "2026-11-11 06:30")
}

func TestCreateIntentionWithAmount(t *testing.T) {
	tests := []struct {
		amount string
		want   intentions.Grosze
	}{
		{amount: "150,50", want: 15050},
		{amount: " 1 500 zł ", want: 150000},
		{amount: "", want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.amount, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			form := validForm()
			form.Set("amount", tt.amount)

			rec := postIntention(t, h, form)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := loadAmount(t, h.db, 1); got != tt.want {
				t.Errorf("saved amount = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCreateIntentionInvalidAmount(t *testing.T) {
	tests := []struct {
		amount string
		want   string
	}{
		{amount: "15O", want: intentions.InvalidAmountMessage},
		{amount: "-50", want: intentions.InvalidAmountMessage},
		{amount: "150,505", want: intentions.InvalidAmountMessage},
		{amount: "100000", want: intentions.AmountTooLargeMessage},
	}

	for _, tt := range tests {
		t.Run(tt.amount, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			form := validForm()
			form.Set("amount", tt.amount)

			rec := postIntention(t, h, form)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			checkBody(t, rec.Body.String(),
				`<p class="field-error" id="amount-error">`+tt.want+`</p>`,
				`name="amount" value="`+tt.amount+`"`,
				`aria-describedby="amount-error amount-hint" aria-invalid="true"`,
				`Nie zapisano. Popraw zaznaczone pola`,
				`<textarea class="textarea" id="content" name="content" maxlength="500" required autofocus>  &#43; Jan Kowalski  </textarea>`,
			)
			if n := countIntentions(t, h.db); n != 0 {
				t.Errorf("intentions = %d, want 0 (nothing saved with an invalid amount)", n)
			}
		})
	}
}

func TestPanelShowsPaymentField(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	body := getWeek(t, h, "/?week=2026-11-02&new=1").Body.String()

	checkBody(t, body,
		`<label class="label" for="amount" data-series-label="Ofiara za całą serię (30 Mszy)">Wpłata</label>`,
		`nie trafia na wydruk</span>`,
		`<input class="input" type="text" id="amount" name="amount" value="" inputmode="decimal" autocomplete="off" placeholder="0" maxlength="20" aria-describedby="amount-hint">`,
		`<span class="amount-unit" aria-hidden="true">zł</span>`,
		`<div class="amount-chips" role="group" aria-label="Częste kwoty">`,
		`<button class="chip" type="button" data-amount="50" aria-label="50`+nbsp+`zł" aria-pressed="false">50</button>`,
		`<button class="chip" type="button" data-amount="200" aria-label="200`+nbsp+`zł" aria-pressed="false">200</button>`,
		`<p class="hint" id="amount-hint">Puste pole – intencja bez wpłaty.</p>`,
	)
	if got := strings.Count(body, `class="chip"`); got != len(settings.DefaultQuickAmounts) {
		t.Errorf("chips = %d, want %d", got, len(settings.DefaultQuickAmounts))
	}
}

func TestEditPanelShowsAmount(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPaid(t, h.db, "2026-11-03 18:00", false, "+ Jan Kowalski", 10000)
	seedPaid(t, h.db, "2026-11-03 18:00", false, "+ Anna Nowak", 15050)

	checkBody(t, getWeek(t, h, "/?week=2026-11-02&edit=1").Body.String(),
		`name="amount" value="100"`,
		`data-amount="100" aria-label="100`+nbsp+`zł" aria-pressed="true">100</button>`,
		`data-amount="150" aria-label="150`+nbsp+`zł" aria-pressed="false">150</button>`,
	)
	checkBody(t, getWeek(t, h, "/?week=2026-11-02&edit=2").Body.String(),
		`name="amount" value="150,50"`,
	)
}

func TestPanelUsesQuickAmountsFromSettings(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	err := settings.SaveParish(h.db, settings.Setting{ParishName: "Parafia", QuickAmounts: []intentions.Grosze{7000, 30000}})
	if err != nil {
		t.Fatalf("saving settings: %v", err)
	}

	body := getWeek(t, h, "/?week=2026-11-02&new=1").Body.String()

	checkBody(t, body, `data-amount="70"`, `data-amount="300"`)
	if got := strings.Count(body, `class="chip"`); got != 2 {
		t.Errorf("chips = %d, want 2", got)
	}
}

func TestUpdateIntentionAmount(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPaid(t, h.db, "2026-11-03 18:00", false, "+ Jan Kowalski", 10000)
	form := validForm()

	form.Set("amount", "200")
	if rec := postToRouter(t, h.db, "/intentions/1", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := loadAmount(t, h.db, 1); got != 20000 {
		t.Errorf("amount = %d, want 20000", got)
	}

	form.Set("amount", "")
	if rec := postToRouter(t, h.db, "/intentions/1", form); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := loadAmount(t, h.db, 1); got != 0 {
		t.Errorf("amount = %d after clearing the field, want 0", got)
	}
}

func TestUpdateIntentionInvalidAmountKeepsSavedOne(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
	seedPaid(t, h.db, "2026-11-03 18:00", false, "+ Jan Kowalski", 10000)
	form := validForm()
	form.Set("amount", "sto")

	rec := postToRouter(t, h.db, "/intentions/1", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	checkBody(t, rec.Body.String(), `<form class="panel-form" method="post" action="/intentions/1">`, `name="amount" value="sto"`)
	if got := loadAmount(t, h.db, 1); got != 10000 {
		t.Errorf("amount = %d, want the saved 10000 unchanged", got)
	}
}

func TestCopyIntentionAmount(t *testing.T) {
	tests := []struct {
		name      string
		gregorian bool
		want      intentions.Grosze
	}{
		{name: "regular intention keeps the amount", want: 15000},
		{name: "gregorian mass is paid once, the copy has no amount", gregorian: true, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))
			seedPaid(t, h.db, "2026-11-03 18:00", false, "+ Anna Kutyba", 15000)
			form := url.Values{
				"week":    {"2026-11-02"},
				"day":     {"2026-11-04"},
				"time":    {"18:00"},
				"content": {"+ Anna Kutyba"},
				"amount":  {"150"},
				"copy":    {"1"},
			}
			if tt.gregorian {
				form.Set("gregorian", "on")
			}

			rec := postToRouter(t, h.db, "/intentions", form)

			if rec.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
			}
			if got := loadAmount(t, h.db, 2); got != tt.want {
				t.Errorf("copy amount = %d, want %d", got, tt.want)
			}
			if got := loadAmount(t, h.db, 1); got != 15000 {
				t.Errorf("original amount = %d, want 15000 unchanged", got)
			}
		})
	}
}

func TestWeekShowsAmounts(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))
	seedPaymentsWeek(t, h.db)

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()

	checkBody(t, body,
		`<a class="button toolbar-payments" href="/?week=2026-11-09&amp;payments=week" title="Wpłaty">`,
		`<a class="week-payments" href="/?week=2026-11-09&amp;payments=week" title="Podsumowanie wpłat · kwota wpisana przy 2 z 3 intencji">`,
		`Wpłaty <strong>250,50`+nbsp+`zł</strong>`,
		`<a class="week-unpaid" href="/?week=2026-11-09&amp;unpaid=1#i-2" title="Zaznacz w tygodniu intencje bez wpisanej kwoty">1 bez kwoty</a>`,
		`<span>&#43; Jan Kowalski</span>
								<span class="amount">150`+nbsp+`zł</span>`,
		`<span>Za parafian</span>
								<span class="amount is-missing" title="Brak wpisanej kwoty"><span aria-hidden="true">—</span><span class="visually-hidden">bez kwoty</span></span>`,
		`<span>&#43; Anna Nowak</span>
								<span class="amount">100,50`+nbsp+`zł</span>`,
		`<span class="placeholder">do uzupełnienia</span>
								<span class="amount"></span>`,
	)
	if strings.Contains(body, "is-unpaid") {
		t.Error("intentions without an amount are highlighted without ?unpaid=1")
	}
	if strings.Contains(body, "dialog-payments") || strings.Contains(body, "<main inert>") {
		t.Error("week page shows the payments dialog without ?payments=")
	}
}

func TestWeekWithoutUnpaidHasNoUnpaidPill(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))
	seedPaid(t, h.db, "2026-11-09 18:00", false, "+ Jan Kowalski", 15000)
	seedPlaceholder(t, h.db, "2026-11-10 06:30")

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()

	checkBody(t, body, `Wpłaty <strong>150`+nbsp+`zł</strong>`)
	if strings.Contains(body, "week-unpaid") {
		t.Error("week where every intention has an amount shows the „bez kwoty” pill")
	}
}

func TestEmptyWeekHasNoPaymentsPill(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))
	seedPlaceholder(t, h.db, "2026-11-10 06:30")

	body := getWeek(t, h, "/?week=2026-11-09").Body.String()

	if strings.Contains(body, `class="week-payments"`) {
		t.Error("week without intentions shows the payments pill")
	}
	checkBody(t, body, `<a class="button toolbar-payments" href="/?week=2026-11-09&amp;payments=week" title="Wpłaty">`)
}

func TestWeekHighlightsUnpaid(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))
	seedPaymentsWeek(t, h.db)

	rec := getWeek(t, h, "/?week=2026-11-09&unpaid=1")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	checkBody(t, body,
		`<li class="intention is-unpaid" id="i-2">`,
		`<span aria-hidden="true">brak</span>`,
		`<a class="week-unpaid is-active" href="/?week=2026-11-09" title="Wyłącz zaznaczenie intencji bez kwoty">1 bez kwoty</a>`,
	)
	if got := strings.Count(body, "is-unpaid"); got != 1 {
		t.Errorf("highlighted intentions = %d, want 1 (paid ones and placeholders stay plain)", got)
	}
}

func TestWeekWithPaymentsDialogForWeek(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 10, 10, 0, 0, 0, intentions.TimeZone))
	seedPaymentsWeek(t, h.db)
	seedPaid(t, h.db, "2026-11-16 07:00", false, "next week", 99900)

	rec := getWeek(t, h, "/?week=2026-11-09&payments=week")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	checkBody(t, rec.Body.String(),
		`<main inert>`,
		`<a class="button toolbar-payments is-open" href="/?week=2026-11-09&amp;payments=week" title="Wpłaty">`,
		`<div class="dialog dialog-payments" role="dialog" aria-modal="true" aria-labelledby="payments-title">`,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;payments=week" aria-current="page" autofocus>Tydzień</a>`,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;payments=month">Miesiąc</a>`,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;payments=year">Rok</a>`,
		`<a class="panel-close" href="/?week=2026-11-09" aria-label="Zamknij podsumowanie wpłat" data-close>×</a>`,
		`<a class="button button-icon" href="/?week=2026-11-02&amp;payments=week" aria-label="Poprzedni tydzień" title="Poprzedni tydzień">`,
		`<a class="button button-icon" href="/?week=2026-11-16&amp;payments=week" aria-label="Następny tydzień" title="Następny tydzień">`,
		`<h3 class="payments-period-name">9 – 15 listopada 2026</h3>`,
		`<a class="payments-period-link" href="/?week=2026-11-09">Otwórz ten tydzień →</a>`,
		`<span class="stat-value">250,50`+nbsp+`zł</span>`,
		`<span class="stat-value">3</span>`,
		`<span class="stat-value is-alert">1</span>`,
		`<a class="stat-link" href="/?week=2026-11-09&amp;unpaid=1#i-2" title="Zaznacz w tygodniu intencje bez wpisanej kwoty">Pokaż</a>`,
		`<span class="stat-value">125`+nbsp+`zł</span>`,
		`<th scope="col">Dzień</th>`,
		`<th scope="row">Poniedziałek 9.11</th>
						<td class="bar-cell"><span class="bar" style="width: 100.0%" title="Poniedziałek 9.11: 150`+nbsp+`zł, 2 intencje"></span></td>
						<td class="num">150`+nbsp+`zł</td>
						<td class="num">2</td>
						<td class="num missing">1</td>`,
		`<tr class="is-current">
						<th scope="row">Wtorek 10.11</th>
						<td class="bar-cell"><span class="bar" style="width: 67.0%" title="Wtorek 10.11: 100,50`+nbsp+`zł, 1 intencja"></span></td>`,
		`<th scope="row">Środa 11.11</th>
						<td class="bar-cell"></td>
						<td class="num zero">—</td>
						<td class="num">0</td>
						<td class="num"></td>`,
		`Kwoty liczone według dnia mszy. Wpłaty nie trafiają na wydruk.`,
	)
	if strings.Contains(rec.Body.String(), "999") {
		t.Error("week summary includes an intention from the next week")
	}
}

func TestWeekWithPaymentsDialogForMonth(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 10, 10, 0, 0, 0, intentions.TimeZone))
	seedPaymentsWeek(t, h.db)
	seedPaid(t, h.db, "2026-11-01 10:00", false, "Wszystkich Świętych", 20000)
	seedPaid(t, h.db, "2026-10-31 18:00", false, "October", 99900)

	body := getWeek(t, h, "/?week=2026-11-09&payments=month").Body.String()

	checkBody(t, body,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;payments=month" aria-current="page" autofocus>Miesiąc</a>`,
		`<h3 class="payments-period-name">Listopad 2026</h3>`,
		`href="/?week=2026-11-09&amp;payments=month&amp;month=2026-10" aria-label="Poprzedni miesiąc"`,
		`href="/?week=2026-11-09&amp;payments=month&amp;month=2026-12" aria-label="Następny miesiąc"`,
		`<th scope="col">Tydzień</th>`,
		`<th scope="row"><a class="row-link" href="/?week=2026-10-26&amp;payments=week">1.11</a></th>`,
		`<tr class="is-current">
						<th scope="row"><a class="row-link" href="/?week=2026-11-09&amp;payments=week">9 – 15.11</a></th>`,
		`<th scope="row"><a class="row-link" href="/?week=2026-11-30&amp;payments=week">30.11</a></th>`,
		`<span class="stat-value">450,50`+nbsp+`zł</span>`,
	)
	if strings.Contains(body, "Otwórz ten tydzień") || strings.Contains(body, `class="stat-link"`) {
		t.Error("month summary links to a single week")
	}
	if strings.Contains(body, "999") {
		t.Error("November summary includes an intention from October")
	}
}

func TestWeekWithPaymentsDialogForYear(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 10, 10, 0, 0, 0, intentions.TimeZone))
	seedPaymentsWeek(t, h.db)
	seedPaid(t, h.db, "2026-03-15 10:00", false, "+ Józef", 20000)
	seedPaid(t, h.db, "2025-12-31 18:00", false, "last year", 99900)

	body := getWeek(t, h, "/?week=2026-11-09&payments=year").Body.String()

	checkBody(t, body,
		`<a class="segmented-item" href="/?week=2026-11-09&amp;payments=year" aria-current="page" autofocus>Rok</a>`,
		`<h3 class="payments-period-name">2026</h3>`,
		`href="/?week=2026-11-09&amp;payments=year&amp;year=2025" aria-label="Poprzedni rok"`,
		`<th scope="col">Miesiąc</th>`,
		`<th scope="row"><a class="row-link" href="/?week=2026-11-09&amp;payments=month&amp;month=2026-03">Marzec</a></th>`,
		`<tr class="is-current">
						<th scope="row"><a class="row-link" href="/?week=2026-11-09&amp;payments=month&amp;month=2026-11">Listopad</a></th>`,
		`<span class="stat-value">450,50`+nbsp+`zł</span>`,
	)
	if got := strings.Count(body, `class="row-link"`); got != 12 {
		t.Errorf("month rows = %d, want 12", got)
	}
}

func TestWeekPaymentsDialogRejectsInvalidPeriod(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))

	for _, target := range []string{
		"/?week=2026-11-09&payments=decade",
		"/?week=2026-11-09&payments=month&month=2026-13",
		"/?week=2026-11-09&payments=month&month=listopad",
		"/?week=2026-11-09&payments=year&year=abc",
		"/?week=2026-11-09&payments=year&year=999",
	} {
		if rec := getWeek(t, h, target); rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want %d", target, rec.Code, http.StatusBadRequest)
		}
	}
}

func TestWeekPaymentsDialogDatabaseError(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 11, 2, 10, 0, 0, 0, intentions.TimeZone))
	err := h.db.Migrator().DropColumn(&intentions.Intention{}, "amount")
	if err != nil {
		t.Fatalf("dropping amount column: %v", err)
	}

	rec := getWeek(t, h, "/?week=2026-11-09&payments=year")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestPaymentsAreNotPrinted(t *testing.T) {
	now := time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)

	plain := newTestHandler(t, now)
	seedIntention(t, plain.db, "2026-11-02 18:00", false, "W int. Parafian")

	paid := newTestHandler(t, now)
	seedPaid(t, paid.db, "2026-11-02 18:00", false, "W int. Parafian", 15050)

	want := getPDF(t, plain, "/pdf?week=2026-11-02").Body.Bytes()
	if got := getPDF(t, paid, "/pdf?week=2026-11-02").Body.Bytes(); !bytes.Equal(got, want) {
		t.Error("PDF of an intention with an amount differs from the same intention without one, want amounts never printed")
	}
	if body := getPrint(t, paid, "/print?week=2026-11-02").Body.String(); strings.Contains(body, "zł") {
		t.Error("print preview mentions an amount")
	}
}

func TestSettingsPageShowsQuickAmounts(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	checkBody(t, getSettings(t, h, "/settings?week=2026-11-02").Body.String(),
		`<legend>Wpłaty</legend>`,
		`<input class="input" type="text" id="quick_amounts" name="quick_amounts" value="50, 100, 150, 200" autocomplete="off" placeholder="50, 100, 150, 200" maxlength="200" aria-describedby="quick_amounts-hint">`,
		`Pełne złote, oddzielone przecinkami, najwyżej 6. Puste pole przywraca domyślne: 50, 100, 150, 200.`,
		`<span class="quick-preview-label">W panelu:</span>
						<span class="chip chip-preview">50</span>
						<span class="chip chip-preview">100</span>`,
	)
}

func TestSaveSettingsQuickAmounts(t *testing.T) {
	h := newTestHandler(t, time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone))

	rec := postSettings(t, h, url.Values{
		"week":          {"2026-11-02"},
		"parish_name":   {"Parafia św. Jana"},
		"quick_amounts": {"70, 100, 300 zł, 100"},
	})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	got, err := settings.Load(h.db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if want := []intentions.Grosze{7000, 10000, 30000}; !slices.Equal(got.QuickAmounts, want) {
		t.Errorf("QuickAmounts = %v, want %v", got.QuickAmounts, want)
	}
	checkBody(t, getSettings(t, h, "/settings?week=2026-11-02").Body.String(),
		`name="quick_amounts" value="70, 100, 300"`,
		`<span class="chip chip-preview">300</span>`,
	)

	rec = postSettings(t, h, url.Values{"parish_name": {"Parafia św. Jana"}, "quick_amounts": {"  "}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	got, err = settings.Load(h.db)
	if err != nil {
		t.Fatalf("loading settings: %v", err)
	}
	if !slices.Equal(got.QuickAmounts, settings.DefaultQuickAmounts) {
		t.Errorf("QuickAmounts = %v after saving an empty field, want the defaults", got.QuickAmounts)
	}
}

func TestSaveSettingsInvalidQuickAmounts(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{text: "50, sto", want: quickAmountsMessage},
		{text: "50.50", want: quickAmountsMessage},
		{text: "10, 20, 30, 40, 50, 60, 70", want: "Podaj najwyżej 6 kwot."},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			h := newTestHandler(t, time.Now())

			rec := postSettings(t, h, url.Values{"parish_name": {"Parafia św. Jana"}, "quick_amounts": {tt.text}})

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			body := rec.Body.String()
			checkBody(t, body,
				`<p class="field-error" id="quick_amounts-error">`+tt.want+`</p>`,
				`name="quick_amounts" value="`+tt.text+`"`,
				`aria-describedby="quick_amounts-error quick_amounts-hint" aria-invalid="true"`,
			)
			if strings.Contains(body, "quick-preview") {
				t.Error("invalid quick amounts still show a preview")
			}

			got, err := settings.Load(h.db)
			if err != nil {
				t.Fatalf("loading settings: %v", err)
			}
			if got.ParishName != settings.DefaultParishName || !slices.Equal(got.QuickAmounts, settings.DefaultQuickAmounts) {
				t.Errorf("settings changed despite errors: %+v", got)
			}
		})
	}
}
