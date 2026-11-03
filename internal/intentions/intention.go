package intentions

import "time"

type Intention struct {
	ID              int       `json:"id"`
	MassAt          time.Time `json:"mass_at" gorm:"index;index:idx_intentions_series_mass,priority:2"`
	Content         string    `json:"content"`
	IsOutsideParish bool      `json:"is_outside_parish"`
	IsPlaceholder   bool      `json:"is_placeholder" gorm:"not null;default:false"`
	IsGregorian     bool      `json:"is_gregorian" gorm:"not null;default:false"`
	IsDeceased      bool      `json:"is_deceased" gorm:"not null;default:false"`
	IsHidden        bool      `json:"is_hidden" gorm:"not null;default:false"`
	Amount          Grosze    `json:"amount" gorm:"not null;default:0"`
	SeriesID        int       `json:"series_id" gorm:"index:idx_intentions_series_mass,priority:1;not null;default:0"`
}

type Input struct {
	MassAt          string `json:"mass_at" validate:"required,datetime=2006-01-02 15:04,quarter_hour"`
	Content         string `json:"content" validate:"required,max=500"`
	IsOutsideParish bool   `json:"is_outside_parish"`
	IsGregorian     bool   `json:"is_gregorian"`
	IsDeceased      bool   `json:"is_deceased"`
	IsHidden        bool   `json:"is_hidden"`
	Amount          Grosze `json:"amount" validate:"amount"`
}

func (i *Intention) Apply(input Input) {
	i.MassAt, _ = time.ParseInLocation(MassAtLayout, input.MassAt, TimeZone)
	i.Content = input.Content
	i.IsOutsideParish = input.IsOutsideParish
	i.IsGregorian = input.IsGregorian
	i.IsDeceased = input.IsDeceased || input.IsGregorian
	i.IsHidden = input.IsHidden
	i.Amount = input.Amount
	i.IsPlaceholder = false
}
