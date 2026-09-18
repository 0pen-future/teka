import { describe, expect, it } from "vitest";

import { afterTaskIdAt, optimisticPositionFor, positionBetween, resolveDrop } from "../positions";
import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask, type TaskId } from "../types";

const colA = { id: asColumnId("col-a"), name: "To do", order: 0, isDone: false };
const colB = { id: asColumnId("col-b"), name: "Doing", order: 1, isDone: false };
const colEmpty = { id: asColumnId("col-empty"), name: "Done", order: 2, isDone: true };

function task(id: string, columnId: typeof colA.id, position: number): KanbanTask {
  return { id: asTaskId(id), columnId, position };
}

const A = asTaskId("A");
const B = asTaskId("B");
const C = asTaskId("C");
const X = asTaskId("X");
const Y = asTaskId("Y");

// Column A holds A,B,C top to bottom; column B holds X,Y. Positions are
// deliberately not 0..n and not in insertion order so the tests prove the
// helpers go through the sorted view, not board.tasks order.
function makeBoard(): KanbanBoard<KanbanTask> {
  return {
    columns: [colA, colB, colEmpty],
    tasks: [
      task("C", colA.id, 30),
      task("A", colA.id, 10),
      task("B", colA.id, 20),
      task("Y", colB.id, 5),
      task("X", colB.id, 1),
    ],
  };
}

const columnA: KanbanTask[] = [
  task("A", colA.id, 10),
  task("B", colA.id, 20),
  task("C", colA.id, 30),
];

/** Oracle: what a vertical sortable list shows after dragging `from` onto `to`. */
function arrayMove<T>(items: readonly T[], from: number, to: number): T[] {
  const next = items.slice();
  next.splice(to, 0, ...next.splice(from, 1));
  return next;
}

describe("afterTaskIdAt", () => {
  it("returns null for index 0 (top of the column)", () => {
    expect(afterTaskIdAt(columnA, 0, X)).toBeNull();
    expect(afterTaskIdAt([], 0, X)).toBeNull();
  });

  it("returns the task at index - 1 once the moving task is removed", () => {
    // Moving task not in the column: plain index - 1.
    expect(afterTaskIdAt(columnA, 1, X)).toBe(A);
    expect(afterTaskIdAt(columnA, 2, X)).toBe(B);
    expect(afterTaskIdAt(columnA, 3, X)).toBe(C);
    // Moving task before the index: it is skipped, remaining = [B, C].
    expect(afterTaskIdAt(columnA, 1, A)).toBe(B);
    expect(afterTaskIdAt(columnA, 2, A)).toBe(C);
    // Moving task after the index: it does not shift the earlier ones.
    expect(afterTaskIdAt(columnA, 1, C)).toBe(A);
  });

  it("clamps an index past the end to the bottom of the column", () => {
    expect(afterTaskIdAt(columnA, 99, X)).toBe(C);
    expect(afterTaskIdAt(columnA, 99, C)).toBe(B);
    expect(afterTaskIdAt(columnA.slice(0, 1), 1, A)).toBeNull();
  });
});

describe("positionBetween", () => {
  it("returns 0 when the column is empty", () => {
    expect(positionBetween(undefined, undefined)).toBe(0);
  });

  it("goes below the first task when moving to the top, even into negatives", () => {
    expect(positionBetween(undefined, 5)).toBe(4);
    expect(positionBetween(undefined, 0)).toBe(-1);
  });

  it("goes above the last task when moving to the bottom", () => {
    expect(positionBetween(7, undefined)).toBe(8);
  });

  it("averages both neighbours, however small the gap", () => {
    expect(positionBetween(10, 20)).toBe(15);
    expect(positionBetween(1, 2)).toBe(1.5);
    expect(positionBetween(1.5, 1.5000001)).toBeCloseTo(1.50000005, 8);
  });
});

