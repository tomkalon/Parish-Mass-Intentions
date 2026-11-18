package intentions

import (
	"cmp"
	"slices"
	"strings"
	"time"
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

func foldRune(r rune) rune {
	r = unicode.ToLower(r)
	if base, ok := polishBase[r]; ok {
		return base
	}
	return r
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
		runes = append(runes, foldRune(r))
		offsets = append(offsets, i)
	}
	if n := len(runes); n > 0 && runes[n-1] == ' ' {
		runes, offsets = runes[:n-1], offsets[:n-1]
	}

	return runes, offsets
}

func foldInto(dst []rune, text string) []rune {
	space := true
	for _, r := range text {
		if unicode.IsSpace(r) {
			if !space {
				dst = append(dst, ' ')
			}
			space = true
			continue
		}
		space = false
		dst = append(dst, foldRune(r))
	}
	if n := len(dst); n > 0 && dst[n-1] == ' ' {
		dst = dst[:n-1]
	}

	return dst
}

func containsRunes(hay, needle []rune) bool {
	for i := 0; i+len(needle) <= len(hay); i++ {
		if equalRunes(hay[i:i+len(needle)], needle) {
			return true
		}
	}
	return false
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

type Hit struct {
	ID     int
	MassAt time.Time
}

func Search(db *gorm.DB, query string) ([]Hit, error) {
	needle := []rune(Fold(query))
	if len(needle) < MinSearchLength {
		return nil, nil
	}

	rows, err := db.Model(&Intention{}).
		Select("id", "mass_at", "content").
		Where("is_placeholder = ? AND content GLOB ?", false, globPattern(string(needle))).
		Rows()
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var hits []Hit
	var folded []rune
	for rows.Next() {
		var hit Hit
		var content string
		err := rows.Scan(&hit.ID, &hit.MassAt, &content)
		if err != nil {
			return nil, err
		}
		folded = foldInto(folded[:0], content)
		if containsRunes(folded, needle) {
			hits = append(hits, hit)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	slices.SortFunc(hits, func(a, b Hit) int {
		return cmp.Or(a.MassAt.Compare(b.MassAt), cmp.Compare(a.ID, b.ID))
	})

	return hits, nil
}

func ByIDs(db *gorm.DB, ids []int) (map[int]Intention, error) {
	result := make(map[int]Intention, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var list []Intention
	err := db.Where("id IN ?", ids).Find(&list).Error
	if err != nil {
		return nil, err
	}
	for _, intention := range list {
		result[intention.ID] = intention
	}

	return result, nil
}
