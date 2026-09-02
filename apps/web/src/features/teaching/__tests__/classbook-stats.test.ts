import { describe, expect, it } from "vitest";

import type { AttendanceResponse, Session } from "@/features/attendance";
import type { Enrollment } from "@/features/roster";

import {
  activeHeadcount,
  classbookTotals,
  deriveSessions,
  lessonIndexBySession,
  meanScore,
  monthWindow,
  monthlyHeadcount,
  parseMonthParam,
  retentionStat,
  scoredStudentCount,
  sessionGross,
  sessionWorkStatus,
  shiftMonth,
  type SessionDerived,
} from "../lib/classbook-stats";

function makeSession(overrides: Partial<Session> & Pick<Session, "id" | "status">): Session {
  return {
    class_id: "class-1",
    class_name: "Toán 6A",
    session_date: "2026-08-05",
    start_time: "18:00",
    cancel_reason: null,
    attendance_confirmed_at: null,
    student_count: 2,
    attendance_summary: null,
    created_at: "2026-08-01T08:00:00Z",
    ...overrides,
  };
}

function makeRoster(
  sessionId: string,
  rows: { enrollmentId: string; status: "present" | "absent" }[],
): AttendanceResponse {
  return {
    session_id: sessionId,
    session_date: "2026-08-05",
    status: "held",
    attendance_confirmed_at: "2026-08-05T20:00:00Z",
    rows: rows.map((row, index) => ({
      student_id: `student-${index}`,
      student_name: `Học sinh ${index}`,
      display_note: null,
      enrollment_id: row.enrollmentId,
      status: row.status,
      billable: row.status === "present",
      note: null,
    })),
  };
}

function makeEnrollment(overrides: Partial<Enrollment> & Pick<Enrollment, "id">): Enrollment {
  return {
    student_id: overrides.id,
    student_name: "Học sinh",
    class_id: "class-1",
    class_name: "Toán 6A",
    started_on: "2026-01-05",
    ended_on: null,
    unit_price: 150_000,
    created_at: "2026-01-05T08:00:00Z",
    ...overrides,
  };
}

describe("meanScore", () => {
  it("rounds to one decimal", () => {
    expect(meanScore([7, 8, 8])).toBe(7.7);
  });

  it("returns null for no scores instead of a fake zero", () => {
    expect(meanScore([])).toBeNull();
  });
});

describe("sessionGross", () => {
  it("sums unit prices of present students only, missing enrollments as 0", () => {
    const roster = makeRoster("s1", [
      { enrollmentId: "e1", status: "present" },
      { enrollmentId: "e2", status: "absent" },
      { enrollmentId: "e-unknown", status: "present" },
    ]);
    const prices = new Map([
      ["e1", 150_000],
      ["e2", 120_000],
    ]);
    expect(sessionGross(roster, prices)).toBe(150_000);
  });
});

describe("lessonIndexBySession", () => {
  it("skips cancelled sessions in the lesson axis", () => {
    const map = lessonIndexBySession([
      makeSession({ id: "a", status: "held" }),
      makeSession({ id: "b", status: "cancelled" }),
      makeSession({ id: "c", status: "planned" }),
    ]);
    expect(map.get("a")).toBe(0);
    expect(map.has("b")).toBe(false);
    expect(map.get("c")).toBe(1);
  });
});

