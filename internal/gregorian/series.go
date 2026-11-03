package gregorian

import (
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"

	"go-parafia/internal/intentions"
)

const Length = 30

var ErrSeriesNotFound = errors.New("gregorian series not found")

type Series struct {
	ID              int        `json:"id"`
	Content         string     `json:"content"`
	IsOutsideParish bool       `json:"is_outside_parish" gorm:"not null;default:false"`
	StoppedAt       *time.Time `json:"stopped_at"`
}

func (Series) TableName() string {
	return "gregorian_series"
}

type Status int

const (
	StatusActive Status = iota
	StatusFinished
	StatusStopped
)

type Mass struct {
	ID     int
	MassAt time.Time
	Amount intentions.Grosze
}

func (m Mass) Date() string {
	return m.MassAt.In(intentions.TimeZone).Format(intentions.DateLayout)
}

type Summary struct {
	Series Series
	Masses []Mass
	Done   int
	Status Status

	GapsAfter []int
	Gaps      []time.Time
	NextDay   time.Time
	End       time.Time
}

func (s Summary) Count() int {
	return len(s.Masses)
}

func (s Summary) Amount() intentions.Grosze {
	var total intentions.Grosze
	for _, m := range s.Masses {
		total += m.Amount
	}

	return total
}

func (s Summary) FirstID() int {
	if len(s.Masses) == 0 {
		return 0
	}

	return s.Masses[0].ID
}

func (s Summary) First() time.Time {
	if len(s.Masses) == 0 {
		return time.Time{}
	}

	return s.Masses[0].MassAt.In(intentions.TimeZone)
}

func (s Summary) Last() time.Time {
	if len(s.Masses) == 0 {
		return time.Time{}
	}

	return s.Masses[len(s.Masses)-1].MassAt.In(intentions.TimeZone)
}

func (s Summary) HasDay(date string) bool {
	return s.MassOn(date) != nil
}

func (s Summary) MassOn(date string) *Mass {
	for i := range s.Masses {
		if s.Masses[i].Date() == date {
			return &s.Masses[i]
		}
	}

	return nil
}

func (s Summary) StepOn(date string) int {
	step := 1
	for _, m := range s.Masses {
		if m.Date() < date {
			step++
		}
	}

	return step
}

func (s Summary) NextTime() string {
	if len(s.Masses) == 0 {
		return ""
	}

	return s.Last().Format("15:04")
}

func (s Summary) Dates() []string {
	dates := make([]string, 0, len(s.Masses))
	for _, m := range s.Masses {
		dates = append(dates, m.Date())
	}

	return dates
}

func (s Summary) Planned() []time.Time {
	var days []time.Time
	if s.NextDay.IsZero() {
		return days
	}

	day := s.NextDay
	for len(s.Masses)+len(days) < Length {
		days = append(days, day)
		day = NextMassDay(day)
	}

	return days
}

func Create(db *gorm.DB, content string, outside bool) (Series, error) {
	series := Series{Content: content, IsOutsideParish: outside}
	err := db.Create(&series).Error

	return series, err
}

func Find(db *gorm.DB, id int) (Series, error) {
	var series Series
	err := db.Limit(1).Find(&series, id).Error
	if err != nil {
		return Series{}, err
	}
	if series.ID == 0 {
		return Series{}, ErrSeriesNotFound
	}

	return series, nil
}

func Load(db *gorm.DB, id int, now time.Time) (Summary, error) {
	series, err := Find(db, id)
	if err != nil {
		return Summary{}, err
	}

	summaries, err := summarize(db, []Series{series}, now)
	if err != nil {
		return Summary{}, err
	}

	return summaries[0], nil
}

