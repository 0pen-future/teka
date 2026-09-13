import { renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { KanbanDataSource } from "../data-source";
import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask } from "../types";
import { useKanban } from "../use-kanban";

const colA = { id: asColumnId("col-a"), name: "To do", order: 0, isDone: false };
const colB = { id: asColumnId("col-b"), name: "Doing", order: 1, isDone: false };

function makeBoard(): KanbanBoard<KanbanTask> {
  return {
    columns: [colA, colB],
    tasks: [{ id: asTaskId("t1"), columnId: colA.id, position: 0 }],
  };
}

function makeDataSource(): KanbanDataSource<KanbanTask> {
  return {
    loadBoard: vi.fn(),
    createColumn: vi.fn(),
    updateColumn: vi.fn(),
    reorderColumns: vi.fn().mockResolvedValue(undefined),
    deleteColumn: vi.fn(),
    createTask: vi.fn(),
    updateTask: vi.fn(),
    moveTask: vi.fn().mockResolvedValue({ id: asTaskId("t1"), columnId: colB.id, position: 0 }),
    deleteTask: vi.fn(),
  };
}

describe("useKanban", () => {
  it("returns the board prop unchanged and never holds its own copy", () => {
    const board = makeBoard();
    const dataSource = makeDataSource();
    const { result } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      {
        initialProps: { board },
      },
    );
    expect(result.current.board).toBe(board);
  });

  it("recomputes tasksByColumn when the board prop changes", () => {
    const dataSource = makeDataSource();
    const board1 = makeBoard();
    const { result, rerender } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      { initialProps: { board: board1 } },
    );
    expect(result.current.tasksByColumn.get(colA.id)).toHaveLength(1);
    expect(result.current.tasksByColumn.get(colB.id)).toHaveLength(0);

    const board2: KanbanBoard<KanbanTask> = {
      columns: [colA, colB],
      tasks: [{ id: asTaskId("t1"), columnId: colB.id, position: 0 }],
    };
    rerender({ board: board2 });
    expect(result.current.tasksByColumn.get(colA.id)).toHaveLength(0);
    expect(result.current.tasksByColumn.get(colB.id)).toHaveLength(1);
  });

  it("moveTask calls dataSource.moveTask with the given args and leaves board untouched until the prop changes", async () => {
    const dataSource = makeDataSource();
    const board = makeBoard();
    const { result, rerender } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      { initialProps: { board } },
    );

    await result.current.moveTask(asTaskId("t1"), colB.id, 0);

    expect(dataSource.moveTask).toHaveBeenCalledWith(asTaskId("t1"), colB.id, 0);
    // The hook does not mutate or replace its own board copy: same prop in,
    // same board out, until the app feeds a new board back.
    expect(result.current.board).toBe(board);
    expect(result.current.tasksByColumn.get(colB.id)).toHaveLength(0);

    const nextBoard: KanbanBoard<KanbanTask> = {
      columns: [colA, colB],
      tasks: [{ id: asTaskId("t1"), columnId: colB.id, position: 0 }],
    };
    rerender({ board: nextBoard });
    expect(result.current.board).toBe(nextBoard);
    expect(result.current.tasksByColumn.get(colB.id)).toHaveLength(1);
  });

  it("reorderColumn swaps the target column with its neighbor and calls dataSource.reorderColumns", async () => {
    const dataSource = makeDataSource();
    const board = makeBoard();
    const { result } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      { initialProps: { board } },
    );

    await result.current.reorderColumn(colA.id, "next");

    expect(dataSource.reorderColumns).toHaveBeenCalledWith([colB.id, colA.id]);
  });

  it("reorderColumn is a no-op at the board's edge", async () => {
    const dataSource = makeDataSource();
    const board = makeBoard();
    const { result } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      { initialProps: { board } },
    );

    await result.current.reorderColumn(colA.id, "previous");

    expect(dataSource.reorderColumns).not.toHaveBeenCalled();
  });

  it("getColumnProps and getTaskProps expose the verified a11y roles and merge extra handlers", () => {
    const dataSource = makeDataSource();
    const board = makeBoard();
    const { result } = renderHook(
      (props: { board: KanbanBoard<KanbanTask> }) => useKanban({ ...props, dataSource }),
      { initialProps: { board } },
    );

    const columnProps = result.current.getColumnProps(colA.id);
    expect(columnProps.role).toBe("listbox");
    expect(columnProps["aria-label"]).toBe("To do");

    const taskProps = result.current.getTaskProps(asTaskId("t1"));
    expect(taskProps.role).toBe("option");
    expect(typeof taskProps.tabIndex).toBe("number");
    expect(typeof taskProps.ref).toBe("function");

    const onClick = vi.fn();
    const withExtra = result.current.getTaskProps(asTaskId("t1"), { onClick });
    expect(withExtra.onClick).toBe(onClick);
  });
});