describe("deriveSessions + classbookTotals", () => {
  const sessions = [
    makeSession({ id: "held-1", status: "held" }),
    makeSession({ id: "cancelled-1", status: "cancelled" }),
    makeSession({ id: "held-2", status: "held" }),
    makeSession({ id: "planned-1", status: "planned", student_count: 3 }),
  ];
  const rosters = new Map<string, AttendanceResponse>([
    [
      "held-1",
      makeRoster("held-1", [
        { enrollmentId: "e1", status: "present" },
        { enrollmentId: "e2", status: "present" },
      ]),
    ],
    [
      "held-2",
      makeRoster("held-2", [
        { enrollmentId: "e1", status: "present" },
        { enrollmentId: "e2", status: "absent" },
      ]),
    ],
  ]);
  const prices = new Map([
    ["e1", 150_000],
    ["e2", 200_000],
  ]);

  it("derives per-session counts, revenue, and store averages", () => {
    const derived = deriveSessions(sessions, rosters, { "held-1": { s1: 7, s2: 8 } }, prices);

    const held1 = derived[0]!;
    // 150k + 200k gross − 300k cost ⇒ +50k
    expect(held1).toMatchObject({ present: 2, eligible: 2, gross: 350_000, net: 50_000 });
    expect(held1.average).toBe(7.5);

    // Only one present at 150k ⇒ 150k − 300k = −150k (the coral case)
    expect(derived[2]).toMatchObject({ present: 1, eligible: 2, net: -150_000 });
    expect(derived[2]!.average).toBeNull();

    expect(derived[1]).toMatchObject({
      lessonIndex: null,
      present: null,
      eligible: null,
      net: null,
    });
    expect(derived[3]).toMatchObject({ lessonIndex: 2, present: null, eligible: 3, net: null });
  });

  it("totals attendance and profit over roster-loaded held sessions only", () => {
    const derived = deriveSessions(sessions, rosters, { "held-1": { s1: 7, s2: 8 } }, prices);
    const totals = classbookTotals(derived);

    expect(totals.presentTotal).toBe(3);
    expect(totals.eligibleTotal).toBe(4);
    expect(totals.attendancePct).toBe(75);
    expect(totals.classAverage).toBe(7.5);
    expect(totals.scoredSessionCount).toBe(1);
    expect(totals.monthGross).toBe(500_000);
    expect(totals.monthCost).toBe(600_000);
    expect(totals.monthNet).toBe(-100_000);
  });

  it("excludes a held session whose roster has not loaded from both revenue sides", () => {
    const derived = deriveSessions(
      sessions,
      new Map([["held-1", rosters.get("held-1")!]]),
      {},
      prices,
    );
    const totals = classbookTotals(derived);
    expect(totals.monthGross).toBe(350_000);
    expect(totals.monthCost).toBe(300_000);
    expect(totals.monthNet).toBe(50_000);
  });
});

describe("activeHeadcount", () => {
  it("counts unique students with an open enrollment", () => {
    expect(
      activeHeadcount([
        makeEnrollment({ id: "a" }),
        makeEnrollment({ id: "b", student_id: "a" }),
        makeEnrollment({ id: "c", student_id: "gone", ended_on: "2026-07-31" }),
      ]),
    ).toBe(1);
  });
});

describe("retentionStat", () => {
  it("computes month-over-month continuation from enrollment windows", () => {
    const stat = retentionStat(
      [
        makeEnrollment({ id: "e1", student_id: "stays" }),
        makeEnrollment({ id: "e2", student_id: "left", ended_on: "2026-07-20" }),
      ],
      "2026-08-01",
    );
    expect(stat).toEqual({ previous: 2, continuing: 1, pct: 50 });
  });

  it("is 100% when the previous month had nobody", () => {
    const stat = retentionStat(
      [makeEnrollment({ id: "e1", student_id: "new", started_on: "2026-08-03" })],
      "2026-08-01",
    );
    expect(stat).toEqual({ previous: 0, continuing: 0, pct: 100 });
  });

  it("handles the January → December year boundary", () => {
    const stat = retentionStat(
      [makeEnrollment({ id: "e1", student_id: "stays", started_on: "2025-12-10" })],
      "2026-01-01",
    );
    expect(stat).toEqual({ previous: 1, continuing: 1, pct: 100 });
  });
});

describe("monthlyHeadcount", () => {
  it("counts unique students per month over the trailing window", () => {
    const history = monthlyHeadcount(
      [
        makeEnrollment({ id: "e1", student_id: "early", started_on: "2026-04-10" }),
        makeEnrollment({
          id: "e2",
          student_id: "left",
          started_on: "2026-04-01",
          ended_on: "2026-06-15",
        }),
        makeEnrollment({ id: "e3", student_id: "new", started_on: "2026-08-03" }),
        // Second enrollment window for an already-counted student — no double count.
        makeEnrollment({ id: "e4", student_id: "early", started_on: "2026-07-01" }),
      ],
      "2026-08-01",
    );
    expect(history).toEqual([
      { label: "T4", count: 2 },
      { label: "T5", count: 2 },
      { label: "T6", count: 2 },
      { label: "T7", count: 1 },
      { label: "T8", count: 2 },
    ]);
  });

  it("crosses the year boundary with correct month labels", () => {
    const history = monthlyHeadcount(
      [makeEnrollment({ id: "e1", student_id: "s", started_on: "2025-11-20" })],
      "2026-02-01",
    );
    expect(history.map((item) => item.label)).toEqual(["T10", "T11", "T12", "T1", "T2"]);
    expect(history.map((item) => item.count)).toEqual([0, 1, 1, 1, 1]);
  });
});

