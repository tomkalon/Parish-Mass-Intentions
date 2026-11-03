package web

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"go-parafia/internal/backup"
	"go-parafia/internal/calendar"
	"go-parafia/internal/database"
	"go-parafia/internal/gregorian"
	"go-parafia/internal/intentions"
)

const benchYears = 30

var benchWeek = intentions.WeekStart(time.Date(2011, 11, 7, 0, 0, 0, 0, intentions.TimeZone)).Format(intentions.DateLayout)

func seedParishYears(b *testing.B, db *gorm.DB, years int) {
	b.Helper()

	r := rand.New(rand.NewPCG(1, 2))
	first := intentions.WeekStart(time.Date(2026-years+1, 1, 5, 0, 0, 0, 0, intentions.TimeZone))
	var list []intentions.Intention
	var days []calendar.Day
	var series []gregorian.Series

	for d := range years * 364 {
		day := first.AddDate(0, 0, d)
		if d%gregorian.Length == 0 {
			series = append(series, gregorian.Series{ID: len(series) + 1, Content: fmt.Sprintf("+ Seria %d", len(series)+1)})
		}
		masses, perMass := []int{6*60 + 30, 18 * 60}, 3
		if day.Weekday() == time.Sunday {
			masses, perMass = []int{7 * 60, 8*60 + 30, 10 * 60, 11*60 + 30, 15 * 60, 18 * 60}, 1
		}
		for i, m := range masses {
			for k := range perMass + r.IntN(2) {
				intention := intentions.Intention{
					MassAt:          time.Date(day.Year(), day.Month(), day.Day(), m/60, m%60, 0, 0, intentions.TimeZone),
					Content:         fmt.Sprintf("+ Jan Kowalski w %d rocznicę śmierci od rodziny", 1+r.IntN(40)),
					IsOutsideParish: k > 0 && r.IntN(3) == 0,
					Amount:          intentions.Grosze(r.IntN(5)) * 5000,
				}
				if i == 0 && k == 0 {
					intention.SeriesID = d/gregorian.Length + 1
					intention.IsGregorian = true
					if d%gregorian.Length > 0 {
						intention.Amount = 0
					}
				}
				list = append(list, intention)
			}
		}
		days = append(days, calendar.Day{Date: day.Format(intentions.DateLayout), Feast: "wspomnienie św. Karola Boromeusza, biskupa"})
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		err := tx.CreateInBatches(series, 500).Error
		if err != nil {
			return err
		}
		err = tx.CreateInBatches(list, 500).Error
		if err != nil {
			return err
		}
		return tx.CreateInBatches(days, 500).Error
	})
	if err != nil {
		b.Fatal(err)
	}
}

func largeParish(b *testing.B) (http.Handler, *backup.Backups) {
	b.Helper()

	dir := b.TempDir()
	db, err := database.Open(filepath.Join(dir, "data.db"))
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			err = sqlDB.Close()
		}
		if err != nil {
			b.Errorf("closing database: %v", err)
		}
	})
	seedParishYears(b, db, benchYears)

	backups, err := backup.Load(db, "")
	if err != nil {
		b.Fatal(err)
	}
	err = backups.SetDir(b.TempDir())
	if err != nil {
		b.Fatal(err)
	}

	router := chi.NewRouter()
	RegisterRoutes(router, db, Options{DataDir: dir, Backups: backups})

	return router, backups
}

func BenchmarkWeekPageLargeParish(b *testing.B) {
	router, _ := largeParish(b)

	for b.Loop() {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/?week="+benchWeek, nil))
		if rec.Code != http.StatusOK {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}

func BenchmarkSaveIntentionLargeParish(b *testing.B) {
	router, _ := largeParish(b)
	form := url.Values{"week": {benchWeek}, "day": {benchWeek}, "time": {"18:00"}, "content": {"+ Jan Kowalski"}}.Encode()

	for b.Loop() {
		req := httptest.NewRequest(http.MethodPost, "/intentions", strings.NewReader(form))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusSeeOther {
			b.Fatalf("status = %d", rec.Code)
		}
	}
}

func BenchmarkPaymentsLargeParish(b *testing.B) {
	router, _ := largeParish(b)

	for _, tab := range []string{paymentsWeek, paymentsMonth, paymentsYear} {
		b.Run(tab, func(b *testing.B) {
			target := "/?week=" + benchWeek + "&payments=" + tab
			for b.Loop() {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
				if rec.Code != http.StatusOK {
					b.Fatalf("status = %d", rec.Code)
				}
			}
		})
	}
}

func BenchmarkGregorianLargeParish(b *testing.B) {
	router, _ := largeParish(b)

	for _, tab := range []string{seriesActive, seriesClosed} {
		b.Run(tab, func(b *testing.B) {
			target := "/?week=" + benchWeek + "&gregorian=" + tab
			for b.Loop() {
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
				if rec.Code != http.StatusOK {
					b.Fatalf("status = %d", rec.Code)
				}
			}
		})
	}
}

func BenchmarkBackupLargeParish(b *testing.B) {
	_, backups := largeParish(b)

	for b.Loop() {
		_, err := backups.Run()
		if err != nil {
			b.Fatal(err)
		}
	}
}
