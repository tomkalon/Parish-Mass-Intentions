package intentions

import "time"

type Intention struct {
	ID              int       `json:"id"`
	MassAt          time.Time `json:"mass_at"`
	Content         string    `json:"content"`
	IsOutsideParish bool      `json:"is_outside_parish"`
}

type Input struct {
	MassAt          string `json:"mass_at" validate:"required,datetime=2006-01-02 15:04,quarter_hour"`
	Content         string `json:"content" validate:"required,max=500"`
	IsOutsideParish bool   `json:"is_outside_parish"`
}

func (i *Intention) Apply(input Input) {
	i.MassAt, _ = time.ParseInLocation(MassAtLayout, input.MassAt, TimeZone)
	i.Content = input.Content
	i.IsOutsideParish = input.IsOutsideParish
}