func Active(db *gorm.DB, now time.Time) ([]Summary, error) {
	var ids []int
	err := db.Model(&intentions.Intention{}).
		Where("series_id > 0").
		Group("series_id").
		Having("COUNT(*) < ? OR MAX(mass_at) >= ?", Length, now.In(intentions.TimeZone)).
		Pluck("series_id", &ids).Error
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, nil
	}

	var series []Series
	err = db.Where("id IN ? AND stopped_at IS NULL", ids).Find(&series).Error
	if err != nil {
		return nil, err
	}

	summaries, err := summarize(db, series, now)
	if err != nil {
		return nil, err
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		return a.First().Compare(b.First())
	})

	return summaries, nil
}

func closedQuery(db *gorm.DB, now time.Time) *gorm.DB {
	closed := db.Model(&intentions.Intention{}).
		Select("series_id").
		Where("series_id > 0").
		Group("series_id").
		Having("COUNT(*) >= ? AND MAX(mass_at) < ?", Length, now.In(intentions.TimeZone))

	return db.Model(&Series{}).Where("stopped_at IS NOT NULL OR id IN (?)", closed)
}

func CountClosed(db *gorm.DB, now time.Time) (int64, error) {
	var total int64
	err := closedQuery(db, now).Count(&total).Error

	return total, err
}

func Closed(db *gorm.DB, now time.Time, limit int) ([]Summary, int64, error) {
	query := closedQuery(db, now)

	var total int64
	err := query.Session(&gorm.Session{}).Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	var series []Series
	err = query.Session(&gorm.Session{}).Order("id DESC").Limit(limit).Find(&series).Error
	if err != nil {
		return nil, 0, err
	}

	summaries, err := summarize(db, series, now)
	if err != nil {
		return nil, 0, err
	}
	slices.SortFunc(summaries, func(a, b Summary) int {
		return b.Last().Compare(a.Last())
	})

	return summaries, total, nil
}

func Steps(db *gorm.DB, ids []int) (map[int]int, error) {
	masses, err := loadMasses(db, ids)
	if err != nil {
		return nil, err
	}

	steps := make(map[int]int)
	for _, list := range masses {
		for i, m := range list {
			steps[m.ID] = i + 1
		}
	}

	return steps, nil
}

func Stop(db *gorm.DB, id int, now time.Time) error {
	result := db.Model(&Series{}).Where("id = ? AND stopped_at IS NULL", id).Update("stopped_at", now.In(intentions.TimeZone))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		_, err := Find(db, id)
		return err
	}

	return nil
}

func Rename(db *gorm.DB, id int, content string) error {
	return db.Transaction(func(tx *gorm.DB) error {
		err := tx.Model(&Series{}).Where("id = ?", id).Update("content", content).Error
		if err != nil {
			return err
		}

		return tx.Model(&intentions.Intention{}).Where("series_id = ?", id).Update("content", content).Error
	})
}

