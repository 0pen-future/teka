import type { AttendanceResponse, Session } from "@/features/attendance";
import type { Enrollment } from "@/features/roster";

import { SESSION_COST_VND, type TeachingState } from "./teaching-store";

/**
 * Pure derivation layer for the classbook screen: real sessions/rosters/
 * enrollments in, the five stat values and per-session row data out. No
 * fetching, no formatting — the page fetches, the components format — so
 * every formula here is unit-testable without msw.
 */

/** One session joined with its roster, store scores, and revenue math. */
export interface SessionDerived {
  session: Session;
  /** Order among non-cancelled sessions — the lesson-plan/curriculum axis. Null for cancelled. */
  lessonIndex: number | null;
  /** Confirmed present count; null until the roster is loaded or for non-held sessions. */
  present: number | null;
  /** Roster size for held (loaded) sessions, `student_count` preview for planned, null for cancelled. */
  eligible: number | null;
  /** Σ present students' `unit_price`; null when not computable (not held / roster pending). */
  gross: number | null;
  /** `gross − SESSION_COST_VND`. */
  net: number | null;
  /** Mean of the store's stored scores for this session; null when nothing stored. */
  average: number | null;
}

/** Mean rounded to 1 decimal; null for an empty list — the UI shows "—", never a fake 0. */
export function meanScore(scores: readonly number[]): number | null {
  if (scores.length === 0) {
    return null;
  }
  return Math.round((scores.reduce((sum, score) => sum + score, 0) / scores.length) * 10) / 10;
}

/**
 * Rows whose enrollment is missing from the map contribute 0 rather than
 * poisoning the sum — the enrollments page is capped at 100 rows, and a
 * partially-priced total degrades more gracefully than no total.
 */
export function sessionGross(
  roster: AttendanceResponse,
  unitPriceByEnrollmentId: ReadonlyMap<string, number>,
): number {
  return roster.rows
    .filter((row) => row.status === "present")
    .reduce((sum, row) => sum + (unitPriceByEnrollmentId.get(row.enrollment_id) ?? 0), 0);
}

/** Session order among non-cancelled sessions (input must be date-ascending). */
export function lessonIndexBySession(sessions: readonly Session[]): Map<string, number> {
  const map = new Map<string, number>();
  let index = 0;
  for (const session of sessions) {
    if (session.status !== "cancelled") {
      map.set(session.id, index);
      index += 1;
    }
  }
  return map;
}

/**
 * Which lesson the upcoming session teaches: one past the held count, clamped
 * to the last lesson. Shared by the teacher's next-plan card and the owner's
 * review queue so both always point at the same `lessonPlanKey`.
 */
export function nextLessonIndex(totalLessons: number, doneCount: number): number {
  return totalLessons === 0 ? 0 : Math.min(doneCount, totalLessons - 1);
}

export function deriveSessions(
  sessions: readonly Session[],
  rosters: ReadonlyMap<string, AttendanceResponse>,
  sessionScores: TeachingState["sessionScores"],
  unitPriceByEnrollmentId: ReadonlyMap<string, number>,
): SessionDerived[] {
  const lessonIndexes = lessonIndexBySession(sessions);
  return sessions.map((session) => {
    const roster = session.status === "held" ? rosters.get(session.id) : undefined;
    const present = roster ? roster.rows.filter((row) => row.status === "present").length : null;
    const eligible = roster
      ? roster.rows.length
      : session.status === "planned"
        ? session.student_count
        : null;
    const gross = roster ? sessionGross(roster, unitPriceByEnrollmentId) : null;
    const scores = sessionScores[session.id];
    return {
      session,
      lessonIndex: lessonIndexes.get(session.id) ?? null,
      present,
      eligible,
      gross,
      net: gross === null ? null : gross - SESSION_COST_VND,
      average: scores ? meanScore(Object.values(scores)) : null,
    };
  });
}

