package classes

import (
	"testing"
	"time"
)

func TestPhaseOf(t *testing.T) {
	today := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	yesterday := today.AddDate(0, 0, -1)
	tomorrow := today.AddDate(0, 0, 1)
	past := today.AddDate(0, -3, 0)

	cases := []struct {
		name  string
		class Class
		want  string
	}{
		{"starts tomorrow", Class{Status: StatusActive, StartDate: tomorrow}, PhaseUpcoming},
		{"starts today", Class{Status: StatusActive, StartDate: today}, PhaseRunning},
		{"started, no end", Class{Status: StatusActive, StartDate: past}, PhaseRunning},
		{"ends today", Class{Status: StatusActive, StartDate: past, EndDate: &today}, PhaseRunning},
		{"ended yesterday", Class{Status: StatusActive, StartDate: past, EndDate: &yesterday}, PhaseEnded},
		{"archived wins over future start", Class{Status: StatusArchived, StartDate: tomorrow}, PhaseArchived},
		{"archived wins over past end", Class{Status: StatusArchived, StartDate: past, EndDate: &yesterday}, PhaseArchived},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PhaseOf(&tc.class, today); got != tc.want {
				t.Fatalf("PhaseOf = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPhaseOfIgnoresClockTime(t *testing.T) {
	// A class opening today must read as running even when "today" carries a
	// wall-clock time later than the DATE column's midnight.
	start := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 9, 23, 21, 15, 0, 0, time.UTC)
	if got := PhaseOf(&Class{Status: StatusActive, StartDate: start}, now); got != PhaseRunning {
		t.Fatalf("PhaseOf = %q, want running", got)
	}
	end := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	if got := PhaseOf(&Class{Status: StatusActive, StartDate: start.AddDate(0, -1, 0), EndDate: &end}, now); got != PhaseRunning {
		t.Fatalf("PhaseOf on the end date = %q, want running", got)
	}
}

func TestShiftOf(t *testing.T) {
	cases := map[TimeOfDay]string{
		"06:00": ShiftMorning,
		"11:59": ShiftMorning,
		"12:00": ShiftAfternoon,
		"17:29": ShiftAfternoon,
		"17:30": ShiftEvening,
		"19:00": ShiftEvening,
		"23:59": ShiftEvening,
	}
	for start, want := range cases {
		if got := ShiftOf(start); got != want {
			t.Errorf("ShiftOf(%q) = %q, want %q", start, got, want)
		}
	}
}

func TestShiftBoundsMatchShiftOf(t *testing.T) {
	// The SQL filter and the Go classifier must agree on every boundary.
	for _, shift := range []string{ShiftMorning, ShiftAfternoon, ShiftEvening} {
		from, to, ok := ShiftBounds(shift)
		if !ok {
			t.Fatalf("ShiftBounds(%q) unknown", shift)
		}
		if got := ShiftOf(from); got != shift {
			t.Errorf("lower bound %q of %s classifies as %s", from, shift, got)
		}
		if to != "" {
			if got := ShiftOf(to); got == shift {
				t.Errorf("upper bound %q of %s must be exclusive", to, shift)
			}
		}
	}
	if _, _, ok := ShiftBounds("night"); ok {
		t.Fatal("unknown shift must not resolve")
	}
}

func TestPhasePredicateRejectsUnknownPhase(t *testing.T) {
	today := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	for _, phase := range []string{PhaseUpcoming, PhaseRunning, PhaseEnded, PhaseArchived} {
		if _, _, ok := PhasePredicate(phase, today); !ok {
			t.Errorf("PhasePredicate(%q) must resolve", phase)
		}
	}
	if _, _, ok := PhasePredicate("paused", today); ok {
		t.Fatal("unknown phase must not resolve")
	}
}
