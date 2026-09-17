import { DndContext, DragOverlay } from "@dnd-kit/core";
import { useCallback, useEffect, useRef, useState } from "react";

import type { ColumnId, KanbanColumn, TaskId, TaskPropsExtra } from "@/lib/kanban";

import type { BoardDndHandle } from "../hooks/use-board-dnd";
import type { AppTask } from "../hooks/use-tasks-data-source";
import { TaskCardPreview } from "./task-card-preview";
import { TaskColumn } from "./task-column";

export interface BoardDesktopProps {
  columns: KanbanColumn[];
  tasksByColumn: Map<ColumnId, AppTask[]>;
  assigneeNameFor: (assigneeId: string | null) => string | null;
  currentUserId: string | undefined;
  canCreate: boolean;
  canMove: boolean;
  dnd: BoardDndHandle<AppTask>;
  onOpenTask: (taskId: TaskId) => void;
  onCreateTask: (columnId: ColumnId) => void;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => void;
  getColumnProps: (columnId: ColumnId) => Record<string, unknown>;
  getTaskProps: (taskId: TaskId, extra?: TaskPropsExtra) => Record<string, unknown>;
}

/** Every column renders side by side; a 4th+ column scrolls horizontally rather than wrapping or shrinking below 230px. */
export function BoardDesktop({
  columns,
  tasksByColumn,
  assigneeNameFor,
  currentUserId,
  canCreate,
  canMove,
  dnd,
  onOpenTask,
  onCreateTask,
  onMoveTask,
  getColumnProps,
  getTaskProps,
}: BoardDesktopProps) {
  const sorted = [...columns].sort((a, b) => a.order - b.order);
  const { activeTask } = dnd;

  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScrollRight, setCanScrollRight] = useState(false);

  const updateCanScrollRight = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    // >1px tolerance so sub-pixel rounding at the end of the scroll range
    // doesn't leave the fade stuck visibly on.
    setCanScrollRight(el.scrollWidth - el.clientWidth - el.scrollLeft > 1);
  }, []);

  useEffect(() => {
    updateCanScrollRight();
    const el = scrollRef.current;
    // jsdom has no ResizeObserver by default outside the test setup stub.
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(updateCanScrollRight);
    observer.observe(el);
    return () => observer.disconnect();
  }, [columns.length, updateCanScrollRight]);

  return (
    <DndContext {...dnd.contextProps}>
      <div
        data-can-scroll-right={canScrollRight || undefined}
        className={[
          "relative after:pointer-events-none after:absolute after:inset-y-0 after:right-0",
          "after:w-8 after:bg-gradient-to-l after:from-cream-50 after:to-transparent",
          "after:opacity-0 after:transition-opacity data-[can-scroll-right]:after:opacity-100",
        ].join(" ")}
      >
        <div
          ref={scrollRef}
          onScroll={updateCanScrollRight}
          className="flex gap-3 overflow-x-auto overscroll-x-contain pb-2 [scroll-snap-type:x_proximity]"
        >
          {sorted.map((column) => (
            <TaskColumn
              key={column.id}
              column={column}
              tasks={tasksByColumn.get(column.id) ?? []}
              columns={columns}
              assigneeNameFor={assigneeNameFor}
              currentUserId={currentUserId}
              canCreate={canCreate}
              canMove={canMove}
              isDropTarget={activeTask !== null && dnd.overColumnId === column.id}
              reducedMotion={dnd.reducedMotion}
              onOpenTask={onOpenTask}
              onCreateTask={onCreateTask}
              onMoveTask={onMoveTask}
              getColumnProps={() => getColumnProps(column.id)}
              getTaskProps={getTaskProps}
              className="min-w-[230px] w-[280px] shrink-0 [scroll-snap-align:start]"
            />
          ))}
        </div>
      </div>
      <DragOverlay dropAnimation={dnd.reducedMotion ? null : undefined}>
        {activeTask ? (
          <TaskCardPreview
            task={activeTask}
            assigneeName={assigneeNameFor(activeTask.assigneeId)}
            isAssignedToMe={currentUserId !== undefined && activeTask.assigneeId === currentUserId}
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
