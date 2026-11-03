package intentions

import (
	"errors"
	"strconv"
	"strings"
)

type Grosze int64

const MaxAmount Grosze = 99_999_99

const (
	InvalidAmountMessage  = "Wpisz kwotę cyframi, np. 100 albo 150,50."
	AmountTooLargeMessage = "Kwota może wynosić najwyżej 99 999,99 zł."
)

var (
	ErrInvalidAmount  = errors.New("invalid amount")
	ErrAmountTooLarge = errors.New("amount too large")
)

func ParseGrosze(text string) (Grosze, error) {
	text = strings.TrimSpace(strings.ToLower(text))
	text = strings.TrimSuffix(text, "zł")
	text = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == ' ' {
			return -1
		}
		return r
	}, text)
	if text == "" {
		return 0, nil
	}

	whole, fraction, hasFraction := strings.Cut(strings.Replace(text, ",", ".", 1), ".")
	if !isDigits(whole) || (hasFraction && (len(fraction) > 2 || !isDigits(fraction))) {
		return 0, ErrInvalidAmount
	}
	if len(whole) > 7 {
		return 0, ErrAmountTooLarge
	}

	zloty, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, ErrInvalidAmount
	}
	grosze := int64(0)
	if hasFraction {
		grosze, err = strconv.ParseInt((fraction + "0")[:2], 10, 64)
		if err != nil {
			return 0, ErrInvalidAmount
		}
	}

	amount := Grosze(zloty*100 + grosze)
	if amount > MaxAmount {
		return 0, ErrAmountTooLarge
	}

	return amount, nil
}

func AmountMessage(err error) string {
	if errors.Is(err, ErrAmountTooLarge) {
		return AmountTooLargeMessage
	}

	return InvalidAmountMessage
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}

	return true
}

func (g Grosze) String() string {
	if g < 0 {
		return "-" + (-g).String()
	}

	zloty := groupThousands(int64(g) / 100)
	if grosze := int64(g) % 100; grosze != 0 {
		return zloty + "," + twoDigits(grosze) + " zł"
	}

	return zloty + " zł"
}

func (g Grosze) FormValue() string {
	if g == 0 {
		return ""
	}

	zloty := strconv.FormatInt(int64(g)/100, 10)
	if grosze := int64(g) % 100; grosze != 0 {
		return zloty + "," + twoDigits(grosze)
	}

	return zloty
}

func groupThousands(n int64) string {
	digits := strconv.FormatInt(n, 10)
	if len(digits) <= 3 {
		return digits
	}

	var b strings.Builder
	head := len(digits) % 3
	if head > 0 {
		b.WriteString(digits[:head])
	}
	for i := head; i < len(digits); i += 3 {
		if b.Len() > 0 {
			b.WriteRune(' ')
		}
		b.WriteString(digits[i : i+3])
	}

	return b.String()
}

func twoDigits(n int64) string {
	if n < 10 {
		return "0" + strconv.FormatInt(n, 10)
	}

	return strconv.FormatInt(n, 10)
}
