import { DndContext, DragOverlay } from "@dnd-kit/core";
import { useCallback, useEffect, useRef, useState } from "react";

import type { ColumnId, TaskId, TaskPropsExtra } from "@/lib/kanban";

import type { BoardDndHandle } from "../hooks/use-board-dnd";
import type { AppColumn, AppTask } from "../hooks/use-tasks-data-source";
import { CollapsedColumnRail } from "./collapsed-column-rail";
import { TaskCardPreview } from "./task-card-preview";
import { TaskColumn } from "./task-column";

export interface BoardDesktopProps {
  columns: AppColumn[];
  tasksByColumn: Map<ColumnId, AppTask[]>;
  assigneeNameFor: (assigneeId: string | null) => string | null;
  canCreate: boolean;
  canMove: boolean;
  /** True when an active board filter, not a genuinely empty column, explains an empty column. */
  filtering: boolean;
  dnd: BoardDndHandle<AppTask>;
  /** Columns collapsed to a rail; persisted in the URL (see `useBoardUrlState`). */
  collapsed: Set<ColumnId>;
  onToggleCollapse: (columnId: ColumnId) => void;
  onOpenTask: (taskId: TaskId) => void;
  onCreateTask: (columnId: ColumnId) => void;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => void;
  onQuickDone: (taskId: TaskId) => void;
  getColumnProps: (columnId: ColumnId) => Record<string, unknown>;
  getTaskProps: (taskId: TaskId, extra?: TaskPropsExtra) => Record<string, unknown>;
}

/** Every column renders side by side at 272px; a 4th+ column scrolls horizontally rather than wrapping or shrinking. */
export function BoardDesktop({
  columns,
  tasksByColumn,
  assigneeNameFor,
  canCreate,
  canMove,
  filtering,
  dnd,
  collapsed,
  onToggleCollapse,
  onOpenTask,
  onCreateTask,
  onMoveTask,
  onQuickDone,
  getColumnProps,
  getTaskProps,
}: BoardDesktopProps) {
  const sorted = [...columns].sort((a, b) => a.order - b.order);
  const { activeTask } = dnd;

  const scrollRef = useRef<HTMLDivElement>(null);
  const [canScroll, setCanScroll] = useState({ left: false, right: false });

  const updateCanScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    // >1px tolerance so sub-pixel rounding at either end of the scroll range
    // doesn't leave a fade stuck visibly on.
    const left = el.scrollLeft > 1;
    const right = el.scrollWidth - el.clientWidth - el.scrollLeft > 1;
    setCanScroll((prev) => (prev.left === left && prev.right === right ? prev : { left, right }));
  }, []);

  useEffect(() => {
    updateCanScroll();
    const el = scrollRef.current;
    // jsdom has no ResizeObserver by default outside the test setup stub.
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(updateCanScroll);
    observer.observe(el);
    return () => observer.disconnect();
  }, [columns.length, updateCanScroll]);

  return (
    <DndContext {...dnd.contextProps}>
      <div
        data-can-scroll-left={canScroll.left || undefined}
        data-can-scroll-right={canScroll.right || undefined}
        className={[
          "relative",
          "before:pointer-events-none before:absolute before:inset-y-0 before:left-0 before:z-[1]",
          "before:w-8 before:bg-gradient-to-r before:from-cream-100 before:to-transparent",
          "before:opacity-0 before:transition-opacity data-[can-scroll-left]:before:opacity-100",
          "after:pointer-events-none after:absolute after:inset-y-0 after:right-0 after:z-[1]",
          "after:w-8 after:bg-gradient-to-l after:from-cream-100 after:to-transparent",
          "after:opacity-0 after:transition-opacity data-[can-scroll-right]:after:opacity-100",
        ].join(" ")}
      >
        <div
          ref={scrollRef}
          onScroll={updateCanScroll}
          className="flex gap-3 overflow-x-auto overscroll-x-contain px-0.5 pb-3 pt-0.5 [scroll-snap-type:x_proximity]"
        >
          {sorted.map((column) =>
            collapsed.has(column.id) ? (
              <CollapsedColumnRail
                key={column.id}
                column={column}
                taskCount={tasksByColumn.get(column.id)?.length ?? 0}
                onExpand={() => onToggleCollapse(column.id)}
                className="[scroll-snap-align:start]"
              />
            ) : (
              <TaskColumn
                key={column.id}
                column={column}
                tasks={tasksByColumn.get(column.id) ?? []}
                columns={columns}
                assigneeNameFor={assigneeNameFor}
                canCreate={canCreate}
                canMove={canMove}
                isDropTarget={activeTask !== null && dnd.overColumnId === column.id}
                reducedMotion={dnd.reducedMotion}
                filtering={filtering}
                onOpenTask={onOpenTask}
                onCreateTask={onCreateTask}
                onMoveTask={onMoveTask}
                onQuickDone={onQuickDone}
                onCollapse={() => onToggleCollapse(column.id)}
                getColumnProps={() => getColumnProps(column.id)}
                getTaskProps={getTaskProps}
                className="w-[272px] shrink-0 [scroll-snap-align:start]"
              />
            ),
          )}
        </div>
      </div>
      <DragOverlay dropAnimation={dnd.reducedMotion ? null : undefined}>
        {activeTask ? (
          <TaskCardPreview
            task={activeTask}
            assigneeName={assigneeNameFor(activeTask.assigneeId)}
          />
        ) : null}
      </DragOverlay>
    </DndContext>
  );
}
