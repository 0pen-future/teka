import type { DragEndEvent, DragOverEvent, DragStartEvent } from "@dnd-kit/core";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask } from "@/lib/kanban";

import { useBoardDnd, type BoardDndData } from "../hooks/use-board-dnd";

const todo = asColumnId("col-todo");
const done = asColumnId("col-done");

const board: KanbanBoard<KanbanTask> = {
  columns: [
    { id: todo, name: "Cần làm", order: 0, isDone: false },
    { id: done, name: "Hoàn thành", order: 1, isDone: true },
  ],
  tasks: [
    { id: asTaskId("t1"), columnId: todo, position: 0 },
    { id: asTaskId("t2"), columnId: todo, position: 1 },
    { id: asTaskId("t3"), columnId: todo, position: 2 },
    { id: asTaskId("d1"), columnId: done, position: 0 },
  ],
};

/** Only the fields the adapter reads; dnd-kit's real events carry far more. */
function entry(id: string, data: BoardDndData) {
  return { id, data: { current: data } };
}
const taskEntry = (id: string, columnId: typeof todo) => entry(id, { type: "task", columnId });
const columnEntry = (id: typeof todo) => entry(id, { type: "column" });

function dragEnd(active: ReturnType<typeof entry>, over: ReturnType<typeof entry> | null) {
  return { active, over } as unknown as DragEndEvent;
}

function setup(allowCrossColumn: boolean) {
  const onMove = vi.fn();
  const hook = renderHook(() => useBoardDnd({ board, allowCrossColumn, onMove }));
  return { onMove, ...hook };
}

describe("useBoardDnd", () => {
  it("maps a drop onto another task in the same column to that task's slot", () => {
    const { result, onMove } = setup(true);

    act(() =>
      result.current.contextProps.onDragEnd?.(
        dragEnd(taskEntry("t1", todo), taskEntry("t3", todo)),
      ),
    );

    expect(onMove).toHaveBeenCalledWith(asTaskId("t1"), {
      columnId: todo,
      index: 2,
      afterTaskId: asTaskId("t3"),
    });
  });

  it("maps a drop onto a column's empty area to the bottom of that column", () => {
    const { result, onMove } = setup(true);

    act(() =>
      result.current.contextProps.onDragEnd?.(dragEnd(taskEntry("t2", todo), columnEntry(done))),
    );

    expect(onMove).toHaveBeenCalledWith(asTaskId("t2"), {
      columnId: done,
      index: 1,
      afterTaskId: asTaskId("d1"),
    });
  });

  it("ignores a drop back onto the dragged task itself and a drop outside any target", () => {
    const { result, onMove } = setup(true);

    act(() =>
      result.current.contextProps.onDragEnd?.(
        dragEnd(taskEntry("t1", todo), taskEntry("t1", todo)),
      ),
    );
    act(() => result.current.contextProps.onDragEnd?.(dragEnd(taskEntry("t1", todo), null)));

    expect(onMove).not.toHaveBeenCalled();
  });

  it("refuses a cross-column drop when allowCrossColumn is off but still reorders within the column", () => {
    const { result, onMove } = setup(false);

    act(() =>
      result.current.contextProps.onDragEnd?.(
        dragEnd(taskEntry("t1", todo), taskEntry("d1", done)),
      ),
    );
    expect(onMove).not.toHaveBeenCalled();

    act(() =>
      result.current.contextProps.onDragEnd?.(
        dragEnd(taskEntry("t1", todo), taskEntry("t2", todo)),
      ),
    );
    expect(onMove).toHaveBeenCalledWith(asTaskId("t1"), {
      columnId: todo,
      index: 1,
      afterTaskId: asTaskId("t2"),
    });
  });

  it("exposes the active task and hovered column while a drag is in flight, then clears both", () => {
    const { result } = setup(true);

    act(() =>
      result.current.contextProps.onDragStart?.({
        active: taskEntry("t2", todo),
      } as unknown as DragStartEvent),
    );
    expect(result.current.activeTask?.id).toBe(asTaskId("t2"));

    act(() =>
      result.current.contextProps.onDragOver?.({
        over: taskEntry("d1", done),
      } as unknown as DragOverEvent),
    );
    expect(result.current.overColumnId).toBe(done);

    act(() =>
      result.current.contextProps.onDragOver?.({
        over: columnEntry(todo),
      } as unknown as DragOverEvent),
    );
    expect(result.current.overColumnId).toBe(todo);

    act(() => result.current.contextProps.onDragCancel?.({} as never));
    expect(result.current.activeTask).toBeNull();
    expect(result.current.overColumnId).toBeNull();
  });

  it("switches collision detection with allowCrossColumn", () => {
    const desktop = setup(true);
    const mobile = setup(false);
    expect(desktop.result.current.contextProps.collisionDetection).not.toBe(
      mobile.result.current.contextProps.collisionDetection,
    );
  });
});
