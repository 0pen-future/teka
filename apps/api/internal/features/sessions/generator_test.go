package sessions_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"teka/apps/api/internal/features/sessions"
)

func d(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func dp(s string) *time.Time {
	t := d(s)
	return &t
}

func dates(ss ...string) []time.Time {
	out := make([]time.Time, len(ss))
	for i, s := range ss {
		out[i] = d(s)
	}
	return out
}

// vietnam is used to exercise the loc-aware path; Vietnam has no DST so its
// calendar arithmetic is identical to UTC, but threading a non-UTC zone
// through the algorithm still proves loc is honoured rather than ignored.
func vietnam(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	return loc
}

func TestExpand(t *testing.T) {
	t.Parallel()
	loc := vietnam(t)

	tests := []struct {
		name      string
		class     sessions.ClassWindow
		schedules []sessions.ScheduleWindow
		from, to  string
		want      []time.Time
	}{
		{
			name:  "class starts mid-range: no session before start_date",
			class: sessions.ClassWindow{StartDate: d("2026-01-14")}, // Wednesday
			schedules: []sessions.ScheduleWindow{
				{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-01")},
			},
			from: "2026-01-01", to: "2026-01-31",
			want: dates("2026-01-14", "2026-01-21", "2026-01-28"),
		},
		{
			name: "end_date inside range: no session after end_date",
			class: sessions.ClassWindow{
				StartDate: d("2026-01-01"),
				EndDate:   dp("2026-01-20"),
			},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-01")},
			},
			from: "2026-01-01", to: "2026-01-31",
			want: dates("2026-01-07", "2026-01-14"),
		},
		{
			name:  "effective_to NULL means open-ended, clamped only by the request range",
			class: sessions.ClassWindow{StartDate: d("2026-01-01")},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-01")},
			},
			from: "2026-02-01", to: "2026-02-28",
			want: dates("2026-02-04", "2026-02-11", "2026-02-18", "2026-02-25"),
		},
		{
			name:  "two schedules on different weekdays merge and sort",
			class: sessions.ClassWindow{StartDate: d("2026-01-01")},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 2, StartTime: "18:00", EffectiveFrom: d("2026-01-01")}, // Tuesday
				{Weekday: 4, StartTime: "09:00", EffectiveFrom: d("2026-01-01")}, // Thursday
			},
			from: "2026-01-05", to: "2026-01-11",
			want: dates("2026-01-06", "2026-01-08"),
		},
		{
			name:  "schedule replaced mid-range: no gap, no overlap",
			class: sessions.ClassWindow{StartDate: d("2026-01-01")},
			schedules: []sessions.ScheduleWindow{
				// Old weekday (Tuesday) runs through 2026-01-13.
				{Weekday: 2, StartTime: "18:00", EffectiveFrom: d("2026-01-01"), EffectiveTo: dp("2026-01-13")},
				// New weekday (Thursday) starts the following day.
				{Weekday: 4, StartTime: "09:00", EffectiveFrom: d("2026-01-14")},
			},
			from: "2026-01-01", to: "2026-01-31",
			// Tuesdays 6th, 13th (old); Thursdays 15th, 22nd, 29th (new) — no
			// Tuesday after the 13th, no Thursday before the 14th.
			want: dates("2026-01-06", "2026-01-13", "2026-01-15", "2026-01-22", "2026-01-29"),
		},
		{
			name:  "requested range entirely predates the class: empty result",
			class: sessions.ClassWindow{StartDate: d("2026-06-01")},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 2, StartTime: "18:00", EffectiveFrom: d("2026-06-01")},
			},
			from: "2026-01-01", to: "2026-01-31",
			want: nil,
		},
		{
			name:  "weekday 0 is Sunday",
			class: sessions.ClassWindow{StartDate: d("2026-01-01")},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 0, StartTime: "09:00", EffectiveFrom: d("2026-01-01")},
			},
			from: "2026-01-01", to: "2026-01-31",
			want: dates("2026-01-04", "2026-01-11", "2026-01-18", "2026-01-25"),
		},
		{
			name:  "no schedules: empty result even inside the class window",
			class: sessions.ClassWindow{StartDate: d("2026-01-01")},
			from:  "2026-01-01", to: "2026-01-31",
			want: nil,
		},
		{
			name:  "class end_date before the schedule's effective_from: empty result",
			class: sessions.ClassWindow{StartDate: d("2026-01-01"), EndDate: dp("2026-01-05")},
			schedules: []sessions.ScheduleWindow{
				{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-10")},
			},
			from: "2026-01-01", to: "2026-01-31",
			want: nil,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := sessions.Expand(tc.class, tc.schedules, d(tc.from), d(tc.to), loc)
			require.Equal(t, tc.want, got)
		})
	}
}

func TestExpandNilLocationDefaultsToUTC(t *testing.T) {
	t.Parallel()
	class := sessions.ClassWindow{StartDate: d("2026-01-01")}
	schedules := []sessions.ScheduleWindow{
		{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-01")},
	}
	got := sessions.Expand(class, schedules, d("2026-01-01"), d("2026-01-31"), nil)
	require.Equal(t, dates("2026-01-07", "2026-01-14", "2026-01-21", "2026-01-28"), got)
}

func TestExpandToBeforeFromIsEmpty(t *testing.T) {
	t.Parallel()
	class := sessions.ClassWindow{StartDate: d("2026-01-01")}
	schedules := []sessions.ScheduleWindow{
		{Weekday: 3, StartTime: "18:00", EffectiveFrom: d("2026-01-01")},
	}
	got := sessions.Expand(class, schedules, d("2026-01-31"), d("2026-01-01"), time.UTC)
	require.Nil(t, got)
}

func TestScheduleForFindsTheMatchingWindow(t *testing.T) {
	t.Parallel()
	schedules := []sessions.ScheduleWindow{
		{Weekday: 2, StartTime: "18:00", EffectiveFrom: d("2026-01-01"), EffectiveTo: dp("2026-01-13")},
		{Weekday: 4, StartTime: "09:00", EffectiveFrom: d("2026-01-14")},
	}

	sw, ok := sessions.ScheduleFor(schedules, d("2026-01-06"))
	require.True(t, ok)
	require.Equal(t, "18:00", sw.StartTime)

	sw, ok = sessions.ScheduleFor(schedules, d("2026-01-22"))
	require.True(t, ok)
	require.Equal(t, "09:00", sw.StartTime)

	_, ok = sessions.ScheduleFor(schedules, d("2026-01-15")) // Thursday, but before the 14th's schedule starts? no — 15th is after 14th
	require.True(t, ok)

	_, ok = sessions.ScheduleFor(schedules, d("2026-01-20")) // Tuesday, after the old schedule closed on the 13th
	require.False(t, ok)
}
