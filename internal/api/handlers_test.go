package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

func newTestRouter(t *testing.T) (http.Handler, *gorm.DB) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "test.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("opening test database: %v", err)
	}

	err = db.AutoMigrate(&intentions.Intention{})
	if err != nil {
		t.Fatalf("migrating test database: %v", err)
	}

	router := chi.NewRouter()
	RegisterRoutes(router, db)

	return router, db
}

func doRequest(t *testing.T, router http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	return rec
}

func massAt(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.ParseInLocation(intentions.MassAtLayout, value, intentions.TimeZone)
	if err != nil {
		t.Fatalf("parsing mass time %q: %v", value, err)
	}

	return parsed
}

func equalIntention(a, b intentions.Intention) bool {
	return a.ID == b.ID &&
		a.MassAt.Equal(b.MassAt) &&
		a.Content == b.Content &&
		a.IsOutsideParish == b.IsOutsideParish
}

func seedIntention(t *testing.T, db *gorm.DB) intentions.Intention {
	t.Helper()

	intention := intentions.Intention{MassAt: massAt(t, "2026-11-01 08:00"), Content: "za śp. Jana", IsOutsideParish: true}
	err := db.Create(&intention).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}

	return intention
}

func decodeIntention(t *testing.T, rec *httptest.ResponseRecorder) intentions.Intention {
	t.Helper()

	var intention intentions.Intention
	err := json.NewDecoder(rec.Body).Decode(&intention)
	if err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	return intention
}

func loadIntention(t *testing.T, db *gorm.DB, id int) intentions.Intention {
	t.Helper()

	var intention intentions.Intention
	err := db.First(&intention, id).Error
	if err != nil {
		t.Fatalf("loading intention %d: %v", id, err)
	}

	return intention
}

func countIntentions(t *testing.T, db *gorm.DB) int64 {
	t.Helper()

	var count int64
	err := db.Model(&intentions.Intention{}).Count(&count).Error
	if err != nil {
		t.Fatalf("counting intentions: %v", err)
	}

	return count
}