func Normalize(db *gorm.DB, id int) error {
	var masses []intentions.Intention
	err := db.Select("id", "amount").Where("series_id = ?", id).Order("mass_at, id").Find(&masses).Error
	if err != nil {
		return err
	}
	if len(masses) == 0 {
		return db.Delete(&Series{}, id).Error
	}

	var total intentions.Grosze
	for _, m := range masses {
		total += m.Amount
	}
	for i, m := range masses {
		want := intentions.Grosze(0)
		if i == 0 {
			want = total
		}
		if m.Amount == want {
			continue
		}
		err = db.Model(&intentions.Intention{}).Where("id = ?", m.ID).Update("amount", want).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func Release(db *gorm.DB, deleted []intentions.Intention) error {
	extra := make(map[int]intentions.Grosze)
	var ids []int
	for _, intention := range deleted {
		if intention.SeriesID == 0 {
			continue
		}
		if !slices.Contains(ids, intention.SeriesID) {
			ids = append(ids, intention.SeriesID)
		}
		extra[intention.SeriesID] += intention.Amount
	}

	for _, id := range ids {
		if extra[id] > 0 {
			var first intentions.Intention
			err := db.Select("id", "amount").Where("series_id = ?", id).Order("mass_at, id").Limit(1).Find(&first).Error
			if err != nil {
				return err
			}
			if first.ID > 0 {
				err = db.Model(&intentions.Intention{}).Where("id = ?", first.ID).Update("amount", first.Amount+extra[id]).Error
				if err != nil {
					return err
				}
			}
		}

		err := Normalize(db, id)
		if err != nil {
			return err
		}
	}

	return nil
}

func NextMassDay(day time.Time) time.Time {
	next := day.AddDate(0, 0, 1)
	for withoutMass(next) {
		next = next.AddDate(0, 0, 1)
	}

	return next
}

func withoutMass(day time.Time) bool {
	easterDay := Easter(day.Year())
	y, m, d := day.Date()
	date := time.Date(y, m, d, 0, 0, 0, 0, intentions.TimeZone)

	return date.Equal(easterDay.AddDate(0, 0, -2)) || date.Equal(easterDay.AddDate(0, 0, -1))
}

func Easter(year int) time.Time {
	a := year % 19
	b := year / 100
	c := year % 100
	d := b / 4
	e := b % 4
	f := (b + 8) / 25
	g := (b - f + 1) / 3
	h := (19*a + b - d - g + 15) % 30
	i := c / 4
	k := c % 4
	l := (32 + 2*e + 2*i - h - k) % 7
	m := (a + 11*h + 22*l) / 451
	month := (h + l - 7*m + 114) / 31
	day := (h+l-7*m+114)%31 + 1

	return time.Date(year, time.Month(month), day, 0, 0, 0, 0, intentions.TimeZone)
}

func loadMasses(db *gorm.DB, ids []int) (map[int][]Mass, error) {
	masses := make(map[int][]Mass)
	if len(ids) == 0 {
		return masses, nil
	}

	var list []intentions.Intention
	err := db.Select("id", "series_id", "mass_at", "amount").
		Where("series_id IN ?", ids).
		Order("mass_at, id").
		Find(&list).Error
	if err != nil {
		return nil, err
	}

	for _, intention := range list {
		masses[intention.SeriesID] = append(masses[intention.SeriesID], Mass{
			ID:     intention.ID,
			MassAt: intention.MassAt.In(intentions.TimeZone),
			Amount: intention.Amount,
		})
	}

	return masses, nil
}

func summarize(db *gorm.DB, series []Series, now time.Time) ([]Summary, error) {
	ids := make([]int, 0, len(series))
	for _, s := range series {
		ids = append(ids, s.ID)
	}

	masses, err := loadMasses(db, ids)
	if err != nil {
		return nil, err
	}

	summaries := make([]Summary, 0, len(series))
	for _, s := range series {
		summaries = append(summaries, newSummary(s, masses[s.ID], now))
	}

	return summaries, nil
}

func newSummary(series Series, masses []Mass, now time.Time) Summary {
	s := Summary{Series: series, Masses: masses}

	for _, m := range masses {
		if m.MassAt.Before(now) {
			s.Done++
		}
	}

	switch {
	case series.StoppedAt != nil:
		s.Status = StatusStopped
	case len(masses) >= Length && !s.Last().After(now):
		s.Status = StatusFinished
	default:
		s.Status = StatusActive
	}

	for i := 1; i < len(masses); i++ {
		day := midnight(masses[i-1].MassAt)
		current := midnight(masses[i].MassAt)
		gap := false
		for next := NextMassDay(day); next.Before(current); next = NextMassDay(next) {
			s.Gaps = append(s.Gaps, next)
			gap = true
		}
		if gap {
			s.GapsAfter = append(s.GapsAfter, i)
		}
	}

	if len(masses) > 0 && len(masses) < Length && s.Status == StatusActive {
		s.NextDay = NextMassDay(midnight(s.Last()))
		planned := s.Planned()
		s.End = planned[len(planned)-1]
	} else if len(masses) > 0 {
		s.End = midnight(s.Last())
	}

	return s
}

func midnight(t time.Time) time.Time {
	y, m, d := t.In(intentions.TimeZone).Date()

	return time.Date(y, m, d, 0, 0, 0, 0, intentions.TimeZone)
}
