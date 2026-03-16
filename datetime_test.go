package luca

import (
	"testing"
	"time"
)

func TestDateTimeString(t *testing.T) {
	tests := []struct {
		name string
		dt   DateTime
		want string
	}{
		{"date only", DateTime{Date: "2026-02-07"}, "2026-02-07"},
		{"year only", DateTime{Date: "2024"}, "2024"},
		{"year-month", DateTime{Date: "2024-01"}, "2024-01"},
		{"full UTC", DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "Z"}, "2026-02-07T14:30:00Z"},
		{"tz offset", DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "+01:00"}, "2026-02-07T14:30:00+01:00"},
		{"negative tz", DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "-05:30"}, "2026-02-07T14:30:00-05:30"},
		{"milliseconds", DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123", Timezone: "Z"}, "2026-02-07T14:30:00.123Z"},
		{"microseconds", DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456", Timezone: "+05:30"}, "2026-02-07T14:30:00.123456+05:30"},
		{"nanoseconds", DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456789", Timezone: "Z"}, "2026-02-07T14:30:00.123456789Z"},
		{"period end year", DateTime{Date: "2024", PeriodAnchor: "$"}, "2024$"},
		{"period end month", DateTime{Date: "2024-01", PeriodAnchor: "$"}, "2024-01$"},
		{"period start", DateTime{Date: "2024-01-15", PeriodAnchor: "^"}, "2024-01-15^"},
		{"period end day", DateTime{Date: "2024-01-31", PeriodAnchor: "$"}, "2024-01-31$"},
		{"period end with time", DateTime{Date: "2024-01-15", Time: "23:59:59", PeriodAnchor: "$"}, "2024-01-15T23:59:59$"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.dt.String()
			if got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDateTimeGranularity(t *testing.T) {
	tests := []struct {
		date string
		want string
	}{
		{"2024", "year"},
		{"2024-01", "month"},
		{"2024-01-15", "day"},
	}
	for _, tt := range tests {
		got := DateTime{Date: tt.date}.DateGranularity()
		if got != tt.want {
			t.Errorf("DateGranularity(%q) = %q, want %q", tt.date, got, tt.want)
		}
	}
}

func TestDateTimeIsDateOnly(t *testing.T) {
	dateOnly := DateTime{Date: "2024-01-15"}
	if !dateOnly.IsDateOnly() {
		t.Error("date-only should return true")
	}
	withTime := DateTime{Date: "2024-01-15", Time: "14:30:00"}
	if withTime.IsDateOnly() {
		t.Error("with time should return false")
	}
}

func TestDateTimeToTime(t *testing.T) {
	tests := []struct {
		name string
		dt   DateTime
		want time.Time
	}{
		{
			"date only",
			DateTime{Date: "2026-02-07"},
			time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC),
		},
		{
			"full UTC",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "Z"},
			time.Date(2026, 2, 7, 14, 30, 0, 0, time.UTC),
		},
		{
			"nanoseconds",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456789", Timezone: "Z"},
			time.Date(2026, 2, 7, 14, 30, 0, 123456789, time.UTC),
		},
		{
			"milliseconds",
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123", Timezone: "Z"},
			time.Date(2026, 2, 7, 14, 30, 0, 123000000, time.UTC),
		},
		{
			"year only",
			DateTime{Date: "2024"},
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		{
			"year-month",
			DateTime{Date: "2024-06"},
			time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dt.ToTime()
			if err != nil {
				t.Fatalf("ToTime: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ToTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateTimeToTimeTimezones(t *testing.T) {
	dt := DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "+01:00"}
	got, err := dt.ToTime()
	if err != nil {
		t.Fatalf("ToTime: %v", err)
	}
	_, offset := got.Zone()
	if offset != 3600 {
		t.Errorf("timezone offset = %d, want 3600", offset)
	}
	if got.Hour() != 14 || got.Minute() != 30 {
		t.Errorf("time = %02d:%02d, want 14:30", got.Hour(), got.Minute())
	}

	dt2 := DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "-05:30"}
	got2, err := dt2.ToTime()
	if err != nil {
		t.Fatalf("ToTime: %v", err)
	}
	_, offset2 := got2.Zone()
	if offset2 != -(5*3600+30*60) {
		t.Errorf("timezone offset = %d, want %d", offset2, -(5*3600 + 30*60))
	}
}

func TestDateTimeToTimePeriodAnchors(t *testing.T) {
	tests := []struct {
		name string
		dt   DateTime
		want time.Time
	}{
		{
			"year-end",
			DateTime{Date: "2024", PeriodAnchor: "$"},
			time.Date(2024, 12, 31, 23, 59, 59, 999999999, time.UTC),
		},
		{
			"month-end jan",
			DateTime{Date: "2024-01", PeriodAnchor: "$"},
			time.Date(2024, 1, 31, 23, 59, 59, 999999999, time.UTC),
		},
		{
			"month-end feb leap",
			DateTime{Date: "2024-02", PeriodAnchor: "$"},
			time.Date(2024, 2, 29, 23, 59, 59, 999999999, time.UTC),
		},
		{
			"month-end feb non-leap",
			DateTime{Date: "2025-02", PeriodAnchor: "$"},
			time.Date(2025, 2, 28, 23, 59, 59, 999999999, time.UTC),
		},
		{
			"day-end",
			DateTime{Date: "2024-01-15", PeriodAnchor: "$"},
			time.Date(2024, 1, 15, 23, 59, 59, 999999999, time.UTC),
		},
		{
			"day-start",
			DateTime{Date: "2024-01-15", PeriodAnchor: "^"},
			time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
		},
		{
			"day-end with time preserves time",
			DateTime{Date: "2024-01-15", Time: "23:59:59", PeriodAnchor: "$"},
			// When time is specified, anchor doesn't override it
			time.Date(2024, 1, 15, 23, 59, 59, 0, time.UTC),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.dt.ToTime()
			if err != nil {
				t.Fatalf("ToTime: %v", err)
			}
			if !got.Equal(tt.want) {
				t.Errorf("ToTime() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDateTimeFromTime(t *testing.T) {
	tests := []struct {
		name string
		t    time.Time
		want DateTime
	}{
		{
			"midnight UTC",
			time.Date(2026, 2, 7, 0, 0, 0, 0, time.UTC),
			DateTime{Date: "2026-02-07"},
		},
		{
			"with time",
			time.Date(2026, 2, 7, 14, 30, 0, 0, time.UTC),
			DateTime{Date: "2026-02-07", Time: "14:30:00", Timezone: "Z"},
		},
		{
			"with nanoseconds",
			time.Date(2026, 2, 7, 14, 30, 0, 123456789, time.UTC),
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456789", Timezone: "Z"},
		},
		{
			"strips trailing zero fractions",
			time.Date(2026, 2, 7, 14, 30, 0, 123000000, time.UTC),
			DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123", Timezone: "Z"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DateTimeFromTime(tt.t)
			if got != tt.want {
				t.Errorf("DateTimeFromTime() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestDateTimeRoundTrip(t *testing.T) {
	// DateTime → ToTime → DateTimeFromTime should preserve what it can.
	// Period anchors and date granularity are lost.
	dt := DateTime{Date: "2026-02-07", Time: "14:30:00", Fractional: ".123456789", Timezone: "Z"}
	tt, err := dt.ToTime()
	if err != nil {
		t.Fatalf("ToTime: %v", err)
	}
	got := DateTimeFromTime(tt)
	if got.Date != dt.Date {
		t.Errorf("Date = %q, want %q", got.Date, dt.Date)
	}
	if got.Time != dt.Time {
		t.Errorf("Time = %q, want %q", got.Time, dt.Time)
	}
	if got.Fractional != dt.Fractional {
		t.Errorf("Fractional = %q, want %q", got.Fractional, dt.Fractional)
	}
	if got.Timezone != dt.Timezone {
		t.Errorf("Timezone = %q, want %q", got.Timezone, dt.Timezone)
	}
}
