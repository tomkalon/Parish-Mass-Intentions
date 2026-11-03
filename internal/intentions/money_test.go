package intentions

import (
	"errors"
	"testing"
)

func TestParseGrosze(t *testing.T) {
	tests := []struct {
		text string
		want Grosze
	}{
		{text: "", want: 0},
		{text: "   ", want: 0},
		{text: "0", want: 0},
		{text: "150", want: 15000},
		{text: " 150 ", want: 15000},
		{text: "150,5", want: 15050},
		{text: "150,50", want: 15050},
		{text: "150.05", want: 15005},
		{text: "0,99", want: 99},
		{text: "007", want: 700},
		{text: "1 500", want: 150000},
		{text: "1 500", want: 150000},
		{text: "1 500,25", want: 150025},
		{text: "150 zł", want: 15000},
		{text: "150zł", want: 15000},
		{text: "150 ZŁ", want: 15000},
		{text: "zł", want: 0},
		{text: "99 999,99", want: MaxAmount},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := ParseGrosze(tt.text)
			if err != nil {
				t.Fatalf("ParseGrosze(%q) error = %v", tt.text, err)
			}
			if got != tt.want {
				t.Errorf("ParseGrosze(%q) = %d, want %d", tt.text, got, tt.want)
			}
		})
	}
}

func TestParseGroszeInvalid(t *testing.T) {
	tests := []struct {
		text string
		want error
	}{
		{text: "abc", want: ErrInvalidAmount},
		{text: "15O", want: ErrInvalidAmount},
		{text: "-5", want: ErrInvalidAmount},
		{text: "+5", want: ErrInvalidAmount},
		{text: "1e3", want: ErrInvalidAmount},
		{text: ",50", want: ErrInvalidAmount},
		{text: "150,", want: ErrInvalidAmount},
		{text: "150,505", want: ErrInvalidAmount},
		{text: "1,500.00", want: ErrInvalidAmount},
		{text: "150,5a", want: ErrInvalidAmount},
		{text: "1.5.0", want: ErrInvalidAmount},
		{text: "100000", want: ErrAmountTooLarge},
		{text: "99999,991", want: ErrInvalidAmount},
		{text: "100 000,00", want: ErrAmountTooLarge},
		{text: "999999999999999999999", want: ErrAmountTooLarge},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := ParseGrosze(tt.text)
			if !errors.Is(err, tt.want) {
				t.Errorf("ParseGrosze(%q) = %d, %v, want error %v", tt.text, got, err, tt.want)
			}
		})
	}
}

func TestAmountMessage(t *testing.T) {
	if got := AmountMessage(ErrInvalidAmount); got != InvalidAmountMessage {
		t.Errorf("AmountMessage(invalid) = %q, want %q", got, InvalidAmountMessage)
	}
	if got := AmountMessage(ErrAmountTooLarge); got != AmountTooLargeMessage {
		t.Errorf("AmountMessage(too large) = %q, want %q", got, AmountTooLargeMessage)
	}
}

func TestGroszeString(t *testing.T) {
	tests := []struct {
		amount Grosze
		want   string
	}{
		{amount: 0, want: "0 zł"},
		{amount: 5, want: "0,05 zł"},
		{amount: 15000, want: "150 zł"},
		{amount: 15050, want: "150,50 zł"},
		{amount: 15005, want: "150,05 zł"},
		{amount: 100000, want: "1 000 zł"},
		{amount: 190000, want: "1 900 zł"},
		{amount: 8295000, want: "82 950 zł"},
		{amount: MaxAmount, want: "99 999,99 zł"},
		{amount: 123456789, want: "1 234 567,89 zł"},
		{amount: -15050, want: "-150,50 zł"},
	}

	for _, tt := range tests {
		if got := tt.amount.String(); got != tt.want {
			t.Errorf("Grosze(%d).String() = %q, want %q", tt.amount, got, tt.want)
		}
	}
}

func TestGroszeFormValue(t *testing.T) {
	tests := []struct {
		amount Grosze
		want   string
	}{
		{amount: 0, want: ""},
		{amount: 15000, want: "150"},
		{amount: 15050, want: "150,50"},
		{amount: 15005, want: "150,05"},
		{amount: 150000, want: "1500"},
	}

	for _, tt := range tests {
		got := tt.amount.FormValue()
		if got != tt.want {
			t.Errorf("Grosze(%d).FormValue() = %q, want %q", tt.amount, got, tt.want)
		}
		if parsed, err := ParseGrosze(got); err != nil || parsed != tt.amount {
			t.Errorf("ParseGrosze(%q) = %d, %v, want the round trip to give %d", got, parsed, err, tt.amount)
		}
	}
}
