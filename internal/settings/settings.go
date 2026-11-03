package settings

import (
	"slices"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"go-parafia/internal/intentions"
)

const (
	settingID         = 1
	DefaultParishName = "Parafia pw. Trójcy Przenajświętszej i św. Leopolda w Rzezawie"
	MaxQuickAmounts   = 6
)

var DefaultQuickAmounts = []intentions.Grosze{5000, 10000, 15000, 20000}

type Setting struct {
	ID                 int                 `json:"id"`
	ParishName         string              `json:"parish_name"`
	Address            string              `json:"address"`
	SelectedScheduleID int                 `json:"selected_schedule_id"`
	FeastImport        FeastImport         `json:"feast_import" gorm:"embedded;embeddedPrefix:feast_import_"`
	QuickAmounts       []intentions.Grosze `json:"quick_amounts" gorm:"serializer:json"`
}

type FeastImport struct {
	SkipSolemnities       bool `json:"skip_solemnities"`
	SkipFeasts            bool `json:"skip_feasts"`
	SkipMemorials         bool `json:"skip_memorials"`
	SkipOptionalMemorials bool `json:"skip_optional_memorials"`
	SkipOtherDays         bool `json:"skip_other_days"`
}

func Load(db *gorm.DB) (Setting, error) {
	var setting Setting
	err := db.Limit(1).Find(&setting, settingID).Error
	if setting.ParishName == "" {
		setting.ParishName = DefaultParishName
	}
	if len(setting.QuickAmounts) == 0 {
		setting.QuickAmounts = slices.Clone(DefaultQuickAmounts)
	}

	return setting, err
}

func SaveSchedule(db *gorm.DB, id int) error {
	return upsert(db, Setting{SelectedScheduleID: id}, "selected_schedule_id")
}

func SaveParish(db *gorm.DB, setting Setting) error {
	return upsert(db, setting,
		"parish_name",
		"address",
		"feast_import_skip_solemnities",
		"feast_import_skip_feasts",
		"feast_import_skip_memorials",
		"feast_import_skip_optional_memorials",
		"feast_import_skip_other_days",
		"quick_amounts",
	)
}

func upsert(db *gorm.DB, setting Setting, columns ...string) error {
	setting.ID = settingID

	return db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(&setting).Error
}
