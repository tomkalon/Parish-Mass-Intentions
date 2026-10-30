package gcatholic

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	BaseURL     = "https://gcatholic.org"
	maxFileSize = 4 << 20
)

type Celebration struct {
	Date string
	Rank string
	Name string
}

type Kind int

const (
	Other Kind = iota
	Weekday
	Solemnity
	Feast
	Memorial
	OptionalMemorial
)

var rankKinds = map[string]Kind{
	"U":  Solemnity,
	"Ś":  Feast,
	"W":  Memorial,
	"w":  OptionalMemorial,
	"w*": OptionalMemorial,
}

var weekdayPrefixes = []string{"Poniedziałek ", "Wtorek ", "Środa ", "Czwartek ", "Piątek ", "Sobota "}

func (c Celebration) Kind() Kind {
	if kind, ok := rankKinds[c.Rank]; ok {
		return kind
	}
	if isWeekday(c.Name) {
		return Weekday
	}

	return Other
}

func isWeekday(name string) bool {
	if strings.HasPrefix(name, "Dzień adwentu") {
		return true
	}

	startsWithDay := slices.ContainsFunc(weekdayPrefixes, func(prefix string) bool {
		return strings.HasPrefix(name, prefix)
	})
	if !startsWithDay {
		return false
	}

	lower := strings.ToLower(name)

	return strings.Contains(lower, " tygodnia ") || strings.Contains(lower, " przed ") || strings.Contains(lower, " po ")
}

var rankWords = map[string]string{
	"U":  "uroczystość",
	"Ś":  "święto",
	"W":  "wspomnienie",
	"w":  "wspomnienie dowolne",
	"w*": "wspomnienie dowolne",
}

var unescape = strings.NewReplacer(`\\`, `\`, `\,`, ",", `\;`, ";", `\n`, " ", `\N`, " ")

func Fetch(ctx context.Context, client *http.Client, baseURL string, year int) ([]Celebration, error) {
	url := fmt.Sprintf("%s/calendar/ics/%d-pl-PL.ics?v=3", baseURL, year)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Go Parafia")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetching %s: unexpected status %s", url, resp.Status)
	}

	return Parse(io.LimitReader(resp.Body, maxFileSize))
}

func Parse(r io.Reader) ([]Celebration, error) {
	lines, err := unfold(r)
	if err != nil {
		return nil, fmt.Errorf("reading calendar: %w", err)
	}

	var celebrations []Celebration
	var current *Celebration
	for _, line := range lines {
		switch line {
		case "BEGIN:VEVENT":
			current = &Celebration{}
			continue
		case "END:VEVENT":
			if current != nil && current.Date != "" && current.Name != "" {
				celebrations = append(celebrations, *current)
			}
			current = nil
			continue
		}
		if current == nil {
			continue
		}

		property, value, _ := strings.Cut(line, ":")
		property, _, _ = strings.Cut(property, ";")

		switch property {
		case "DTSTART":
			day, err := time.Parse("20060102", value)
			if err != nil {
				return nil, fmt.Errorf("invalid DTSTART %q: %w", value, err)
			}
			current.Date = day.Format("2006-01-02")
		case "SUMMARY":
			current.Rank, current.Name = parseSummary(unescape.Replace(value))
		}
	}

	return celebrations, nil
}

func unfold(r io.Reader) ([]string, error) {
	var lines []string
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if len(lines) > 0 && (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) {
			lines[len(lines)-1] += line[1:]
			continue
		}
		lines = append(lines, line)
	}

	return lines, scanner.Err()
}

func parseSummary(summary string) (rank, name string) {
	if first, rest, ok := strings.Cut(summary, " "); ok && !startsWithLetter(first) {
		summary = rest
	}

	if rest, ok := strings.CutPrefix(summary, "["); ok {
		if rank, name, ok := strings.Cut(rest, "] "); ok {
			return rank, name
		}
	}

	return "", summary
}

func startsWithLetter(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)

	return unicode.IsLetter(r) || unicode.IsDigit(r)
}

func (c Celebration) Text() string {
	word, ok := rankWords[c.Rank]
	if !ok {
		return c.Name
	}

	name := c.Name
	if rest, ok := strings.CutPrefix(name, "Św. "); ok {
		name = "św. " + rest
	}

	return word + " " + name
}

func Texts(celebrations []Celebration) map[string]string {
	texts := make(map[string]string)
	for _, c := range celebrations {
		if texts[c.Date] != "" {
			texts[c.Date] += "; "
		}
		texts[c.Date] += c.Text()
	}

	return texts
}
