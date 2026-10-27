package utils

import "time"

// LocalDay is t's calendar day in loc, stamped as UTC midnight; a nil loc means UTC.
// e.g. Jan 31 18:30 UTC in Asia/Kolkata → Feb 1 00:00 UTC.
func LocalDay(t time.Time, loc *time.Location) time.Time {
	if loc == nil {
		loc = time.UTC
	}
	y, m, d := t.In(loc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
