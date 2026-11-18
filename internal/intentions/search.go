package intentions

import (
	"cmp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"gorm.io/gorm"
)

const MinSearchLength = 3

var polishBase = map[rune]rune{
	'ą': 'a', 'ć': 'c', 'ę': 'e', 'ł': 'l', 'ń': 'n', 'ó': 'o', 'ś': 's', 'ź': 'z', 'ż': 'z',
}

var polishVariants = map[rune]string{
	'a': "ąĄ", 'c': "ćĆ", 'e': "ęĘ", 'l': "łŁ", 'n': "ńŃ", 'o': "óÓ", 's': "śŚ", 'z': "źŹżŻ",
}

func Fold(text string) string {
	folded, _ := foldRunes(text)
	return string(folded)
}

func SearchReady(query string) bool {
	return utf8.RuneCountInString(Fold(query)) >= MinSearchLength
}

func foldRunes(text string) ([]rune, []int) {
	runes := make([]rune, 0, len(text))
	offsets := make([]int, 0, len(text))
	space := true
	for i, r := range text {
		if unicode.IsSpace(r) {
			if !space {
				runes = append(runes, ' ')
				offsets = append(offsets, i)
			}
			space = true
			continue
		}
		space = false
		r = unicode.ToLower(r)
		if base, ok := polishBase[r]; ok {
			r = base
		}
		runes = append(runes, r)
		offsets = append(offsets, i)
	}
	if n := len(runes); n > 0 && runes[n-1] == ' ' {
		runes, offsets = runes[:n-1], offsets[:n-1]
	}

	return runes, offsets
}

func Find(content, query string) [][2]int {
	needle := []rune(Fold(query))
	if len(needle) == 0 {
		return nil
	}

	hay, offsets := foldRunes(content)
	var found [][2]int
	for i := 0; i+len(needle) <= len(hay); i++ {
		if !equalRunes(hay[i:i+len(needle)], needle) {
			continue
		}
		last := offsets[i+len(needle)-1]
		_, size := utf8.DecodeRuneInString(content[last:])
		found = append(found, [2]int{offsets[i], last + size})
		i += len(needle) - 1
	}

	return found
}

func equalRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func globPattern(folded string) string {
	var pattern strings.Builder
	pattern.WriteByte('*')
	for _, r := range folded {
		switch {
		case r == ' ':
			pattern.WriteByte('*')
		case strings.ContainsRune("*?[]^", r):
			pattern.WriteByte('?')
		default:
			pattern.WriteByte('[')
			pattern.WriteRune(r)
			if upper := unicode.ToUpper(r); upper != r {
				pattern.WriteRune(upper)
			}
			pattern.WriteString(polishVariants[r])
			pattern.WriteByte(']')
		}
	}
	pattern.WriteByte('*')

	return pattern.String()
}

func Search(db *gorm.DB, query string) ([]Intention, error) {
	folded := Fold(query)
	if utf8.RuneCountInString(folded) < MinSearchLength {
		return nil, nil
	}

	var candidates []Intention
	err := db.Where("is_placeholder = ? AND content GLOB ?", false, globPattern(folded)).
		Find(&candidates).Error
	if err != nil {
		return nil, err
	}

	list := candidates[:0]
	for _, intention := range candidates {
		if strings.Contains(Fold(intention.Content), folded) {
			list = append(list, intention)
		}
	}
	slices.SortFunc(list, func(a, b Intention) int {
		return cmp.Or(a.MassAt.Compare(b.MassAt), cmp.Compare(a.ID, b.ID))
	})

	return list, nil
}
