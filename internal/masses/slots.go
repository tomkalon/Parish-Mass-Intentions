package masses

import (
	"slices"
	"strings"
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

			added := false
			for _, f := range s.FixedAt(date.Weekday(), massTime) {
				if fixedLoaded(f, at, existing) {
					continue
				}

				slots = append(slots, intentions.Intention{
					MassAt:          at,
					Content:         f.Content,
					IsOutsideParish: f.IsOutsideParish,
					IsDeceased:      f.IsDeceased,
					IsHidden:        f.IsHidden,
					FixedID:         f.ID,
				})
				added = true
			}
			if added {
				continue
			}

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

func fixedLoaded(f FixedIntention, at time.Time, existing []intentions.Intention) bool {
	content := strings.TrimSpace(f.Content)

	return slices.ContainsFunc(existing, func(intention intentions.Intention) bool {
		if f.ID != 0 && intention.FixedID == f.ID {
			return true
		}

		return !intention.IsPlaceholder &&
			intention.MassAt.Equal(at) &&
			strings.EqualFold(strings.TrimSpace(intention.Content), content)
	})
}
