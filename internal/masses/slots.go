package masses

import (
	"slices"
	"time"

	"go-parafia/internal/intentions"
)

func (s Schedule) Slots(weekStart time.Time, existing []intentions.Intention) []intentions.Intention {
	weekStart = weekStart.In(intentions.TimeZone)

	var slots []intentions.Intention
	for i := range 7 {
		date := weekStart.AddDate(0, 0, i)

		for _, massTime := range s.Masses[date.Weekday()] {
			at, _ := time.ParseInLocation(intentions.MassAtLayout, date.Format(intentions.DateLayout)+" "+massTime, intentions.TimeZone)

			taken := slices.ContainsFunc(existing, func(intention intentions.Intention) bool {
				return intention.MassAt.Equal(at)
			})
			if taken {
				continue
			}

			slots = append(slots, intentions.Intention{MassAt: at, IsPlaceholder: true})
		}
	}

	return slots
}
