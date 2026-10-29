package intentions

import (
	"maps"
	"strings"
	"testing"
)

func TestValidationMessages(t *testing.T) {
	validate := NewValidator()

	tests := []struct {
		name  string
		input Input
		want  map[string]string
	}{
		{
			name:  "missing mass time and content",
			input: Input{},
			want: map[string]string{
				"mass_at": "Pole jest wymagane.",
				"content": "Pole jest wymagane.",
			},
		},
		{
			name:  "invalid mass time format",
			input: Input{MassAt: "jutro", Content: "za śp. Jana"},
			want:  map[string]string{"mass_at": "Nieprawidłowy format, oczekiwano RRRR-MM-DD GG:MM."},
		},
		{
			name:  "mass time not on a quarter hour",
			input: Input{MassAt: "2026-11-02 18:10", Content: "za śp. Jana"},
			want:  map[string]string{"mass_at": "Godzina musi być pełnym kwadransem (:00, :15, :30, :45)."},
		},
		{
			name:  "content too long",
			input: Input{MassAt: "2026-11-02 18:00", Content: strings.Repeat("a", 501)},
			want:  map[string]string{"content": "Maksymalnie 500 znaków."},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validate.Struct(tt.input)

			got, ok := ValidationMessages(err)
			if !ok {
				t.Fatalf("ValidationMessages(%v) ok = false, want true", err)
			}

			if !maps.Equal(got, tt.want) {
				t.Errorf("messages = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidationMessagesForValidInput(t *testing.T) {
	err := NewValidator().Struct(Input{MassAt: "2026-11-02 18:15", Content: "za śp. Jana"})
	if err != nil {
		t.Fatalf("validating correct input: %v", err)
	}

	if _, ok := ValidationMessages(err); ok {
		t.Error("ValidationMessages(nil) ok = true, want false")
	}
}
