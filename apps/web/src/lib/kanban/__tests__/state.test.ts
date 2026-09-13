import { describe, expect, it } from "vitest";

import { kanbanReducer, type KanbanAction } from "../state";
import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask } from "../types";

const columnA = { id: asColumnId("col-a"), name: "To do", order: 0, isDone: false };
const columnB = { id: asColumnId("col-b"), name: "Done", order: 1, isDone: true };

function makeBoard(): KanbanBoard<KanbanTask> {
  return {
    columns: [columnA, columnB],
    tasks: [
      { id: asTaskId("task-1"), columnId: columnA.id, position: 0 },
      { id: asTaskId("task-2"), columnId: columnA.id, position: 1 },
    ],
  };
}

describe("kanbanReducer", () => {
  it("board/replaced returns the replacement board", () => {
    const board = makeBoard();
    const replacement = makeBoard();
    const next = kanbanReducer(board, { type: "board/replaced", board: replacement });
    expect(next).toBe(replacement);
  });

  it("tasks/moved updates only the matching task's columnId and position", () => {
    const board = makeBoard();
    const next = kanbanReducer(board, {
      type: "tasks/moved",
      taskId: asTaskId("task-1"),
      columnId: columnB.id,
      position: 0,
    });
    expect(next.tasks[0]).toMatchObject({ columnId: columnB.id, position: 0 });
    expect(next.tasks[1]).toBe(board.tasks[1]);
    expect(board.tasks[0]?.columnId).toBe(columnA.id);
  });

  it("tasks/upserted appends a new task and replaces an existing one", () => {
    const board = makeBoard();
    const inserted = kanbanReducer(board, {
      type: "tasks/upserted",
      task: { id: asTaskId("task-3"), columnId: columnA.id, position: 2 },
    });
    expect(inserted.tasks).toHaveLength(3);

    const replaced = kanbanReducer(board, {
      type: "tasks/upserted",
      task: { id: asTaskId("task-1"), columnId: columnB.id, position: 5 },
    });
    expect(replaced.tasks).toHaveLength(2);
    expect(replaced.tasks[0]).toMatchObject({ columnId: columnB.id, position: 5 });
  });

  it("tasks/removed drops the matching task only", () => {
    const board = makeBoard();
    const next = kanbanReducer(board, { type: "tasks/removed", taskId: asTaskId("task-1") });
    expect(next.tasks.map((task) => task.id)).toEqual([asTaskId("task-2")]);
  });

  it("columns/reordered rewrites order for listed columns and leaves others untouched", () => {
    const board = makeBoard();
    const next = kanbanReducer(board, {
      type: "columns/reordered",
      order: [columnB.id, columnA.id],
    });
    expect(next.columns.find((c) => c.id === columnA.id)?.order).toBe(1);
    expect(next.columns.find((c) => c.id === columnB.id)?.order).toBe(0);
  });

  it("columns/upserted appends a new column and replaces an existing one", () => {
    const board = makeBoard();
    const inserted = kanbanReducer(board, {
      type: "columns/upserted",
      column: { id: asColumnId("col-c"), name: "Blocked", order: 2, isDone: false },
    });
    expect(inserted.columns).toHaveLength(3);

    const replaced = kanbanReducer(board, {
      type: "columns/upserted",
      column: { id: columnA.id, name: "Renamed", order: 0, isDone: false },
    });
    expect(replaced.columns[0]?.name).toBe("Renamed");
  });

  it("columns/removed drops the matching column only", () => {
    const board = makeBoard();
    const next = kanbanReducer(board, { type: "columns/removed", columnId: columnA.id });
    expect(next.columns.map((c) => c.id)).toEqual([columnB.id]);
  });

  it("does not mutate the input board for any action", () => {
    const board = makeBoard();
    const snapshot = JSON.parse(JSON.stringify(board)) as unknown;
    kanbanReducer(board, { type: "tasks/removed", taskId: asTaskId("task-1") });
    kanbanReducer(board, { type: "columns/removed", columnId: columnA.id });
    expect(JSON.parse(JSON.stringify(board))).toEqual(snapshot);
  });

  it("returns the same board reference for an action of unrecognized shape", () => {
    const board = makeBoard();
    const unknownAction = { type: "tasks/exploded" } as unknown as KanbanAction<KanbanTask>;
    const next = kanbanReducer(board, unknownAction);
    expect(next).toBe(board);
  });
});
