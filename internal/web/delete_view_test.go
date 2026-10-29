package web

import (
	"testing"
	"time"

	"go-parafia/internal/intentions"
)

func TestNewDeleteView(t *testing.T) {
	start := time.Date(2026, 11, 2, 0, 0, 0, 0, intentions.TimeZone)

	tests := []struct {
		name     string
		outside  bool
		wantWhen string
	}{
		{name: "parish", outside: false, wantWhen: "Wtorek 3.11., godz. 6:30"},
		{name: "outside the parish", outside: true, wantWhen: "Wtorek 3.11., godz. 6:30 · poza parafią"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := newDeleteView(start, newIntention(t, 42, "2026-11-03 06:30", tt.outside, "+ Jan Kowalski"))

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