export interface ClassbookTotals {
  presentTotal: number;
  eligibleTotal: number;
  attendancePct: number;
  /** Mean of held sessions' averages; null when no held session has scores yet. */
  classAverage: number | null;
  scoredSessionCount: number;
  monthGross: number;
  monthCost: number;
  monthNet: number;
}

/**
 * Aggregates over held sessions whose roster has loaded — a still-loading
 * roster is excluded from both the revenue and the cost side, so the LÃI/LỖ
 * breakdown always balances (`net = gross − cost`).
 */
export function classbookTotals(derived: readonly SessionDerived[]): ClassbookTotals {
  let presentTotal = 0;
  let eligibleTotal = 0;
  let monthGross = 0;
  let costedSessionCount = 0;
  const heldAverages: number[] = [];
  for (const row of derived) {
    if (row.session.status !== "held") {
      continue;
    }
    if (row.present !== null && row.eligible !== null) {
      presentTotal += row.present;
      eligibleTotal += row.eligible;
    }
    if (row.gross !== null) {
      monthGross += row.gross;
      costedSessionCount += 1;
    }
    if (row.average !== null) {
      heldAverages.push(row.average);
    }
  }
  const monthCost = costedSessionCount * SESSION_COST_VND;
  return {
    presentTotal,
    eligibleTotal,
    attendancePct: eligibleTotal === 0 ? 0 : Math.round((presentTotal / eligibleTotal) * 100),
    classAverage: meanScore(heldAverages),
    scoredSessionCount: heldAverages.length,
    monthGross,
    monthCost,
    monthNet: monthGross - monthCost,
  };
}

/** SĨ SỐ HIỆN TẠI — unique students with an open enrollment in this class. */
export function activeHeadcount(enrollments: readonly Enrollment[]): number {
  return new Set(
    enrollments.filter((enrollment) => enrollment.ended_on === null).map((e) => e.student_id),
  ).size;
}

export interface RetentionStat {
  /** Unique students whose enrollment window overlapped the previous month. */
  previous: number;
  /** Of those, how many also overlap the current month. */
  continuing: number;
  pct: number;
}

/** First/last day of the month `offset` months away from `monthStart` (a YYYY-MM-01 string). */
function monthRange(monthStart: string, offset: number): { from: string; to: string } {
  const [year = 0, month = 1] = monthStart.split("-").map(Number);
  const first = new Date(Date.UTC(year, month - 1 + offset, 1));
  const last = new Date(Date.UTC(year, month + offset, 0));
  return { from: first.toISOString().slice(0, 10), to: last.toISOString().slice(0, 10) };
}

function overlaps(enrollment: Enrollment, range: { from: string; to: string }): boolean {
  return (
    enrollment.started_on <= range.to &&
    (enrollment.ended_on === null || enrollment.ended_on >= range.from)
  );
}

/**
 * TÁI TỤC from real enrollment windows — the prototype fakes this from a
 * synthetic history. 100% when the previous month had nobody (nothing to
 * retain), matching the prototype's degenerate case.
 */
export function retentionStat(
  enrollments: readonly Enrollment[],
  monthStart: string,
): RetentionStat {
  const previousRange = monthRange(monthStart, -1);
  const currentRange = monthRange(monthStart, 0);
  const previousStudents = new Set(
    enrollments.filter((e) => overlaps(e, previousRange)).map((e) => e.student_id),
  );
  const currentStudents = new Set(
    enrollments.filter((e) => overlaps(e, currentRange)).map((e) => e.student_id),
  );
  let continuing = 0;
  for (const studentId of previousStudents) {
    if (currentStudents.has(studentId)) {
      continuing += 1;
    }
  }
  return {
    previous: previousStudents.size,
    continuing,
    pct: previousStudents.size === 0 ? 100 : Math.round((continuing / previousStudents.size) * 100),
  };
}

export interface MonthHeadcount {
  /** "T4"-style month label. */
  label: string;
  /** Unique students with an enrollment window overlapping that month. */
  count: number;
}

