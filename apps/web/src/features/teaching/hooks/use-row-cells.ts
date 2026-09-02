import { useMemo } from "react";

import type { HvScoreInputState } from "@/components/hv";
import type { AttendanceRow } from "@/features/attendance";

import { cellKey, cellValue, type ScoreCell } from "../lib/score-entry-summary";
import type { ClassScoreComponent } from "../schemas/teaching-schemas";

/** One student's cell as the row components consume it. */
export interface RowCell {
  key: string;
  componentName: string;
  raw: string;
  state: HvScoreInputState;
  /** What the cell stands for right now, for read-only rendering and averages. */
  value: number | null;
}

export function sameRowCells(a: readonly RowCell[], b: readonly RowCell[]): boolean {
  if (a.length !== b.length) return false;
  return a.every((cell, index) => {
    const other = b[index]!;
    return (
      cell.key === other.key &&
      cell.raw === other.raw &&
      cell.state === other.state &&
      cell.value === other.value &&
      cell.componentName === other.componentName
    );
  });
}

/**
 * `React.memo` comparator for row components: every prop by identity except
 * `cells`, which is compared structurally. `useRowCells` builds fresh arrays
 * on each keystroke (the draft's cell Map is new each time), so this is what
 * keeps typing in one student's cell from re-rendering every other row.
 */
export function areRowPropsEqual<P extends { cells: readonly RowCell[] }>(
  previous: P,
  next: P,
): boolean {
  if (Object.keys(previous).length !== Object.keys(next).length) return false;
  for (const key of Object.keys(next) as (keyof P)[]) {
    if (key === "cells") continue;
    if (!Object.is(previous[key], next[key])) return false;
  }
  return sameRowCells(previous.cells, next.cells);
}

/** Groups the draft's cell map by student, in component order. */
export function useRowCells(
  rosterRows: readonly AttendanceRow[],
  components: readonly ClassScoreComponent[],
  cells: ReadonlyMap<string, ScoreCell>,
): ReadonlyMap<string, RowCell[]> {
  return useMemo(() => {
    const next = new Map<string, RowCell[]>();
    for (const row of rosterRows) {
      next.set(
        row.student_id,
        components.map((component) => {
          const key = cellKey(row.student_id, component.id);
          const cell = cells.get(key);
          return {
            key,
            componentName: component.name,
            raw: cell?.raw ?? "",
            state: cell?.state ?? "idle",
            value: cellValue(cell),
          };
        }),
      );
    }
    return next;
  }, [rosterRows, components, cells]);
}

export interface RowStats {
  scored: number;
  average: number | null;
}

/** Scored-cell count and mean of a row's current values. */
export function rowStats(cells: readonly RowCell[]): RowStats {
  const values = cells.map((cell) => cell.value).filter((value): value is number => value !== null);
  return {
    scored: values.length,
    average: values.length > 0 ? values.reduce((a, b) => a + b, 0) / values.length : null,
  };
}

export function formatScore(value: number | null): string {
  return value === null ? "—" : String(value);
}

/** Row averages read like the ledger's ĐTB column: one decimal, Vietnamese comma. */
export { formatLedgerScore as formatAverage } from "../lib/classbook-stats";
