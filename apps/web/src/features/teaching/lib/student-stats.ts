import type { AttendanceResponse, Session } from "@/features/attendance";

import { lessonIndexBySession } from "./classbook-stats";
import type { TeachingState } from "./teaching-store";

export type TrendTone = "up" | "flat" | "down";

export interface Trend {
  arrow: "↗" | "→" | "↘";
  label: string;
  tone: TrendTone;
}

/**
 * Prototype trend rule: with at least 4 scores, compare the mean of the first
 * k against the last k (k = min(3, ⌊n/2⌋)); a strict ±0.4 band separates
 * "Tiến bộ" / "Đi xuống" from "Ổn định". Fewer scores → no verdict.
 */
export function trendOf(scores: readonly number[]): Trend {
  if (scores.length < 4) {
    return { arrow: "→", label: "Chưa đủ dữ liệu", tone: "flat" };
  }
  const k = Math.min(3, Math.floor(scores.length / 2));
  const mean = (slice: readonly number[]) =>
    slice.reduce((sum, score) => sum + score, 0) / slice.length;
  const head = mean(scores.slice(0, k));
  const tail = mean(scores.slice(-k));
  // Strictly greater than 0.4, with float tolerance so a delta that IS 0.4
  // (e.g. 7.4 − 7 in binary) never flips the verdict on rounding noise.
  const band = 0.4 + 1e-9;
  if (tail - head > band) {
    return { arrow: "↗", label: "Tiến bộ", tone: "up" };
  }
  if (head - tail > band) {
    return { arrow: "↘", label: "Đi xuống", tone: "down" };
  }
  return { arrow: "→", label: "Ổn định", tone: "flat" };
}

export interface StudentSessionRow {
  session: Session;
  /** Position on the shared lesson axis (cancelled sessions excluded). */
  lessonIndex: number | null;
  absent: boolean;
  /** Stored score when present and scored; null when absent or unscored. */
  score: number | null;
}

/**
 * The student's held sessions this month, in list order. Only sessions whose
 * roster has loaded AND contains the student count — a session before the
 * student enrolled has no roster row and is invisible to their record, which
 * is what keeps averages and absence counts fair for mid-month joiners.
 */
export function studentSessionRows(
  sessions: readonly Session[],
  rosters: ReadonlyMap<string, AttendanceResponse>,
  sessionScores: TeachingState["sessionScores"],
  studentId: string,
): StudentSessionRow[] {
  const lessonAxis = lessonIndexBySession(sessions);
  const rows: StudentSessionRow[] = [];
  for (const session of sessions) {
    if (session.status !== "held") {
      continue;
    }
    const row = rosters.get(session.id)?.rows.find((item) => item.student_id === studentId);
    if (!row) {
      continue;
    }
    const absent = row.status === "absent";
    rows.push({
      session,
      lessonIndex: lessonAxis.get(session.id) ?? null,
      absent,
      score: absent ? null : (sessionScores[session.id]?.[studentId] ?? null),
    });
  }
  return rows;
}

export interface StudentAggregate {
  /** Scores in session order — the input to `trendOf`. */
  scores: number[];
  absences: number;
  /** Sessions the student was eligible for (present + absent). */
  held: number;
}

export function aggregateStudent(rows: readonly StudentSessionRow[]): StudentAggregate {
  return {
    scores: rows.map((row) => row.score).filter((score): score is number => score !== null),
    absences: rows.filter((row) => row.absent).length,
    held: rows.length,
  };
}
