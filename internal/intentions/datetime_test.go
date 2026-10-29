package intentions

import (
	"testing"
	"time"
)

func TestWeekStart(t *testing.T) {
	tests := []struct {
		name string
		day  time.Time
		want time.Time
	}{
		{
			name: "wednesday",
			day:  time.Date(2026, 10, 21, 15, 0, 0, 0, TimeZone),
			want: time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "monday midnight is its own week start",
			day:  time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone),
			want: time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "sunday after the DST change belongs to the same week",
			day:  time.Date(2026, 10, 25, 18, 0, 0, 0, TimeZone),
			want: time.Date(2026, 10, 19, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "next monday starts a new week",
			day:  time.Date(2026, 10, 26, 6, 30, 0, 0, TimeZone),
			want: time.Date(2026, 10, 26, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "sunday belongs to the previous week",
			day:  time.Date(2026, 10, 18, 7, 0, 0, 0, TimeZone),
			want: time.Date(2026, 10, 12, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "across the year boundary",
			day:  time.Date(2027, 1, 1, 12, 0, 0, 0, TimeZone),
			want: time.Date(2026, 12, 28, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "UTC late sunday evening is already monday in Poland",
			day:  time.Date(2026, 10, 25, 23, 30, 0, 0, time.UTC),
			want: time.Date(2026, 10, 26, 0, 0, 0, 0, TimeZone),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := WeekStart(tt.day)

			if !got.Equal(tt.want) {
				t.Errorf("WeekStart(%v) = %v, want %v", tt.day, got, tt.want)
			}
		})
	}
}

func TestNextWeekStart(t *testing.T) {
	tests := []struct {
		name string
		day  time.Time
		want time.Time
	}{
		{
			name: "sunday gives the next day",
			day:  time.Date(2026, 11, 1, 18, 0, 0, 0, TimeZone),
			want: time.Date(2026, 11, 2, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "monday gives the monday a week later",
			day:  time.Date(2026, 11, 2, 6, 30, 0, 0, TimeZone),
			want: time.Date(2026, 11, 9, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "wednesday",
			day:  time.Date(2026, 11, 4, 12, 0, 0, 0, TimeZone),
			want: time.Date(2026, 11, 9, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "sunday of the DST change",
			day:  time.Date(2026, 10, 25, 18, 0, 0, 0, TimeZone),
			want: time.Date(2026, 10, 26, 0, 0, 0, 0, TimeZone),
		},
		{
			name: "UTC late sunday evening is already monday in Poland",
			day:  time.Date(2026, 11, 1, 23, 30, 0, 0, time.UTC),
			want: time.Date(2026, 11, 9, 0, 0, 0, 0, TimeZone),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextWeekStart(tt.day)

			if !got.Equal(tt.want) {
				t.Errorf("NextWeekStart(%v) = %v, want %v", tt.day, got, tt.want)
			}
		})
	}
}
