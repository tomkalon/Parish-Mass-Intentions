package web

import (
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/intentions"
	"go-parafia/internal/masses"
)

var fixedNow = time.Date(2026, 10, 29, 10, 0, 0, 0, intentions.TimeZone)

func addFixed(t *testing.T, db *gorm.DB, f masses.FixedIntention) masses.FixedIntention {
	t.Helper()

	if f.ScheduleID == 0 {
		f.ScheduleID = 1
	}
	err := masses.SaveFixed(db, &f)
	if err != nil {
		t.Fatalf("SaveFixed: %v", err)
	}

	return f
}

func fixedList(t *testing.T, db *gorm.DB, scheduleID int) []masses.FixedIntention {
	t.Helper()

	s, err := masses.Find(db, scheduleID)
	if err != nil {
		t.Fatalf("Find: %v", err)
	}

	return s.Fixed
}

func loadedRows(t *testing.T, db *gorm.DB) []string {
	t.Helper()

	var list []intentions.Intention
	err := db.Order("mass_at, id").Find(&list).Error
	if err != nil {
		t.Fatal(err)
	}

	var rows []string
	for _, it := range list {
		label := it.Content
		if it.IsPlaceholder {
			label = "—"
		}
		if it.FixedID != 0 {
			label += " #" + strconv.Itoa(it.FixedID)
		}
		rows = append(rows, it.MassAt.In(intentions.TimeZone).Format(intentions.MassAtLayout)+" "+label)
	}

	return rows
}

func TestCreateSlotsAddsFixedIntentions(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	novena := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe"})
	dead := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Sunday, Time: "10:00", Content: "Intencje zbiorowe za zmarłych", IsDeceased: true, IsHidden: true})
	form := url.Values{"week": {"2026-11-02"}, "schedule": {"1"}}

	if rec := postSlots(t, h, form); rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}

	want := []string{
		"2026-11-02 06:30 —",
		"2026-11-02 18:00 Intencje nowennowe #" + strconv.Itoa(novena.ID),
		"2026-11-08 07:00 —",
		"2026-11-08 10:00 Intencje zbiorowe za zmarłych #" + strconv.Itoa(dead.ID),
	}
	if got := loadedRows(t, h.db); !slices.Equal(got, want) {
		t.Fatalf("week = %q, want %q", got, want)
	}

	var sunday intentions.Intention
	err := h.db.Where("fixed_id = ?", dead.ID).First(&sunday).Error
	if err != nil {
		t.Fatal(err)
	}
	if !sunday.IsDeceased || !sunday.IsHidden || sunday.IsOutsideParish || sunday.Amount != 0 || sunday.AcceptedOn != "" {
		t.Errorf("loaded intention = %+v", sunday)
	}

	err = h.db.Model(&intentions.Intention{}).Where("fixed_id = ?", novena.ID).Update("content", "Intencje nowennowe: + Jan").Error
	if err != nil {
		t.Fatal(err)
	}
	postSlots(t, h, form)

	if got := countIntentions(t, h.db); got != 4 {
		t.Errorf("intentions after reloading = %d, want 4 (no duplicates)", got)
	}
}

func TestCreateSlotsReplacesOldPlaceholderWithFixed(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	form := url.Values{"week": {"2026-11-02"}, "schedule": {"1"}}
	postSlots(t, h, form)

	novena := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe"})
	outside := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Sunday, Time: "07:00", Content: "Za parafian", IsOutsideParish: true})
	postSlots(t, h, form)

	want := []string{
		"2026-11-02 06:30 —",
		"2026-11-02 18:00 Intencje nowennowe #" + strconv.Itoa(novena.ID),
		"2026-11-08 07:00 —",
		"2026-11-08 07:00 Za parafian #" + strconv.Itoa(outside.ID),
		"2026-11-08 10:00 —",
	}
	if got := loadedRows(t, h.db); !slices.Equal(got, want) {
		t.Errorf("week = %q, want %q (outside intentions keep the parish slot)", got, want)
	}
}