describe("month window", () => {
  it("ends today for the current month and spans the full month otherwise", () => {
    expect(monthWindow("2026-08", "2026-08-20")).toEqual({
      month: "2026-08",
      from: "2026-08-01",
      to: "2026-08-20",
      label: "08",
    });
    expect(monthWindow("2026-07", "2026-08-20")).toMatchObject({
      from: "2026-07-01",
      to: "2026-07-31",
    });
    expect(monthWindow("2026-09", "2026-08-20")).toMatchObject({
      from: "2026-09-01",
      to: "2026-09-30",
    });
    expect(monthWindow("2028-02", "2026-08-20").to).toBe("2028-02-29");
  });

  it("parses ?month= and falls back to today's month", () => {
    expect(parseMonthParam("2026-07", "2026-08-20")).toBe("2026-07");
    expect(parseMonthParam(null, "2026-08-20")).toBe("2026-08");
    expect(parseMonthParam("2026-13", "2026-08-20")).toBe("2026-08");
    expect(parseMonthParam("abc", "2026-08-20")).toBe("2026-08");
    expect(parseMonthParam("2026-8", "2026-08-20")).toBe("2026-08");
  });

  it("shifts across year boundaries", () => {
    expect(shiftMonth("2026-01", -1)).toBe("2025-12");
    expect(shiftMonth("2026-12", 1)).toBe("2027-01");
    expect(shiftMonth("2026-08", 0)).toBe("2026-08");
  });
});

describe("sessionWorkStatus", () => {
  const derived = (overrides: Partial<SessionDerived>): SessionDerived => ({
    session: makeSession({ id: "s1", status: "held" }),
    lessonIndex: 0,
    present: 2,
    eligible: 3,
    gross: 0,
    net: 0,
    average: null,
    ...overrides,
  });

  it("marks a held session done once it has a note and every present student scored", () => {
    expect(sessionWorkStatus(derived({}), "Lớp sôi nổi", 2)).toEqual({
      hasNote: true,
      scored: 2,
      total: 2,
      noteChip: "done",
      scoreChip: "done",
    });
  });

  it("flags missing note and partial scoring", () => {
    expect(sessionWorkStatus(derived({}), "   ", 1)).toMatchObject({
      noteChip: "missing",
      scoreChip: "partial",
      scored: 1,
      total: 2,
    });
  });

  it("shows no scoring chip while the roster is unknown or nobody was present", () => {
    // Roster still loading: a "0/0" chip would flash on every held row.
    expect(sessionWorkStatus(derived({ present: null }), "note", 0)).toMatchObject({
      noteChip: "done",
      scoreChip: "none",
      total: 0,
    });
    // Everyone absent: nothing to grade, so no work is pending either.
    expect(sessionWorkStatus(derived({ present: 0 }), undefined, 0).scoreChip).toBe("none");
  });

  it("carries no work for cancelled and planned sessions", () => {
    const cancelled = derived({
      session: makeSession({ id: "c", status: "cancelled" }),
      present: null,
    });
    expect(sessionWorkStatus(cancelled, undefined, 0)).toMatchObject({
      noteChip: "none",
      scoreChip: "none",
      total: 0,
    });
    const planned = derived({
      session: makeSession({ id: "p", status: "planned" }),
      present: null,
    });
    expect(sessionWorkStatus(planned, "note", 3).scoreChip).toBe("none");
  });

  it("counts students holding a general score", () => {
    expect(scoredStudentCount(undefined)).toBe(0);
    expect(scoredStudentCount({ a: 7, b: 8.5 })).toBe(2);
  });
});