func TestList(t *testing.T) {
	router, db := newTestRouter(t)
	seedIntention(t, db)
	seedIntention(t, db)

	rec := doRequest(t, router, http.MethodGet, "/intentions", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got []intentions.Intention
	err := json.NewDecoder(rec.Body).Decode(&got)
	if err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if len(got) != 2 {
		t.Errorf("got %d intentions, want 2", len(got))
	}
}

func TestListEmpty(t *testing.T) {
	router, _ := newTestRouter(t)

	rec := doRequest(t, router, http.MethodGet, "/intentions", "")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	body := strings.TrimSpace(rec.Body.String())
	if body != "[]" {
		t.Errorf("body = %s, want []", body)
	}
}

func TestShow(t *testing.T) {
	router, db := newTestRouter(t)

	want := intentions.Intention{MassAt: massAt(t, "2026-11-01 08:00"), Content: "za śp. Jana"}
	err := db.Create(&want).Error
	if err != nil {
		t.Fatalf("creating intention: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/intentions/1", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var got intentions.Intention
	err = json.NewDecoder(rec.Body).Decode(&got)
	if err != nil {
		t.Fatalf("decoding response: %v", err)
	}

	if !equalIntention(got, want) {
		t.Errorf("intention = %+v, want %+v", got, want)
	}
}

func TestShowNotFound(t *testing.T) {
	router, _ := newTestRouter(t)

	req := httptest.NewRequest(http.MethodGet, "/intentions/999", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestShowDatabaseError(t *testing.T) {
	router, db := newTestRouter(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("getting sql.DB: %v", err)
	}
	sqlDB.Close()

	rec := doRequest(t, router, http.MethodGet, "/intentions/1", "")

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreateDatabaseError(t *testing.T) {
	router, db := newTestRouter(t)

	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("getting sql.DB: %v", err)
	}
	sqlDB.Close()

	rec := doRequest(t, router, http.MethodPost, "/intentions",
		`{"mass_at":"2026-11-01 08:00","content":"za śp. Jana"}`)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestCreate(t *testing.T) {
	router, db := newTestRouter(t)

	rec := doRequest(t, router, http.MethodPost, "/intentions",
		`{"id":500,"mass_at":"2026-11-01 08:00","content":"  za śp. Jana  ","is_outside_parish":true}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
	}

	want := intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-01 08:00"), Content: "za śp. Jana", IsOutsideParish: true}

	got := decodeIntention(t, rec)
	if !equalIntention(got, want) {
		t.Errorf("response = %+v, want %+v", got, want)
	}

	saved := loadIntention(t, db, 1)
	if !equalIntention(saved, want) {
		t.Errorf("saved = %+v, want %+v", saved, want)
	}
}

func TestCreateInvalid(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{"invalid json", `{nie json`},
		{"missing mass_at", `{"content":"za śp. Jana"}`},
		{"invalid mass_at", `{"mass_at":"jutro","content":"za śp. Jana"}`},
		{"mass_at not on quarter hour", `{"mass_at":"2026-11-01 08:10","content":"za śp. Jana"}`},
		{"empty content", `{"mass_at":"2026-11-01 08:00","content":""}`},
		{"whitespace content", `{"mass_at":"2026-11-01 08:00","content":"   "}`},
		{"content too long", `{"mass_at":"2026-11-01 08:00","content":"` + strings.Repeat("a", 501) + `"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := newTestRouter(t)

			rec := doRequest(t, router, http.MethodPost, "/intentions", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			count := countIntentions(t, db)
			if count != 0 {
				t.Errorf("intentions in db = %d, want 0", count)
			}
		})
	}
}

func TestPut(t *testing.T) {
	router, db := newTestRouter(t)
	seedIntention(t, db)

	rec := doRequest(t, router, http.MethodPut, "/intentions/1",
		`{"id":500,"mass_at":"2026-11-02 18:00","content":"  za zdrowie Anny  "}`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	want := intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-02 18:00"), Content: "za zdrowie Anny", IsOutsideParish: false}

	got := decodeIntention(t, rec)
	if !equalIntention(got, want) {
		t.Errorf("response = %+v, want %+v", got, want)
	}

	saved := loadIntention(t, db, 1)
	if !equalIntention(saved, want) {
		t.Errorf("saved = %+v, want %+v", saved, want)
	}
}

func TestPatch(t *testing.T) {
	tests := []struct {
		name string
		body string
		want intentions.Intention
	}{
		{
			name: "content only, trimmed",
			body: `{"content":"  za zdrowie Anny  "}`,
			want: intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-01 08:00"), Content: "za zdrowie Anny", IsOutsideParish: true},
		},
		{
			name: "false overrides true",
			body: `{"is_outside_parish":false}`,
			want: intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-01 08:00"), Content: "za śp. Jana", IsOutsideParish: false},
		},
		{
			name: "id in body is ignored",
			body: `{"id":500,"content":"za zdrowie Anny"}`,
			want: intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-01 08:00"), Content: "za zdrowie Anny", IsOutsideParish: true},
		},
		{
			name: "empty object changes nothing",
			body: `{}`,
			want: intentions.Intention{ID: 1, MassAt: massAt(t, "2026-11-01 08:00"), Content: "za śp. Jana", IsOutsideParish: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := newTestRouter(t)
			seedIntention(t, db)

			rec := doRequest(t, router, http.MethodPatch, "/intentions/1", tt.body)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}

			got := decodeIntention(t, rec)
			if !equalIntention(got, tt.want) {
				t.Errorf("response = %+v, want %+v", got, tt.want)
			}

			saved := loadIntention(t, db, 1)
			if !equalIntention(saved, tt.want) {
				t.Errorf("saved = %+v, want %+v", saved, tt.want)
			}
		})
	}
}

func TestPutAndPatchInvalid(t *testing.T) {
	tests := []struct {
		name   string
		method string
		body   string
	}{
		{"put invalid json", http.MethodPut, `{nie json`},
		{"put missing mass_at", http.MethodPut, `{"content":"za zdrowie Anny"}`},
		{"put invalid mass_at", http.MethodPut, `{"mass_at":"jutro","content":"za zdrowie Anny"}`},
		{"patch invalid json", http.MethodPatch, `{nie json`},
		{"patch invalid mass_at", http.MethodPatch, `{"mass_at":"jutro"}`},
		{"patch empty content", http.MethodPatch, `{"content":""}`},
		{"patch whitespace content", http.MethodPatch, `{"content":"   "}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := newTestRouter(t)
			want := seedIntention(t, db)

			rec := doRequest(t, router, tt.method, "/intentions/1", tt.body)

			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			saved := loadIntention(t, db, 1)
			if !equalIntention(saved, want) {
				t.Errorf("saved = %+v, want unchanged %+v", saved, want)
			}
		})
	}
}

func TestDelete(t *testing.T) {
	router, db := newTestRouter(t)
	seedIntention(t, db)

	rec := doRequest(t, router, http.MethodDelete, "/intentions/1", "")

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	count := countIntentions(t, db)
	if count != 0 {
		t.Errorf("intentions in db = %d, want 0", count)
	}
}

func TestUnknownOrInvalidID(t *testing.T) {
	valid := `{"mass_at":"2026-11-01 08:00","content":"za śp. Jana"}`

	tests := []struct {
		name   string
		method string
		path   string
		body   string
		want   int
	}{
		{"show invalid id", http.MethodGet, "/intentions/abc", "", http.StatusBadRequest},
		{"put not found", http.MethodPut, "/intentions/999", valid, http.StatusNotFound},
		{"put invalid id", http.MethodPut, "/intentions/abc", valid, http.StatusBadRequest},
		{"patch not found", http.MethodPatch, "/intentions/999", valid, http.StatusNotFound},
		{"patch invalid id", http.MethodPatch, "/intentions/abc", valid, http.StatusBadRequest},
		{"delete not found", http.MethodDelete, "/intentions/999", "", http.StatusNotFound},
		{"delete invalid id", http.MethodDelete, "/intentions/abc", "", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := newTestRouter(t)

			rec := doRequest(t, router, tt.method, tt.path, tt.body)

			if rec.Code != tt.want {
				t.Errorf("status = %d, want %d", rec.Code, tt.want)
			}

			count := countIntentions(t, db)
			if count != 0 {
				t.Errorf("intentions in db = %d, want 0", count)
			}
		})
	}
}

func TestValidationErrorResponse(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
	}{
		{"create", http.MethodPost, "/intentions"},
		{"put", http.MethodPut, "/intentions/1"},
		{"patch", http.MethodPatch, "/intentions/1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, db := newTestRouter(t)
			seedIntention(t, db)

			rec := doRequest(t, router, tt.method, tt.path, `{"mass_at":"jutro","content":""}`)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			var got struct {
				Errors map[string]string `json:"errors"`
			}
			err := json.NewDecoder(rec.Body).Decode(&got)
			if err != nil {
				t.Fatalf("decoding response: %v", err)
			}

			for _, field := range []string{"mass_at", "content"} {
				_, ok := got.Errors[field]
				if !ok {
					t.Errorf("errors = %v, want key %q", got.Errors, field)
				}
			}
		})
	}
}

func TestMassAtJSONFormat(t *testing.T) {
	tests := []struct {
		name   string
		massAt string
		want   string
	}{
		{"winter time", "2026-11-01 08:00", "2026-11-01T08:00:00+01:00"},
		{"summer time", "2026-07-05 10:45", "2026-07-05T10:45:00+02:00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router, _ := newTestRouter(t)

			rec := doRequest(t, router, http.MethodPost, "/intentions",
				`{"mass_at":"`+tt.massAt+`","content":"za śp. Jana"}`)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusCreated)
			}

			var got struct {
				MassAt string `json:"mass_at"`
			}
			err := json.NewDecoder(rec.Body).Decode(&got)
			if err != nil {
				t.Fatalf("decoding response: %v", err)
			}

			if got.MassAt != tt.want {
				t.Errorf("mass_at = %q, want %q", got.MassAt, tt.want)
			}
		})
	}
}
