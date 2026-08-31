import { describe, expect, it } from "vitest";

import type { AttendanceResponse, Session } from "@/features/attendance";

import { aggregateStudent, studentSessionRows, trendOf } from "../lib/student-stats";

describe("trendOf", () => {
  it("needs at least 4 scores before giving a verdict", () => {
    expect(trendOf([9, 9, 9])).toEqual({ arrow: "→", label: "Chưa đủ dữ liệu", tone: "flat" });
    expect(trendOf([])).toEqual({ arrow: "→", label: "Chưa đủ dữ liệu", tone: "flat" });
  });

  it("compares first-k vs last-k means with k = min(3, half)", () => {
    // n=4 → k=2: (5+5)/2 vs (8+8)/2 = +3.
    expect(trendOf([5, 5, 8, 8])).toMatchObject({ arrow: "↗", label: "Tiến bộ", tone: "up" });
    // n=8 → k=3: only the outer 3 on each side count.
    expect(trendOf([9, 9, 9, 5, 5, 6, 6, 6])).toMatchObject({ arrow: "↘", label: "Đi xuống" });
  });

  it("treats a delta of exactly 0.4 as stable — the band is strict", () => {
    // k=2: 7.0 vs 7.4 → +0.4, not > 0.4.
    expect(trendOf([7, 7, 7.4, 7.4])).toMatchObject({ arrow: "→", label: "Ổn định", tone: "flat" });
    expect(trendOf([7.4, 7.4, 7, 7])).toMatchObject({ arrow: "→", label: "Ổn định" });
    // Just past the band in both directions.
    expect(trendOf([7, 7, 7.5, 7.5])).toMatchObject({ label: "Tiến bộ" });
    expect(trendOf([7.5, 7.5, 7, 7])).toMatchObject({ label: "Đi xuống" });
  });
});

function makeSession(id: string, status: Session["status"], day: string): Session {
  return {
    id,
    status,
    class_id: "class-1",
    class_name: "Toán 6A",
    session_date: `2026-08-${day}`,
    start_time: "18:00",
    cancel_reason: null,
    attendance_confirmed_at: null,
    student_count: 2,
    attendance_summary: null,
    created_at: "2026-08-01T08:00:00Z",
  };
}

function makeRoster(
  sessionId: string,
  rows: { studentId: string; status: "present" | "absent" }[],
): AttendanceResponse {
  return {
    session_id: sessionId,
    session_date: "2026-08-05",
    status: "held",
    attendance_confirmed_at: "2026-08-05T20:00:00Z",
    rows: rows.map((row, index) => ({
      student_id: row.studentId,
      student_name: `Học sinh ${index}`,
      display_note: null,
      enrollment_id: `enrollment-${index}`,
      status: row.status,
      billable: row.status === "present",
      note: null,
    })),
  };
}

describe("studentSessionRows + aggregateStudent", () => {
  const sessions = [
    makeSession("s1", "held", "03"),
    makeSession("s2", "cancelled", "05"),
    makeSession("s3", "held", "10"),
    makeSession("s4", "held", "12"),
    makeSession("s5", "planned", "19"),
  ];
  const rosters = new Map<string, AttendanceResponse>([
    ["s1", makeRoster("s1", [{ studentId: "an", status: "present" }])],
    // s3 predates Bình's enrollment: no roster row for them there.
    ["s3", makeRoster("s3", [{ studentId: "an", status: "absent" }])],
    [
      "s4",
      makeRoster("s4", [
        { studentId: "an", status: "present" },
        { studentId: "binh", status: "present" },
      ]),
    ],
  ]);
  const scores = { s1: { an: 7 }, s4: { an: 8.5 } };

  it("keeps only held sessions where the student has a roster row", () => {
    const rows = studentSessionRows(sessions, rosters, scores, "an");
    expect(rows.map((row) => row.session.id)).toEqual(["s1", "s3", "s4"]);
    // The cancelled session is skipped on the lesson axis: s3 is Bài 2, s4 Bài 3.
    expect(rows.map((row) => row.lessonIndex)).toEqual([0, 1, 2]);
    expect(rows.map((row) => row.absent)).toEqual([false, true, false]);
    // Absent → no score even if one were stored; unscored present → null.
    expect(rows.map((row) => row.score)).toEqual([7, null, 8.5]);
  });

  it("excludes sessions before the student's roster rows from their record", () => {
    const rows = studentSessionRows(sessions, rosters, scores, "binh");
    expect(rows.map((row) => row.session.id)).toEqual(["s4"]);
    expect(rows[0]!.score).toBeNull();
  });

  it("aggregates scores in order, absences, and eligible-session count", () => {
    const aggregate = aggregateStudent(studentSessionRows(sessions, rosters, scores, "an"));
    expect(aggregate).toEqual({ scores: [7, 8.5], absences: 1, held: 3 });
  });
});