/** SĨ SỐ THEO THÁNG — the last `months` months ending at `monthStart`'s month. */
export function monthlyHeadcount(
  enrollments: readonly Enrollment[],
  monthStart: string,
  months = 5,
): MonthHeadcount[] {
  const history: MonthHeadcount[] = [];
  for (let offset = 1 - months; offset <= 0; offset += 1) {
    const range = monthRange(monthStart, offset);
    const students = new Set(
      enrollments.filter((enrollment) => overlaps(enrollment, range)).map((e) => e.student_id),
    );
    history.push({ label: `T${Number(range.from.slice(5, 7))}`, count: students.size });
  }
  return history;
}

export interface MonthWindow {
  /** "YYYY-MM" — the month itself, as carried in `?month=`. */
  month: string;
  /** First day of the month, YYYY-MM-DD. */
  from: string;
  /** Today for the current month (planned sessions past today stay out), else the last day. */
  to: string;
  /** "MM" — kept for the CSV filename (`{class}_ky{MM}.csv`). */
  label: string;
}

function localIso(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(
    date.getDate(),
  ).padStart(2, "0")}`;
}

export function todayIso(): string {
  return localIso(new Date());
}

const MONTH_PARAM = /^(\d{4})-(0[1-9]|1[0-2])$/;

/** `?month=` → a valid "YYYY-MM", falling back to `today`'s month. */
export function parseMonthParam(raw: string | null | undefined, today = todayIso()): string {
  return raw && MONTH_PARAM.test(raw) ? raw : today.slice(0, 7);
}

/** "YYYY-MM" moved by `delta` months, crossing years. */
export function shiftMonth(month: string, delta: number): string {
  const [year = 0, monthNumber = 1] = month.split("-").map(Number);
  return localIso(new Date(year, monthNumber - 1 + delta, 1)).slice(0, 7);
}

/**
 * The sessions-list window for a month. The current month ends today so the
 * table never lists a planned session that has not happened yet; any other
 * month spans its full length.
 */
export function monthWindow(month: string, today = todayIso()): MonthWindow {
  const [year = 0, monthNumber = 1] = month.split("-").map(Number);
  const from = localIso(new Date(year, monthNumber - 1, 1));
  const last = localIso(new Date(year, monthNumber, 0));
  const isCurrent = today.slice(0, 7) === month;
  return {
    month,
    from,
    to: isCurrent ? today : last,
    label: String(monthNumber).padStart(2, "0"),
  };
}

/** Students holding a general score for one session. */
export function scoredStudentCount(scores: Record<string, number> | undefined): number {
  return scores ? Object.keys(scores).length : 0;
}

export type NoteChip = "done" | "missing" | "none";
export type ScoreChip = "done" | "partial" | "none";

export interface SessionWorkStatus {
  hasNote: boolean;
  /** Students with at least one score. */
  scored: number;
  /** Students who can be scored — the session's present count. */
  total: number;
  noteChip: NoteChip;
  scoreChip: ScoreChip;
}

/**
 * The per-row "what is left to do" language: a held session either has its
 * whole-class note or not, and has n/N present students scored. Cancelled
 * and planned sessions carry no work, so both chips are `none`.
 */
export function sessionWorkStatus(
  derived: SessionDerived,
  noteText: string | undefined,
  scored: number,
): SessionWorkStatus {
  const hasNote = Boolean(noteText?.trim());
  const total = derived.present ?? 0;
  if (derived.session.status !== "held") {
    return { hasNote, scored: 0, total, noteChip: "none", scoreChip: "none" };
  }
  // No roster yet, or nobody present, means nothing to grade — no chip
  // rather than a misleading "0/0".
  const scoreChip: ScoreChip =
    derived.present === null || total === 0 ? "none" : scored >= total ? "done" : "partial";
  return { hasNote, scored, total, noteChip: hasNote ? "done" : "missing", scoreChip };
}

/** "7,5" — Vietnamese decimal comma for the ledger's score cells. */
export function formatLedgerScore(value: number | null): string {
  return value === null ? "—" : value.toFixed(1).replace(".", ",");
}
