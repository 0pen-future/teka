import { describe, expect, it } from "vitest";

import { cellKey, cellValue, summarize, type ScoreCell } from "../lib/score-entry-summary";

const components = [{ id: "c1" }, { id: "c2" }];
const roster = [{ student_id: "s1" }, { student_id: "s2" }, { student_id: "s3" }];

function cell(raw: string, server: number | null, state: ScoreCell["state"]): ScoreCell {
  return { raw, server, state };
}

describe("cellValue", () => {
  it.each([
    [undefined, null],
    [cell("8", 8, "idle"), 8],
    [cell("8", 8, "saved"), 8],
    [cell("7,5", 8, "dirty"), 7.5],
    [cell("", 8, "dirty"), null],
    [cell("abc", 8, "invalid"), null],
  ])("resolves %j to %j", (input, expected) => {
    expect(cellValue(input)).toBe(expected);
  });
});

describe("summarize", () => {
  it("counts scored students, per-student progress and averages", () => {
    const cells = new Map<string, ScoreCell>([
      [cellKey("s1", "c1"), cell("8", 8, "idle")],
      [cellKey("s1", "c2"), cell("9", 9, "idle")],
      [cellKey("s2", "c1"), cell("6,5", null, "dirty")],
      [cellKey("s2", "c2"), cell("", null, "idle")],
      [cellKey("s3", "c1"), cell("", null, "idle")],
    ]);

    const summary = summarize(cells, roster, components);

    expect(summary.scoredStudents).toBe(2);
    expect(summary.total).toBe(3);
    expect(summary.dirtyCount).toBe(1);
    expect(summary.invalidCount).toBe(0);
    expect(summary.perStudent.get("s1")).toEqual({ scored: 2, total: 2, average: 8.5 });
    expect(summary.perStudent.get("s2")).toEqual({ scored: 1, total: 2, average: 6.5 });
    expect(summary.perStudent.get("s3")).toEqual({ scored: 0, total: 2, average: null });
  });

  it("treats a cleared or unreadable draft as unscored but still unsaved", () => {
    const cells = new Map<string, ScoreCell>([
      [cellKey("s1", "c1"), cell("", 8, "dirty")],
      [cellKey("s1", "c2"), cell("abc", null, "invalid")],
    ]);

    const summary = summarize(cells, [roster[0]!], components);

    expect(summary.scoredStudents).toBe(0);
    expect(summary.dirtyCount).toBe(2);
    expect(summary.invalidCount).toBe(1);
    expect(summary.perStudent.get("s1")).toEqual({ scored: 0, total: 2, average: null });
  });

  it("does not count a just-saved flash as unsaved", () => {
    const cells = new Map<string, ScoreCell>([[cellKey("s1", "c1"), cell("8", 8, "saved")]]);
    expect(summarize(cells, [roster[0]!], components).dirtyCount).toBe(0);
  });
});