func TestFixedPage(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	novena := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe", IsDeceased: true, IsHidden: true, IsOutsideParish: true})

	rec := serveRouter(t, h.db, http.MethodGet, "/settings/schedules/1/fixed?week=2026-11-04&mass=0-10:00", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()

	for _, want := range []string{
		`<title>Stałe intencje – Porządek zwykły – Go Parafia</title>`,
		`<a class="button" href="/settings?week=2026-11-02#porzadki" data-close>← Wróć do ustawień</a>`,
		`<h3 class="fixed-day-name">Poniedziałek</h3>`,
		`<div class="fixed-mass" id="msza-1-1800">`,
		`<span class="fixed-time">6:30</span>`,
		`<span class="badge badge-outside">poza parafią</span> Intencje nowennowe`,
		`za zmarłych</span>`,
		`ukryta na wydruku</span>`,
		`href="/settings/schedules/1/fixed?edit=` + strconv.Itoa(novena.ID) + `&amp;week=2026-11-02#formularz"`,
		`href="/settings/schedules/1/fixed?delete=` + strconv.Itoa(novena.ID) + `&amp;week=2026-11-02"`,
		`<a class="button-small fixed-add" href="/settings/schedules/1/fixed?mass=1-06%3A30&amp;week=2026-11-02#formularz">+ Dodaj</a>`,
		`<form class="settings-form" id="formularz" method="post" action="/settings/schedules/1/fixed">`,
		`<legend>Dodaj stałą intencję</legend>`,
		`<option value="0-10:00" selected>Niedziela, 10:00</option>`,
		`<option value="1-06:30">Poniedziałek, 6:30</option>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	if strings.Contains(body, "Wtorek") {
		t.Error("page lists a day without masses")
	}
	if strings.Index(body, "Poniedziałek</h3>") > strings.Index(body, "Niedziela</h3>") {
		t.Error("days are not listed from Monday")
	}
}

func TestFixedPageEditAndDeleteDialog(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	novena := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe", IsDeceased: true})
	id := strconv.Itoa(novena.ID)

	body := serveRouter(t, h.db, http.MethodGet, "/settings/schedules/1/fixed?edit="+id, nil).Body.String()
	for _, want := range []string{
		`<legend>Zmień stałą intencję</legend>`,
		`action="/settings/schedules/1/fixed/` + id + `"`,
		`<option value="1-18:00" selected>Poniedziałek, 18:00</option>`,
		`>Intencje nowennowe</textarea>`,
		`<input type="checkbox" name="deceased" checked> Za zmarłych`,
		`<div class="fixed-item is-editing">`,
		`<a class="button" href="/settings/schedules/1/fixed">Anuluj</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("edit page does not contain %q", want)
		}
	}

	body = serveRouter(t, h.db, http.MethodGet, "/settings/schedules/1/fixed?delete="+id+"&week=2026-11-02", nil).Body.String()
	for _, want := range []string{
		`<h2 class="dialog-title" id="dialog-title">Usunąć stałą intencję?</h2>`,
		`Poniedziałek, 18:00: „Intencje nowennowe”. Intencje już wczytane do tygodni zostaną bez zmian.`,
		`action="/settings/schedules/1/fixed/` + id + `/delete"`,
		`<main class="settings" inert>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("delete dialog does not contain %q", want)
		}
	}
}

func TestFixedPageNotFound(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	other := addFixed(t, h.db, masses.FixedIntention{ScheduleID: 2, Weekday: time.Sunday, Time: "09:00", Content: "Wakacyjna"})
	id := strconv.Itoa(other.ID)

	for _, target := range []string{
		"/settings/schedules/99/fixed",
		"/settings/schedules/1/fixed?edit=" + id,
		"/settings/schedules/1/fixed?delete=" + id,
		"/settings/schedules/1/fixed?edit=999",
	} {
		if rec := serveRouter(t, h.db, http.MethodGet, target, nil); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s: status = %d, want %d", target, rec.Code, http.StatusNotFound)
		}
	}
	if rec := serveRouter(t, h.db, http.MethodGet, "/settings/schedules/x/fixed", nil); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid id: status = %d, want %d", rec.Code, http.StatusBadRequest)
	}

	form := url.Values{"mass": {"1-18:00"}, "content": {"x"}}
	if rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed/"+id, form); rec.Code != http.StatusNotFound {
		t.Errorf("update of another schedule's intention: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed/"+id+"/delete", url.Values{}); rec.Code != http.StatusNotFound {
		t.Errorf("delete of another schedule's intention: status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	if got := fixedList(t, h.db, 2); len(got) != 1 || got[0].Content != "Wakacyjna" {
		t.Errorf("other schedule = %+v, want it untouched", got)
	}
}

func TestSaveFixedFromForm(t *testing.T) {
	h := newTestHandler(t, fixedNow)

	rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed", url.Values{
		"week": {"2026-11-04"}, "mass": {"1-18:00"}, "content": {"  Intencje nowennowe  "}, "deceased": {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusSeeOther, rec.Body.String())
	}
	if got, want := rec.Header().Get("Location"), "/settings/schedules/1/fixed?week=2026-11-02#msza-1-1800"; got != want {
		t.Errorf("Location = %q, want %q", got, want)
	}

	list := fixedList(t, h.db, 1)
	want := masses.FixedIntention{ID: list[0].ID, ScheduleID: 1, Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe", IsDeceased: true}
	if len(list) != 1 || list[0] != want {
		t.Fatalf("fixed = %+v, want %+v", list, want)
	}

	rec = serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed/"+strconv.Itoa(want.ID), url.Values{
		"mass": {"0-07:00"}, "content": {"Za parafian"}, "outside": {"on"}, "hidden": {"on"},
	})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("update: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	want = masses.FixedIntention{ID: want.ID, ScheduleID: 1, Weekday: time.Sunday, Time: "07:00", Content: "Za parafian", IsOutsideParish: true, IsHidden: true}
	if list := fixedList(t, h.db, 1); len(list) != 1 || list[0] != want {
		t.Errorf("fixed after update = %+v, want %+v", list, want)
	}

	rec = serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed/"+strconv.Itoa(want.ID)+"/delete", url.Values{"week": {"2026-11-02"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/settings/schedules/1/fixed?week=2026-11-02" {
		t.Errorf("delete: status = %d, Location = %q", rec.Code, rec.Header().Get("Location"))
	}
	if list := fixedList(t, h.db, 1); list != nil {
		t.Errorf("fixed after delete = %+v, want none", list)
	}
}

func TestSaveFixedValidation(t *testing.T) {
	tests := []struct {
		name string
		form url.Values
		want []string
	}{
		{
			name: "empty content and no mass",
			form: url.Values{"content": {"   "}},
			want: []string{`<p class="field-error" id="mass-error">Wybierz Mszę z listy.</p>`, `<p class="field-error" id="content-error">Pole jest wymagane.</p>`},
		},
		{
			name: "mass not in the schedule",
			form: url.Values{"mass": {"2-18:00"}, "content": {"Intencje nowennowe"}, "deceased": {"on"}},
			want: []string{`<p class="field-error" id="mass-error">Wybierz Mszę z listy.</p>`, `>Intencje nowennowe</textarea>`, `name="deceased" checked`},
		},
		{
			name: "garbage mass",
			form: url.Values{"mass": {"x-18:00"}, "content": {"Intencje nowennowe"}},
			want: []string{`<p class="field-error" id="mass-error">Wybierz Mszę z listy.</p>`},
		},
		{
			name: "content too long",
			form: url.Values{"mass": {"1-18:00"}, "content": {strings.Repeat("ą", 501)}},
			want: []string{`<p class="field-error" id="content-error">Maksymalnie 500 znaków.</p>`, `<option value="1-18:00" selected>`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newTestHandler(t, fixedNow)

			rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1/fixed", tt.form)

			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
			}
			body := rec.Body.String()
			for _, want := range append(tt.want, "Nie zapisano. Popraw zaznaczone pola") {
				if !strings.Contains(body, want) {
					t.Errorf("page does not contain %q", want)
				}
			}
			if list := fixedList(t, h.db, 1); list != nil {
				t.Errorf("fixed = %+v, want nothing saved", list)
			}
		})
	}
}

func TestSettingsListShowsFixedIntentions(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	addFixed(t, h.db, masses.FixedIntention{Weekday: time.Sunday, Time: "10:00", Content: "Za parafian"})
	addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe"})

	body := serveRouter(t, h.db, http.MethodGet, "/settings?week=2026-11-02", nil).Body.String()
	for _, want := range []string{
		`<p class="schedule-fixed" title="Pn 18:00 – Intencje nowennowe · Nd 10:00 – Za parafian">Stałe intencje: Pn 18:00 – Intencje nowennowe · Nd 10:00 – Za parafian</p>`,
		`<a class="button-small" href="/settings/schedules/1/fixed?week=2026-11-02" aria-label="Stałe intencje porządku Porządek zwykły">`,
		`<a class="button-small" href="/settings/schedules/2/fixed?week=2026-11-02" aria-label="Stałe intencje porządku Wakacje">`,
		`Godziny Mszy i stałe intencje wczytywane przyciskiem`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings do not contain %q", want)
		}
	}
	if strings.Count(body, `class="schedule-fixed"`) != 1 {
		t.Error("a schedule without fixed intentions shows the line")
	}

	body = serveRouter(t, h.db, http.MethodGet, "/settings?delete_schedule=1", nil).Body.String()
	if want := "Jego stałe intencje (2) też zostaną usunięte."; !strings.Contains(body, want) {
		t.Errorf("delete dialog does not contain %q", want)
	}
	body = serveRouter(t, h.db, http.MethodGet, "/settings?delete_schedule=2", nil).Body.String()
	if strings.Contains(body, "stałe intencje (") {
		t.Error("delete dialog mentions fixed intentions of a schedule without any")
	}
}

func TestSaveScheduleConfirmsDroppingFixed(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	novena := addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe"})
	addFixed(t, h.db, masses.FixedIntention{Weekday: time.Sunday, Time: "10:00", Content: "Za parafian"})
	form := url.Values{"week": {"2026-11-02"}, "name": {"Porządek zwykły"}, "day-1": {"6:30, 18:30"}, "day-0": {"7:00, 10:00"}}

	rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1", form)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnprocessableEntity)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`zapisanie usunie przypięte do nich stałe intencje`,
		`<li>Poniedziałek, 18:00 – Intencje nowennowe</li>`,
		`<input type="hidden" name="drop" value="` + strconv.Itoa(novena.ID) + `">`,
		`value="6:30, 18:30"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("form does not contain %q", want)
		}
	}
	if strings.Contains(body, "Za parafian") || strings.Contains(body, "Popraw zaznaczone pola") {
		t.Error("warning lists a kept intention or shows field errors")
	}
	if s, _ := masses.Find(h.db, 1); !slices.Equal(s.Masses[time.Monday], []string{"06:30", "18:00"}) || len(s.Fixed) != 2 {
		t.Fatalf("schedule changed before confirmation: %+v", s)
	}

	form.Set("drop", strconv.Itoa(novena.ID))
	form.Set("day-0", "7:00")
	rec = serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1", form)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "<li>Niedziela, 10:00 – Za parafian</li>") {
		t.Fatalf("a new orphan was not confirmed: status = %d", rec.Code)
	}

	form.Set("day-0", "7:00, 10:00")
	rec = serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1", form)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("confirmed: status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	s, err := masses.Find(h.db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(s.Masses[time.Monday], []string{"06:30", "18:30"}) {
		t.Errorf("monday = %q, want the new hours", s.Masses[time.Monday])
	}
	if len(s.Fixed) != 1 || s.Fixed[0].Content != "Za parafian" {
		t.Errorf("fixed = %+v, want only the Sunday one", s.Fixed)
	}
}

func TestSaveScheduleWithoutOrphansNeedsNoConfirmation(t *testing.T) {
	h := newTestHandler(t, fixedNow)
	addFixed(t, h.db, masses.FixedIntention{Weekday: time.Monday, Time: "18:00", Content: "Intencje nowennowe"})

	rec := serveRouter(t, h.db, http.MethodPost, "/settings/schedules/1", url.Values{"name": {"Zwykły"}, "day-1": {"7:00, 18:00"}})

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if list := fixedList(t, h.db, 1); len(list) != 1 {
		t.Errorf("fixed = %+v, want it kept", list)
	}
}
