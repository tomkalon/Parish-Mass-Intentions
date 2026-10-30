package settings

import "gorm.io/gorm"

const settingID = 1

type Setting struct {
	ID               int    `json:"id"`
	SelectedSchedule string `json:"selected_schedule"`
}

func Load(db *gorm.DB) (Setting, error) {
	var setting Setting
	err := db.Limit(1).Find(&setting, settingID).Error

	return setting, err
}

func SaveSchedule(db *gorm.DB, name string) error {
	return db.Save(&Setting{ID: settingID, SelectedSchedule: name}).Error
}