describe("optimisticPositionFor", () => {
  it("uses the neighbours at index with the moving task removed", () => {
    expect(optimisticPositionFor(columnA, 0, X)).toBe(9);
    expect(optimisticPositionFor(columnA, 1, X)).toBe(15);
    expect(optimisticPositionFor(columnA, 3, X)).toBe(31);
    // Moving A to index 1 of [B, C] → between B and C.
    expect(optimisticPositionFor(columnA, 1, A)).toBe(25);
    // Moving C to the top → below A.
    expect(optimisticPositionFor(columnA, 0, C)).toBe(9);
    expect(optimisticPositionFor([], 0, X)).toBe(0);
  });

  it("clamps index like afterTaskIdAt so both describe the same slot", () => {
    // Index counted on the un-filtered list (3) while C is the moving task:
    // afterTaskIdAt says "after B", so the position must be above B too.
    expect(afterTaskIdAt(columnA, 3, C)).toBe(B);
    expect(optimisticPositionFor(columnA, 3, C)).toBe(21);
    expect(optimisticPositionFor(columnA, 99, X)).toBe(31);
    expect(optimisticPositionFor(columnA, -1, X)).toBe(9);
  });
});

describe("resolveDrop", () => {
  describe("over a task in the same column (arrayMove semantics)", () => {
    it("[A,B,C] dragging A onto C gives [B,C,A]", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: C, overType: "task" })).toEqual({
        columnId: colA.id,
        index: 2,
        afterTaskId: C,
      });
    });

    it("[A,B,C] dragging C onto A gives [C,A,B]", () => {
      expect(resolveDrop(makeBoard(), { taskId: C, overId: A, overType: "task" })).toEqual({
        columnId: colA.id,
        index: 0,
        afterTaskId: null,
      });
    });

    it("[A,B,C] dragging A onto B gives [B,A,C]", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: B, overType: "task" })).toEqual({
        columnId: colA.id,
        index: 1,
        afterTaskId: B,
      });
    });

    it("matches arrayMove for every (from, to) pair", () => {
      const ids: TaskId[] = [A, B, C];
      for (const [from, moving] of ids.entries()) {
        for (const [to, over] of ids.entries()) {
          if (from === to) {
            continue;
          }
          const expected = arrayMove(ids, from, to);
          const target = resolveDrop(makeBoard(), {
            taskId: moving,
            overId: over,
            overType: "task",
          });
          expect(target, `${from} -> ${to}`).not.toBeNull();
          const index = expected.indexOf(moving);
          expect(target?.index, `${from} -> ${to}`).toBe(index);
          expect(target?.afterTaskId, `${from} -> ${to}`).toBe(expected[index - 1] ?? null);
        }
      }
    });
  });

  describe("over a task in another column", () => {
    it("inserts before the task at the top", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: X, overType: "task" })).toEqual({
        columnId: colB.id,
        index: 0,
        afterTaskId: null,
      });
    });

    it("inserts before a task further down", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: Y, overType: "task" })).toEqual({
        columnId: colB.id,
        index: 1,
        afterTaskId: X,
      });
    });
  });

  describe("over a column", () => {
    it("lands at index 0 in an empty column", () => {
      expect(
        resolveDrop(makeBoard(), { taskId: A, overId: colEmpty.id, overType: "column" }),
      ).toEqual({ columnId: colEmpty.id, index: 0, afterTaskId: null });
    });

    it("lands at the bottom of a column that has tasks", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: colB.id, overType: "column" })).toEqual({
        columnId: colB.id,
        index: 2,
        afterTaskId: Y,
      });
    });

    it("lands at the bottom of its own column without counting itself", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: colA.id, overType: "column" })).toEqual({
        columnId: colA.id,
        index: 2,
        afterTaskId: C,
      });
    });

    it("returns null when the last task of a column is dropped on that column", () => {
      expect(
        resolveDrop(makeBoard(), { taskId: C, overId: colA.id, overType: "column" }),
      ).toBeNull();
      expect(
        resolveDrop(makeBoard(), { taskId: Y, overId: colB.id, overType: "column" }),
      ).toBeNull();
    });
  });

  describe("no-ops", () => {
    it("returns null when a task is dropped on itself", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: A, overType: "task" })).toBeNull();
    });

    it("returns null when `over` is not on the board", () => {
      expect(resolveDrop(makeBoard(), { taskId: A, overId: "ghost", overType: "task" })).toBeNull();
      expect(
        resolveDrop(makeBoard(), { taskId: A, overId: "ghost", overType: "column" }),
      ).toBeNull();
    });

    it("returns null when the dragged task is not on the board", () => {
      expect(
        resolveDrop(makeBoard(), { taskId: asTaskId("ghost"), overId: B, overType: "task" }),
      ).toBeNull();
    });
  });
});
