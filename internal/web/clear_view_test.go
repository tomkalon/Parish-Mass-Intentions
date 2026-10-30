package web

import (
	"testing"

	"go-parafia/internal/intentions"
)

func TestIntentionsCount(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{
		{n: 0, want: "0 intencji"},
		{n: 1, want: "1 intencja"},
		{n: 2, want: "2 intencje"},
		{n: 4, want: "4 intencje"},
		{n: 5, want: "5 intencji"},
		{n: 11, want: "11 intencji"},
		{n: 12, want: "12 intencji"},
		{n: 14, want: "14 intencji"},
		{n: 21, want: "21 intencji"},
		{n: 22, want: "22 intencje"},
		{n: 112, want: "112 intencji"},
		{n: 123, want: "123 intencje"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := intentionsCount(tt.n); got != tt.want {
				t.Errorf("intentionsCount(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}

func TestNewClearView(t *testing.T) {
	slot := newIntention(t, 1, "2026-11-02 06:30", false, "")
	slot.IsPlaceholder = true

	view := newClearView([]intentions.Intention{
		slot,
		newIntention(t, 2, "2026-11-02 18:00", false, "+ Jan Kowalski"),
		newIntention(t, 3, "2026-11-08 10:00", true, "+ Anna Nowak"),
	})

	if view.Count != "3 intencje" {
		t.Errorf("Count = %q, want %q", view.Count, "3 intencje")
	}
	if view.Placeholders != 1 {
		t.Errorf("Placeholders = %d, want 1", view.Placeholders)
	}
}
