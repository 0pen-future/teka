package sessions

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"teka/apps/api/internal/shared/id"
)

// TestFromDetailAttendanceSummaryNilUntilConfirmed pins the null rule: a
// session never confirmed reports attendance_summary null, so the UI can
// distinguish "not recorded yet" from "recorded, everyone absent".
func TestFromDetailAttendanceSummaryNilUntilConfirmed(t *testing.T) {
	d := &Detail{
		Row: Row{
			Session: Session{
				ID:          id.New(),
				SessionDate: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
				Status:      StatusPlanned,
			},
			ClassName: "Fixture Class",
		},
		StudentCount: 5,
	}
	resp := FromDetail(d)
	if resp.AttendanceSummary != nil {
		t.Fatalf("unconfirmed session must map attendance_summary to nil, got %+v", *resp.AttendanceSummary)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(raw), `"attendance_summary":null`) {
		t.Fatalf("wire body must carry an explicit null attendance_summary, got %s", raw)
	}
}

// TestFromDetailAttendanceSummaryCarriesCounts pins the confirmed mapping,
// including the confirmed-but-empty case: zero counts still yield a non-nil
// summary because the session was actually recorded.
func TestFromDetailAttendanceSummaryCarriesCounts(t *testing.T) {
	confirmedAt := time.Date(2026, 1, 6, 12, 0, 0, 0, time.UTC)
	d := &Detail{
		Row: Row{
			Session: Session{
				ID:                    id.New(),
				SessionDate:           time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
				Status:                StatusHeld,
				AttendanceConfirmedAt: &confirmedAt,
			},
			ClassName:    "Fixture Class",
			PresentCount: 2,
			LateCount:    1,
			AbsentCount:  1,
			ExcusedCount: 0,
		},
		StudentCount: 4,
	}
	resp := FromDetail(d)
	want := AttendanceSummary{Present: 2, Late: 1, Absent: 1, Excused: 0}
	if resp.AttendanceSummary == nil || *resp.AttendanceSummary != want {
		t.Fatalf("confirmed session must carry its counts, want %+v got %+v", want, resp.AttendanceSummary)
	}

	empty := &Detail{Row: Row{Session: Session{
		ID:                    id.New(),
		SessionDate:           time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC),
		Status:                StatusHeld,
		AttendanceConfirmedAt: &confirmedAt,
	}}}
	if got := FromDetail(empty).AttendanceSummary; got == nil || *got != (AttendanceSummary{}) {
		t.Fatalf("a confirmed session with an empty roster must report a zero summary, not null, got %+v", got)
	}
}

// TestPendingResponseAttendanceSummaryIsAlwaysNull pins the pending feed's
// shape: its predicate is attendance_confirmed_at IS NULL, so the summary is
// null by definition — the field exists only so all three session surfaces
// share one wire contract.
func TestPendingResponseAttendanceSummaryIsAlwaysNull(t *testing.T) {
	row := &PendingRow{
		Session: Session{
			ID:          id.New(),
			SessionDate: time.Date(2026, 1, 6, 0, 0, 0, 0, time.UTC),
			Status:      StatusPlanned,
		},
		ClassName:            "Fixture Class",
		ExpectedStudentCount: 3,
	}
	resp := fromPendingRow(row, time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	if resp.AttendanceSummary != nil {
		t.Fatalf("pending sessions are unconfirmed by predicate; summary must be nil, got %+v", *resp.AttendanceSummary)
	}
	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal response: %v", err)
	}
	if !strings.Contains(string(raw), `"attendance_summary":null`) {
		t.Fatalf("pending wire body must carry an explicit null attendance_summary, got %s", raw)
	}
}
