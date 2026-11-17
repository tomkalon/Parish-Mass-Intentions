package web

import "strconv"

func countOf(n int, one, few, many string) string {
	return strconv.Itoa(n) + " " + pluralForm(n, one, few, many)
}

func pluralForm(n int, one, few, many string) string {
	switch {
	case n == 1:
		return one
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return few
	default:
		return many
	}
}
