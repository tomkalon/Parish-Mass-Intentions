package intentions

import "testing"

func TestApplyClearsPlaceholder(t *testing.T) {
	intention := Intention{IsPlaceholder: true}

	intention.Apply(Input{MassAt: "2026-11-02 18:00", Content: "+ Jan Kowalski"})

	if intention.IsPlaceholder {
		t.Error("IsPlaceholder = true after Apply, want false")
	}
	if intention.Content != "+ Jan Kowalski" {
		t.Errorf("Content = %q, want %q", intention.Content, "+ Jan Kowalski")
	}
}

func TestApplyCopiesFlags(t *testing.T) {
	intention := Intention{IsGregorian: true, IsDeceased: true}

	intention.Apply(Input{MassAt: "2026-11-02 18:00", Content: "+ Anna Kutyba", IsGregorian: true})
	if !intention.IsGregorian || intention.IsDeceased {
		t.Errorf("flags = gregorian %v, deceased %v; want true, false", intention.IsGregorian, intention.IsDeceased)
	}

	intention.Apply(Input{MassAt: "2026-11-02 18:00", Content: "+ Anna Kutyba", IsDeceased: true})
	if intention.IsGregorian || !intention.IsDeceased {
		t.Errorf("flags = gregorian %v, deceased %v; want false, true", intention.IsGregorian, intention.IsDeceased)
	}
}
