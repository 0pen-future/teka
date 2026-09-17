import {
  closestCenter,
  closestCorners,
  MouseSensor,
  TouchSensor,
  useSensor,
  useSensors,
  type DndContextProps,
  type DragEndEvent,
  type DragOverEvent,
  type DragStartEvent,
  type UniqueIdentifier,
} from "@dnd-kit/core";
import { useCallback, useMemo, useState } from "react";

import { useMediaQuery } from "@/lib/hooks/use-media-query";
import {
  asColumnId,
  asTaskId,
  resolveDrop,
  type ColumnId,
  type DropEvent,
  type DropTarget,
  type KanbanBoard,
  type KanbanTask,
  type TaskId,
} from "@/lib/kanban";

/**
 * `data` attached to every dnd-kit draggable/droppable on the board so a
 * collision can be mapped back onto the lib's `DropEvent` vocabulary
 * without guessing from the id alone (task and column ids share a shape).
 */
export type BoardDndData = { type: "task"; columnId: ColumnId } | { type: "column" };

export interface UseBoardDndOptions<TTask extends KanbanTask> {
  board: KanbanBoard<TTask>;
  /**
   * Whether a drop may land in a different column than the task started in.
   * Off on phones: only one column is on screen, and a cross-column drop
   * there would be a guess rather than a gesture.
   */
  allowCrossColumn: boolean;
  onMove: (taskId: TaskId, target: DropTarget) => void;
}

export interface BoardDndHandle<TTask extends KanbanTask> {
  /** Spread onto `<DndContext>` — sensors, collision detection, handlers, a11y. */
  contextProps: DndContextProps;
  /** The task under the pointer while a drag is in flight, for `<DragOverlay>`. */
  activeTask: TTask | null;
  /** The column the pointer is currently over (task or empty area), for highlighting. */
  overColumnId: ColumnId | null;
  /**
   * `prefers-reduced-motion` read once here rather than per card: cards
   * drop their sort transition and the overlay its drop animation.
   */
  reducedMotion: boolean;
}

/** Silence dnd-kit's built-in English announcer: the page owns the Vietnamese live region. */
const SILENT_ACCESSIBILITY: NonNullable<DndContextProps["accessibility"]> = {
  announcements: {
    onDragStart: () => undefined,
    onDragOver: () => undefined,
    onDragEnd: () => undefined,
    onDragCancel: () => undefined,
  },
  screenReaderInstructions: { draggable: "" },
};

function dataOf(entry: { data: { current: unknown } } | null | undefined): BoardDndData | null {
  const data = entry?.data.current as Partial<BoardDndData> | undefined;
  if (data?.type === "task" && data.columnId !== undefined) {
    return { type: "task", columnId: data.columnId };
  }
  if (data?.type === "column") {
    return { type: "column" };
  }
  return null;
}

function columnIdOf(overId: UniqueIdentifier, data: BoardDndData): ColumnId {
  return data.type === "column" ? asColumnId(String(overId)) : data.columnId;
}

/**
 * Adapter between dnd-kit's pointer gestures and the headless lib's pure
 * `resolveDrop`. Sensors are fixed for every viewport: a 6px mouse threshold
 * keeps clicks opening the card, a 250ms touch hold keeps the column
 * scrollable. There is deliberately no keyboard sensor — the lib's `[`/`]`
 * shortcut already covers keyboard moves with its own announcements — and no
 * pointer sensor, since mouse and touch need different activation rules.
 */
export function useBoardDnd<TTask extends KanbanTask>({
  board,
  allowCrossColumn,
  onMove,
}: UseBoardDndOptions<TTask>): BoardDndHandle<TTask> {
  const sensors = useSensors(
    useSensor(MouseSensor, { activationConstraint: { distance: 6 } }),
    useSensor(TouchSensor, { activationConstraint: { delay: 250, tolerance: 5 } }),
  );
  const reducedMotion = useMediaQuery("(prefers-reduced-motion: reduce)");
  const [activeId, setActiveId] = useState<TaskId | null>(null);
  const [overColumnId, setOverColumnId] = useState<ColumnId | null>(null);

  const activeTask = useMemo(
    () => (activeId ? (board.tasks.find((task) => task.id === activeId) ?? null) : null),
    [board, activeId],
  );

  const reset = useCallback(() => {
    setActiveId(null);
    setOverColumnId(null);
  }, []);

  const onDragStart = useCallback(({ active }: DragStartEvent) => {
    setActiveId(asTaskId(String(active.id)));
  }, []);

  const onDragOver = useCallback(({ over }: DragOverEvent) => {
    const data = dataOf(over);
    setOverColumnId(over && data ? columnIdOf(over.id, data) : null);
  }, []);

  const onDragEnd = useCallback(
    ({ active, over }: DragEndEvent) => {
      reset();
      const data = dataOf(over);
      if (!over || !data) return;
      const taskId = asTaskId(String(active.id));
      const event: DropEvent = { taskId, overId: String(over.id), overType: data.type };
      const target = resolveDrop(board, event);
      if (!target) return;
      const moving = board.tasks.find((task) => task.id === taskId);
      if (!allowCrossColumn && moving && moving.columnId !== target.columnId) return;
      onMove(taskId, target);
    },
    [board, allowCrossColumn, onMove, reset],
  );

  const contextProps = useMemo<DndContextProps>(
    () => ({
      sensors,
      collisionDetection: allowCrossColumn ? closestCorners : closestCenter,
      accessibility: SILENT_ACCESSIBILITY,
      onDragStart,
      onDragOver,
      onDragEnd,
      onDragCancel: reset,
    }),
    [sensors, allowCrossColumn, onDragStart, onDragOver, onDragEnd, reset],
  );

  return { contextProps, activeTask, overColumnId, reducedMotion };
}
