package masses

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode"
)

var weekdays = map[string]time.Weekday{
	"poniedziałek": time.Monday,
	"wtorek":       time.Tuesday,
	"środa":        time.Wednesday,
	"czwartek":     time.Thursday,
	"piątek":       time.Friday,
	"sobota":       time.Saturday,
	"niedziela":    time.Sunday,
}

//go:embed schedules.json
var defaultFile []byte

type scheduleFile struct {
	Name   string              `json:"name"`
	Masses map[string][]string `json:"masses"`
}

type Schedule struct {
	ID     int
	Name   string
	Masses [7][]string
}

func Default() ([]Schedule, error) {
	return ParseSchedules(defaultFile)
}

func ParseSchedules(data []byte) ([]Schedule, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()

	var files []scheduleFile
	err := decoder.Decode(&files)
	if err != nil {
		return nil, fmt.Errorf("decoding schedules: %w", err)
	}
	if len(files) == 0 {
		return nil, errors.New("no schedules defined")
	}

	schedules := make([]Schedule, 0, len(files))
	seen := make(map[string]bool)
	for _, file := range files {
		if file.Name == "" {
			return nil, errors.New("schedule without a name")
		}
		if seen[file.Name] {
			return nil, fmt.Errorf("duplicate schedule %q", file.Name)
		}
		seen[file.Name] = true

		schedule, err := parseSchedule(file)
		if err != nil {
			return nil, fmt.Errorf("schedule %q: %w", file.Name, err)
		}
		schedules = append(schedules, schedule)
	}

	return schedules, nil
}

func parseSchedule(file scheduleFile) (Schedule, error) {
	schedule := Schedule{Name: file.Name}

	for _, name := range slices.Sorted(maps.Keys(file.Masses)) {
		day, ok := weekdays[name]
		if !ok {
			return Schedule{}, fmt.Errorf("unknown day %q", name)
		}

		times, err := parseTimes(file.Masses[name])
		if err != nil {
			return Schedule{}, fmt.Errorf("%s: %w", name, err)
		}
		schedule.Masses[day] = times
	}

	for _, name := range slices.Sorted(maps.Keys(weekdays)) {
		if _, ok := file.Masses[name]; !ok {
			return Schedule{}, fmt.Errorf("missing day %q", name)
		}
	}

	return schedule, nil
}

var (
	ErrInvalidTime    = errors.New("invalid time")
	ErrNotQuarterHour = errors.New("not a full quarter hour")
	ErrDuplicateTime  = errors.New("duplicate time")
)

type TimeError struct {
	Value  string
	Reason error
}

func (e *TimeError) Error() string {
	return fmt.Sprintf("%v %q", e.Reason, e.Value)
}

func (e *TimeError) Unwrap() error {
	return e.Reason
}

func ParseDay(text string) ([]string, error) {
	values := strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == ';' || unicode.IsSpace(r)
	})

	return parseTimes(values)
}

func parseTimes(values []string) ([]string, error) {
	times := make([]string, 0, len(values))
	for _, value := range values {
		t, err := time.Parse("15:04", value)
		if err != nil {
			return nil, &TimeError{Value: value, Reason: ErrInvalidTime}
		}
		if t.Minute()%15 != 0 {
			return nil, &TimeError{Value: value, Reason: ErrNotQuarterHour}
		}

		normalized := t.Format("15:04")
		if slices.Contains(times, normalized) {
			return nil, &TimeError{Value: value, Reason: ErrDuplicateTime}
		}
		times = append(times, normalized)
	}
	slices.Sort(times)

	return times, nil
}
