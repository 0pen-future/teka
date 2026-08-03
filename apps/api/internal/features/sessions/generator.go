package sessions

import (
	"sort"
	"time"
)

// ClassWindow is the slice of a class the pure date-expansion algorithm
// clamps against: no session before the class starts, none after it ends
// (when it has an end date).
type ClassWindow struct {
	StartDate time.Time
	EndDate   *time.Time
}

// ScheduleWindow is the slice of a class_schedules row the algorithm needs:
// which weekday, at what time, effective for which date range.
type ScheduleWindow struct {
	Weekday       int16
	StartTime     string
	EffectiveFrom time.Time
	EffectiveTo   *time.Time
}

// dateOnly strips the time-of-day and re-expresses t at midnight in loc,
// keeping only its calendar year/month/day. Y/M/D are read off t directly
// regardless of t's own location, so a value already produced by dateOnly in
// a different zone still round-trips correctly.
func dateOnly(t time.Time, loc *time.Location) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, loc)
}

// latestOf returns whichever date-only value is later.
func latestOf(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// earliestOf returns whichever date-only value is earlier.
func earliestOf(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Expand computes every calendar date in [from, to] that a session must
// exist for, given a class's window and its (already effective-range
// filtered) schedules. It performs no database access: callers resolve the
// class and its schedules first and pass in plain values, which is what
// keeps the boundary cases — mid-range schedule changes, an end_date inside
// the range, weekday 0 (Sunday) — cheap to table-test without a database.
//
// Iteration walks day by day with AddDate(0, 0, 1), never a fixed 24-hour
// duration, and every boundary is normalised to midnight in loc (the
// teacher's timezone) before comparison — the pair of choices that keeps
// this correct under a DST transition, even though Vietnam currently has
// none. The returned dates are re-expressed at UTC midnight, matching how
// every other DATE column in this codebase is represented; pgx encodes a
// DATE from a time.Time's Year/Month/Day fields directly, never through an
// actual UTC clock conversion, so this changes representation only, not the
// calendar date itself.
func Expand(class ClassWindow, schedules []ScheduleWindow, from, to time.Time, loc *time.Location) []time.Time {
	if loc == nil {
		loc = time.UTC
	}

	rangeLo := dateOnly(from, loc)
	rangeHi := dateOnly(to, loc)
	if rangeHi.Before(rangeLo) {
		return nil
	}

	windowLo := latestOf(rangeLo, dateOnly(class.StartDate, loc))
	windowHi := rangeHi
	if class.EndDate != nil {
		windowHi = earliestOf(windowHi, dateOnly(*class.EndDate, loc))
	}
	if windowHi.Before(windowLo) {
		return nil
	}

	seen := make(map[time.Time]bool)
	var out []time.Time
	for _, s := range schedules {
		lo := latestOf(windowLo, dateOnly(s.EffectiveFrom, loc))
		hi := windowHi
		if s.EffectiveTo != nil {
			hi = earliestOf(hi, dateOnly(*s.EffectiveTo, loc))
		}
		if hi.Before(lo) {
			continue
		}
		for d := lo; !d.After(hi); d = d.AddDate(0, 0, 1) {
			if int(d.Weekday()) != int(s.Weekday) {
				continue
			}
			key := dateOnly(d, time.UTC)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, key)
		}
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Before(out[j]) })
	return out
}

// ScheduleFor returns the schedule matching date's weekday and covering it
// within [effective_from, effective_to] — used after Expand to attach the
// right start_time to each generated date. Effective ranges for the same
// weekday are expected not to overlap (schedule replacement closes the old
// row before the new one opens), so the first match wins.
func ScheduleFor(schedules []ScheduleWindow, date time.Time) (ScheduleWindow, bool) {
	d := dateOnly(date, time.UTC)
	for _, s := range schedules {
		if int(d.Weekday()) != int(s.Weekday) {
			continue
		}
		lo := dateOnly(s.EffectiveFrom, time.UTC)
		if d.Before(lo) {
			continue
		}
		if s.EffectiveTo != nil {
			hi := dateOnly(*s.EffectiveTo, time.UTC)
			if d.After(hi) {
				continue
			}
		}
		return s, true
	}
	return ScheduleWindow{}, false
}
