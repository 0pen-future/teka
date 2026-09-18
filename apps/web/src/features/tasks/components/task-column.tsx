import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CheckIcon, ChevronLeftIcon, PlusIcon } from "lucide-react";
import type { ComponentProps } from "react";

import type { ColumnId, TaskId, TaskPropsExtra } from "@/lib/kanban";
import { cn } from "@/lib/utils";

import type { BoardDndData } from "../hooks/use-board-dnd";
import type { AppColumn, AppTask } from "../hooks/use-tasks-data-source";
import { columnTintClassName } from "../lib/column-colors";
import { TaskCard } from "./task-card";

export interface TaskColumnProps {
  column: AppColumn;
  tasks: AppTask[];
  columns: AppColumn[];
  assigneeNameFor: (assigneeId: string | null) => string | null;
  canCreate: boolean;
  canMove: boolean;
  /** True while a dragged card hovers anywhere over this column. */
  isDropTarget: boolean;
  reducedMotion: boolean;
  /** True when this column is empty because a board filter hid every task, not because it has none. */
  filtering: boolean;
  onOpenTask: (taskId: TaskId) => void;
  onCreateTask: (columnId: ColumnId) => void;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => void;
  onQuickDone: (taskId: TaskId) => void;
  getColumnProps: () => ComponentProps<"div">;
  getTaskProps: (taskId: TaskId, extra: TaskPropsExtra) => ComponentProps<"div">;
  /** Desktop-only: omitted on mobile, which does not support collapsing columns. */
  onCollapse?: () => void;
  className?: string;
}

const HEADER_BUTTON =
  "inline-flex size-9 shrink-0 items-center justify-center rounded-full text-ink-500 transition-colors hover:bg-white hover:text-ink-900 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100";

/**
 * One kanban column: header (name, count, done marker) plus its task list.
 * The list is both the lib's listbox and a dnd-kit droppable, so a card
 * dropped on the column's empty space (the trailing spacer keeps some even
 * in a full column) lands at the bottom rather than nowhere.
 */
export function TaskColumn({
  column,
  tasks,
  columns,
  assigneeNameFor,
  canCreate,
  canMove,
  isDropTarget,
  reducedMotion,
  filtering,
  onOpenTask,
  onCreateTask,
  onMoveTask,
  onQuickDone,
  getColumnProps,
  getTaskProps,
  onCollapse,
  className,
}: TaskColumnProps) {
  const droppableData: BoardDndData = { type: "column" };
  const { setNodeRef } = useDroppable({ id: column.id, data: droppableData, disabled: !canMove });

  return (
    <div
      data-over={isDropTarget || undefined}
      className={cn(
        "flex min-h-[360px] flex-col rounded-[16px] p-2 transition-[box-shadow,background-color]",
        columnTintClassName(column),
        isDropTarget && "shadow-[0_0_0_2px_var(--color-mint-400),0_0_0_6px_var(--color-mint-100)]",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-1.5 pb-2 pl-2 pr-1 pt-1">
        <div className="flex min-w-0 items-center gap-[7px] text-[15px] font-extrabold text-ink-900">
          {column.isDone ? (
            <CheckIcon aria-hidden className="size-4 shrink-0 text-mint-600" />
          ) : null}
          <p className="truncate">{column.name}</p>
          <span className="shrink-0 rounded-full bg-white px-2 py-[3px] font-body text-[11.5px] font-extrabold tabular-nums text-ink-500">
            {tasks.length}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-0.5">
          {onCollapse ? (
            <button
              type="button"
              onClick={onCollapse}
              aria-expanded
              aria-label={`Thu gọn cột ${column.name}`}
              className={HEADER_BUTTON}
            >
              <ChevronLeftIcon aria-hidden className="size-[18px]" />
            </button>
          ) : null}
          {canCreate ? (
            <button
              type="button"
              onClick={() => onCreateTask(column.id)}
              aria-label={`Thêm việc vào ${column.name}`}
              className={HEADER_BUTTON}
            >
              <PlusIcon aria-hidden className="size-[18px]" />
            </button>
          ) : null}
        </div>
      </div>
      <SortableContext items={tasks.map((task) => task.id)} strategy={verticalListSortingStrategy}>
        <div
          {...getColumnProps()}
          ref={setNodeRef}
          className="flex min-h-[80px] flex-1 flex-col gap-2 rounded-[12px]"
        >
          {tasks.length === 0 ? (
            <div
              className={cn(
                "flex min-h-[120px] flex-1 flex-col items-center justify-center gap-1.5 rounded-[12px]",
                "border-2 border-dashed border-line-300 px-3 py-5 text-center text-[13px] text-ink-500",
                isDropTarget && "border-mint-400 bg-mint-50",
              )}
            >
              <p>{filtering ? "Không có việc khớp bộ lọc" : "Chưa có việc"}</p>
              {canCreate && !filtering ? (
                <button
                  type="button"
                  onClick={() => onCreateTask(column.id)}
                  className="inline-flex min-h-9 items-center gap-1.5 rounded-[var(--radius-md)] px-3 font-display text-[14px] font-bold text-mint-600 hover:bg-mint-50 focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100"
                >
                  <PlusIcon aria-hidden className="size-4" />
                  Thêm việc
                </button>
              ) : null}
            </div>
          ) : (
            tasks.map((task) => (
              <TaskCard
                key={task.id}
                task={task}
                columns={columns}
                assigneeName={assigneeNameFor(task.assigneeId)}
                onOpen={() => onOpenTask(task.id)}
                onMove={(columnId) => onMoveTask(task.id, columnId)}
                onQuickDone={() => onQuickDone(task.id)}
                moveDisabled={!canMove}
                reducedMotion={reducedMotion}
                getTaskProps={(extra) => getTaskProps(task.id, extra)}
              />
            ))
          )}
          {/* Drop zone below the last card so "into this column" is always reachable. */}
          <div aria-hidden className="min-h-12 flex-1" />
        </div>
      </SortableContext>
    </div>
  );
}
