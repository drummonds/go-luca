package luca

import (
	"fmt"
	"strings"
	"time"
)

// DateTime represents a goluca datetime preserving text-format precision.
// It can represent dates at different granularities (year, year-month,
// year-month-day) with optional time, fractional seconds, timezone,
// and period anchors.
type DateTime struct {
	Date         string // "2024", "2024-01", "2024-01-15"
	Time         string // "14:30:00" or "" if date-only
	Fractional   string // ".123", ".123456", ".123456789" or ""
	Timezone     string // "Z", "+01:00", "-05:30" or ""
	PeriodAnchor string // "^", "$", or ""
}

// String returns the goluca text representation.
func (dt DateTime) String() string {
	var sb strings.Builder
	sb.WriteString(dt.Date)
	if dt.Time != "" {
		sb.WriteByte('T')
		sb.WriteString(dt.Time)
		sb.WriteString(dt.Fractional)
		sb.WriteString(dt.Timezone)
	}
	sb.WriteString(dt.PeriodAnchor)
	return sb.String()
}

// IsZero returns true if the DateTime has no date component.
func (dt DateTime) IsZero() bool { return dt.Date == "" }

// IsDateOnly returns true if this datetime has no time component.
func (dt DateTime) IsDateOnly() bool { return dt.Time == "" }

// DateGranularity returns "year", "month", or "day" based on the date format.
func (dt DateTime) DateGranularity() string {
	switch strings.Count(dt.Date, "-") {
	case 0:
		return "year"
	case 1:
		return "month"
	default:
		return "day"
	}
}

// ToTime converts to Go time.Time. This is lossy:
//   - Year-only and year-month dates are expanded to their first day
//   - Period anchor "$" expands to end-of-period (23:59:59.999999999)
//   - Period anchor "^" maps to start-of-period (00:00:00)
//   - Missing timezone is treated as UTC
func (dt DateTime) ToTime() (time.Time, error) {
	year, month, day := 0, 1, 1
	parts := strings.Split(dt.Date, "-")
	if len(parts) >= 1 {
		if _, err := fmt.Sscanf(parts[0], "%d", &year); err != nil {
			return time.Time{}, fmt.Errorf("parse year %q: %w", parts[0], err)
		}
	}
	if len(parts) >= 2 {
		var m int
		if _, err := fmt.Sscanf(parts[1], "%d", &m); err != nil {
			return time.Time{}, fmt.Errorf("parse month %q: %w", parts[1], err)
		}
		month = m
	}
	if len(parts) >= 3 {
		if _, err := fmt.Sscanf(parts[2], "%d", &day); err != nil {
			return time.Time{}, fmt.Errorf("parse day %q: %w", parts[2], err)
		}
	}

	hour, min, sec := 0, 0, 0
	if dt.Time != "" {
		if _, err := fmt.Sscanf(dt.Time, "%d:%d:%d", &hour, &min, &sec); err != nil {
			return time.Time{}, fmt.Errorf("parse time %q: %w", dt.Time, err)
		}
	}

	nsec := 0
	if dt.Fractional != "" {
		frac := dt.Fractional[1:] // strip leading dot
		for len(frac) < 9 {
			frac += "0"
		}
		_, _ = fmt.Sscanf(frac[:9], "%d", &nsec)
	}

	loc := time.UTC
	if dt.Timezone != "" && dt.Timezone != "Z" {
		sign := 1
		if dt.Timezone[0] == '-' {
			sign = -1
		}
		var tzH, tzM int
		_, _ = fmt.Sscanf(dt.Timezone[1:], "%d:%d", &tzH, &tzM)
		offset := sign * (tzH*3600 + tzM*60)
		loc = time.FixedZone(dt.Timezone, offset)
	}

	t := time.Date(year, time.Month(month), day, hour, min, sec, nsec, loc)

	// Apply period anchor
	if dt.PeriodAnchor == "$" && dt.Time == "" {
		switch dt.DateGranularity() {
		case "year":
			t = time.Date(year, 12, 31, 23, 59, 59, 999999999, loc)
		case "month":
			firstOfNext := time.Date(year, time.Month(month)+1, 1, 0, 0, 0, 0, loc)
			lastDay := firstOfNext.AddDate(0, 0, -1)
			t = time.Date(year, time.Month(month), lastDay.Day(), 23, 59, 59, 999999999, loc)
		case "day":
			t = time.Date(year, time.Month(month), day, 23, 59, 59, 999999999, loc)
		}
	} else if dt.PeriodAnchor == "^" && dt.Time == "" {
		t = time.Date(year, time.Month(month), day, 0, 0, 0, 0, loc)
	}

	return t, nil
}

// DateTimeFromTime creates a date-only DateTime from a Go time.Time.
// If the time has a non-zero time component, it is included.
// This is a lossy conversion: period anchors and date granularity are not preserved.
func DateTimeFromTime(t time.Time) DateTime {
	dt := DateTime{
		Date: t.Format("2006-01-02"),
	}
	if t.Hour() != 0 || t.Minute() != 0 || t.Second() != 0 || t.Nanosecond() != 0 {
		dt.Time = t.Format("15:04:05")
		if t.Nanosecond() != 0 {
			ns := fmt.Sprintf(".%09d", t.Nanosecond())
			ns = strings.TrimRight(ns, "0")
			dt.Fractional = ns
		}
		_, offset := t.Zone()
		if offset == 0 {
			dt.Timezone = "Z"
		} else {
			sign := "+"
			if offset < 0 {
				sign = "-"
				offset = -offset
			}
			dt.Timezone = fmt.Sprintf("%s%02d:%02d", sign, offset/3600, (offset%3600)/60)
		}
	}
	return dt
}
