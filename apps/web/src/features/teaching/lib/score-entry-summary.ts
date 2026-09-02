import { parseScoreInput } from "@/components/hv";

export type ScoreCellState = "idle" | "dirty" | "saved" | "invalid";

/** One student×component cell as the score-entry UIs see it. */
export interface ScoreCell {
  /** Text currently shown in the cell — the draft while editing, else the stored score. */
  raw: string;
  /** Last value the server confirmed (`null` = empty cell). */
  server: number | null;
  state: ScoreCellState;
}

export interface StudentScoreSummary {
  /** Cells holding a readable score (draft or stored). */
  scored: number;
  /** Number of components, i.e. cells the student has. */
  total: number;
  /** Mean of the readable scores, `null` when none. */
  average: number | null;
}

export interface ScoreEntrySummary {
  /** Students with at least one readable score. */
  scoredStudents: number;
  /** Students considered (the rows passed in). */
  total: number;
  /** Cells edited and not yet confirmed by the server (invalid ones included). */
  dirtyCount: number;
  invalidCount: number;
  perStudent: Map<string, StudentScoreSummary>;
}

export function cellKey(studentId: string, componentId: string): string {
  return `${studentId}#${componentId}`;
}

/**
 * The value a cell currently stands for: the draft text when the user has
 * touched it (unreadable text counts as no score), otherwise the stored one.
 */
export function cellValue(cell: ScoreCell | undefined): number | null {
  if (!cell) return null;
  if (cell.state === "idle" || cell.state === "saved") return cell.server;
  const parsed = parseScoreInput(cell.raw);
  return typeof parsed === "number" ? parsed : null;
}

/**
 * Pure progress bookkeeping shared by the by-student panel and the full
 * table: per-student `k/M` + average, overall "n/N học sinh đã chấm", and
 * the unsaved/invalid cell counts the footer and the save button key off.
 * A student counts as graded once any cell holds a score — a session rarely
 * grades every column, so "all cells filled" would sit at 0/N forever.
 */
export function summarize(
  cells: ReadonlyMap<string, ScoreCell>,
  rosterRows: readonly { student_id: string }[],
  components: readonly { id: string }[],
): ScoreEntrySummary {
  const perStudent = new Map<string, StudentScoreSummary>();
  let scoredStudents = 0;
  for (const row of rosterRows) {
    let scored = 0;
    let sum = 0;
    for (const component of components) {
      const value = cellValue(cells.get(cellKey(row.student_id, component.id)));
      if (value !== null) {
        scored += 1;
        sum += value;
      }
    }
    if (scored > 0) scoredStudents += 1;
    perStudent.set(row.student_id, {
      scored,
      total: components.length,
      average: scored > 0 ? sum / scored : null,
    });
  }

  let dirtyCount = 0;
  let invalidCount = 0;
  for (const cell of cells.values()) {
    if (cell.state === "dirty" || cell.state === "invalid") dirtyCount += 1;
    if (cell.state === "invalid") invalidCount += 1;
  }

  return { scoredStudents, total: rosterRows.length, dirtyCount, invalidCount, perStudent };
}
