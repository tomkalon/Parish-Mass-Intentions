package web

import (
	"fmt"

	"go-parafia/internal/intentions"
)

type clearView struct {
	Count        string
	Placeholders int
}

func newClearView(list []intentions.Intention) clearView {
	view := clearView{Count: intentionsCount(len(list))}
	for _, intention := range list {
		if intention.IsPlaceholder {
			view.Placeholders++
		}
	}

	return view
}

func intentionsCount(n int) string {
	switch {
	case n == 1:
		return "1 intencja"
	case n%10 >= 2 && n%10 <= 4 && (n%100 < 12 || n%100 > 14):
		return fmt.Sprintf("%d intencje", n)
	default:
		return fmt.Sprintf("%d intencji", n)
	}
}
