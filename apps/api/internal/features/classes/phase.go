package classes

import "time"

// Phase values: the display state a class is in, derived from status and the
// opening/closing dates rather than stored. This file is the single source
// of truth — PhaseOf for a loaded row, PhasePredicate for the SQL filter and
// stats — and the web only renders the phase the API returns.
const (
	PhaseUpcoming = "upcoming"
	PhaseRunning  = "running"
	PhaseEnded    = "ended"
	PhaseArchived = "archived"
)

// Shift values classify a schedule's start time into the three daily bands
// the class list filters on.
const (
	ShiftMorning   = "morning"
	ShiftAfternoon = "afternoon"
	ShiftEvening   = "evening"
)

// Shift boundaries as wall-clock "HH:MM", exclusive upper bounds.
const (
	afternoonStart TimeOfDay = "12:00"
	eveningStart   TimeOfDay = "17:30"
)

// PhaseOf derives the class's phase on the calendar day of today: archived
// wins outright; otherwise a class is upcoming until its start date, ended
// the day after its end date, and running in between (both dates inclusive).
// Only the date part of today matters — DATE columns carry no clock.
func PhaseOf(c *Class, today time.Time) string {
	if c.Status == StatusArchived {
		return PhaseArchived
	}
	day := dateOnly(today)
	if dateOnly(c.StartDate).After(day) {
		return PhaseUpcoming
	}
	if c.EndDate != nil && dateOnly(*c.EndDate).Before(day) {
		return PhaseEnded
	}
	return PhaseRunning
}

// PhasePredicate returns the SQL fragment and its bound argument selecting
// classes in phase on the given day — the same rules PhaseOf applies in Go,
// expressed once so the list filter and the stats counters cannot drift.
// ok is false for an unknown phase.
func PhasePredicate(phase string, today time.Time) (frag string, args []any, ok bool) {
	day := dateOnly(today)
	switch phase {
	case PhaseArchived:
		return "classes.status = '" + StatusArchived + "'", nil, true
	case PhaseUpcoming:
		return "classes.status <> '" + StatusArchived + "' AND classes.start_date > ?", []any{day}, true
	case PhaseEnded:
		return "classes.status <> '" + StatusArchived + "' AND classes.end_date IS NOT NULL AND classes.end_date < ?", []any{day}, true
	case PhaseRunning:
		return "classes.status <> '" + StatusArchived + "' AND classes.start_date <= ? AND (classes.end_date IS NULL OR classes.end_date >= ?)", []any{day, day}, true
	default:
		return "", nil, false
	}
}

// ShiftOf classifies a start time: before 12:00 is morning, before 17:30 is
// afternoon, anything later is evening. "HH:MM" strings compare correctly
// as plain strings because both halves are zero-padded.
func ShiftOf(start TimeOfDay) string {
	switch {
	case start < afternoonStart:
		return ShiftMorning
	case start < eveningStart:
		return ShiftAfternoon
	default:
		return ShiftEvening
	}
}

// ShiftBounds returns the [from, to) start-time window for a shift, with an
// empty to for the open-ended evening band. ok is false for an unknown shift.
func ShiftBounds(shift string) (from, to TimeOfDay, ok bool) {
	switch shift {
	case ShiftMorning:
		return "00:00", afternoonStart, true
	case ShiftAfternoon:
		return afternoonStart, eveningStart, true
	case ShiftEvening:
		return eveningStart, "", true
	default:
		return "", "", false
	}
}

// Today is the calendar date phases are judged against: the centre's wall
// clock (teachers.DefaultTimezone) rather than the server's UTC clock, so a
// class opening tomorrow does not read as "running" from 17:00 VN onwards.
// It is a date at UTC midnight, comparable with the DATE columns.
func Today() time.Time {
	return dateOnly(time.Now().In(vietnam))
}

var vietnam = loadVietnam()

func loadVietnam() *time.Location {
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		return time.FixedZone("ICT", 7*60*60)
	}
	return loc
}

func dateOnly(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
