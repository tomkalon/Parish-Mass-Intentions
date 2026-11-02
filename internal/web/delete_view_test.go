package web

import (
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestNewDeleteView(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		name      string
		outside   bool
		deceased  bool
		gregorian bool
		wantWhen  string
	}{
		{name: "parish", wantWhen: "Wtorek 3.11., godz. 6:30"},
		{name: "outside the parish", outside: true, wantWhen: "Wtorek 3.11., godz. 6:30 · poza parafią"},
		{name: "for the deceased", deceased: true, wantWhen: "Wtorek 3.11., godz. 6:30 · za zmarłych"},
		{
			name:      "gregorian outside the parish",
			outside:   true,
			deceased:  true,
			gregorian: true,
			wantWhen:  "Wtorek 3.11., godz. 6:30 · poza parafią · za zmarłych · msza gregoriańska",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			intention := newIntention(t, 42, "2026-11-03 06:30", tt.outside, "+ Jan Kowalski")
			intention.IsDeceased = tt.deceased
			intention.IsGregorian = tt.gregorian

			got := newDeleteView(start, intention)

			want := deleteView{
				Action:   "/intentions/42/delete",
				When:     tt.wantWhen,
				Content:  "+ Jan Kowalski",
				CloseURL: "/?week=2026-11-02",
			}
			if got != want {
				t.Errorf("newDeleteView = %+v, want %+v", got, want)
			}
		})
	}
}
