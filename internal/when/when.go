// Package when parses the two ways an admin can express a schedule time: a
// relative duration ("in 2h30m") or an absolute date-time ("at 2026-07-20
// 20:00" in a timezone). It is pure and unit-tested; the bot layer turns the
// results into a fire time.
package when

import (
	"fmt"
	"strings"
	"time"
)

// MaxHorizon caps how far in the future a game may be scheduled.
const MaxHorizon = 60 * 24 * time.Hour // 60 days

// ParseDuration parses a relative duration with day support, e.g. "2h", "90m",
// "1h30m", "1d", "1d6h". It extends time.ParseDuration, which does not know "d".
func ParseDuration(s string) (time.Duration, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty duration")
	}
	var days time.Duration
	if i := strings.IndexByte(s, 'd'); i >= 0 {
		var n int
		if _, err := fmt.Sscanf(s[:i], "%d", &n); err != nil || n < 0 {
			return 0, fmt.Errorf("invalid day count in %q", s)
		}
		days = time.Duration(n) * 24 * time.Hour
		s = s[i+1:]
	}
	var rest time.Duration
	if s != "" {
		var err error
		rest, err = time.ParseDuration(s)
		if err != nil {
			return 0, fmt.Errorf("invalid duration %q: %w", s, err)
		}
	}
	total := days + rest
	if total <= 0 {
		return 0, fmt.Errorf("duration must be positive")
	}
	return total, nil
}

// acceptedLayouts are the absolute date-time formats accepted, most specific
// first.
var acceptedLayouts = []string{
	"2006-01-02 15:04",
	"2006-01-02T15:04",
	"2006-01-02 15:04:05",
	"02.01.2006 15:04", // day-first, common in Europe
	"2006-01-02 15",
}

// ParseAt parses an absolute date-time in the given location. tz is an IANA name
// ("Europe/Berlin") or empty for UTC.
func ParseAt(s, tz string) (time.Time, error) {
	loc := time.UTC
	if tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil {
			return time.Time{}, fmt.Errorf("unknown timezone %q", tz)
		}
		loc = l
	}
	s = strings.TrimSpace(s)
	for _, layout := range acceptedLayouts {
		if t, err := time.ParseInLocation(layout, s, loc); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("could not parse date-time %q (try 2006-01-02 15:04)", s)
}

// Resolve turns an optional duration and optional absolute time into a fire
// time, validating that exactly one was given and it is in the future within the
// allowed horizon. now is injected for testability.
func Resolve(now time.Time, in, at, tz string) (time.Time, error) {
	in = strings.TrimSpace(in)
	at = strings.TrimSpace(at)
	switch {
	case in != "" && at != "":
		return time.Time{}, fmt.Errorf("give either a duration or a date-time, not both")
	case in != "":
		d, err := ParseDuration(in)
		if err != nil {
			return time.Time{}, err
		}
		return validateFuture(now, now.Add(d))
	case at != "":
		t, err := ParseAt(at, tz)
		if err != nil {
			return time.Time{}, err
		}
		return validateFuture(now, t)
	default:
		return time.Time{}, fmt.Errorf("provide a time: either `in` (e.g. 2h30m) or `at` (e.g. 2026-07-20 20:00)")
	}
}

func validateFuture(now, t time.Time) (time.Time, error) {
	if !t.After(now) {
		return time.Time{}, fmt.Errorf("that time is in the past")
	}
	if t.Sub(now) > MaxHorizon {
		return time.Time{}, fmt.Errorf("that time is more than %d days away", int(MaxHorizon.Hours()/24))
	}
	return t, nil
}
