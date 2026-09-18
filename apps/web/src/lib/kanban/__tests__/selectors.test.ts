import { describe, expect, it } from "vitest";

import { selectAdjacentColumnId, selectColumnCounts, selectTasksByColumn } from "../selectors";
import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask } from "../types";

const colA = { id: asColumnId("col-a"), name: "To do", order: 0, isDone: false };
const colB = { id: asColumnId("col-b"), name: "Doing", order: 1, isDone: false };
const colC = { id: asColumnId("col-c"), name: "Done", order: 2, isDone: true };

describe("selectTasksByColumn", () => {
  it("groups tasks by column and includes empty columns", () => {
    const board: KanbanBoard<KanbanTask> = {
      columns: [colA, colB, colC],
      tasks: [{ id: asTaskId("t1"), columnId: colA.id, position: 0 }],
    };
    const byColumn = selectTasksByColumn(board);
    expect(byColumn.get(colA.id)).toHaveLength(1);
    expect(byColumn.get(colB.id)).toEqual([]);
    expect(byColumn.get(colC.id)).toEqual([]);
  });

  it("sorts by position ascending", () => {
    const board: KanbanBoard<KanbanTask> = {
      columns: [colA],
      tasks: [
        { id: asTaskId("t2"), columnId: colA.id, position: 2 },
        { id: asTaskId("t1"), columnId: colA.id, position: 0 },
        { id: asTaskId("t3"), columnId: colA.id, position: 1 },
      ],
    };
    const sorted = selectTasksByColumn(board).get(colA.id) ?? [];
    expect(sorted.map((t) => t.id)).toEqual([asTaskId("t1"), asTaskId("t3"), asTaskId("t2")]);
  });

  it("tie-breaks equal positions by createdAt ascending", () => {
    const board: KanbanBoard<KanbanTask> = {
      columns: [colA],
      tasks: [
        {
          id: asTaskId("later"),
          columnId: colA.id,
          position: 0,
          createdAt: "2026-01-02T00:00:00Z",
        },
        {
          id: asTaskId("earlier"),
          columnId: colA.id,
          position: 0,
          createdAt: "2026-01-01T00:00:00Z",
        },
      ],
    };
    const sorted = selectTasksByColumn(board).get(colA.id) ?? [];
    expect(sorted.map((t) => t.id)).toEqual([asTaskId("earlier"), asTaskId("later")]);
  });

  it("keeps stable input order when position and createdAt cannot disambiguate", () => {
    const board: KanbanBoard<KanbanTask> = {
      columns: [colA],
      tasks: [
        { id: asTaskId("first"), columnId: colA.id, position: 0 },
        { id: asTaskId("second"), columnId: colA.id, position: 0 },
      ],
    };
    const sorted = selectTasksByColumn(board).get(colA.id) ?? [];
    expect(sorted.map((t) => t.id)).toEqual([asTaskId("first"), asTaskId("second")]);
  });

  it("does not mutate the input board's task array", () => {
    const tasks: KanbanTask[] = [
      { id: asTaskId("t2"), columnId: colA.id, position: 2 },
      { id: asTaskId("t1"), columnId: colA.id, position: 0 },
    ];
    const board: KanbanBoard<KanbanTask> = { columns: [colA], tasks };
    selectTasksByColumn(board);
    expect(board.tasks).toBe(tasks);
    expect(board.tasks.map((t) => t.id)).toEqual([asTaskId("t2"), asTaskId("t1")]);
  });
});

describe("selectColumnCounts", () => {
  it("counts tasks per column and reports zero for empty columns", () => {
    const board: KanbanBoard<KanbanTask> = {
      columns: [colA, colB],
      tasks: [
        { id: asTaskId("t1"), columnId: colA.id, position: 0 },
        { id: asTaskId("t2"), columnId: colA.id, position: 1 },
      ],
    };
    const counts = selectColumnCounts(board);
    expect(counts.get(colA.id)).toBe(2);
    expect(counts.get(colB.id)).toBe(0);
  });
});

describe("selectAdjacentColumnId", () => {
  const board: KanbanBoard<KanbanTask> = { columns: [colA, colB, colC], tasks: [] };

  it("returns the next column ranked by order", () => {
    expect(selectAdjacentColumnId(board, colA.id, "next")).toBe(colB.id);
  });

  it("returns the previous column ranked by order", () => {
    expect(selectAdjacentColumnId(board, colC.id, "previous")).toBe(colB.id);
  });

  it("returns undefined past the last column", () => {
    expect(selectAdjacentColumnId(board, colC.id, "next")).toBeUndefined();
  });

  it("returns undefined before the first column", () => {
    expect(selectAdjacentColumnId(board, colA.id, "previous")).toBeUndefined();
  });

  it("returns undefined for an unknown column", () => {
    expect(selectAdjacentColumnId(board, asColumnId("missing"), "next")).toBeUndefined();
  });
});
