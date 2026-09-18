import { useDroppable } from "@dnd-kit/core";
import { SortableContext, verticalListSortingStrategy } from "@dnd-kit/sortable";
import { CheckIcon, ChevronLeftIcon, PlusIcon } from "lucide-react";
import type { ComponentProps } from "react";

import type { ColumnId, TaskId, TaskPropsExtra } from "@/lib/kanban";
import { cn } from "@/lib/utils";

import type { BoardDndData } from "../hooks/use-board-dnd";
import type { AppColumn, AppTask } from "../hooks/use-tasks-data-source";
import { COLUMN_DOT, COLUMN_TINT } from "../lib/column-colors";
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
        "flex min-h-0 flex-col rounded-[16px] border border-line-200 p-2.5 transition-colors",
        COLUMN_TINT[column.color],
        isDropTarget && "border-mint-400 ring-2 ring-mint-100",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2 px-1 pb-2">
        <div className="flex min-w-0 items-center gap-1.5">
          <span
            aria-hidden
            className={cn("size-2.5 shrink-0 rounded-full", COLUMN_DOT[column.color])}
          />
          {column.isDone ? (
            <CheckIcon aria-hidden className="size-4 shrink-0 text-mint-600" />
          ) : null}
          <p className="truncate text-[13.5px] font-extrabold text-ink-900">{column.name}</p>
          <span className="shrink-0 rounded-full bg-white px-1.5 py-0.5 text-[11px] font-bold text-ink-500">
            {tasks.length}
          </span>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {canCreate ? (
            <button
              type="button"
              onClick={() => onCreateTask(column.id)}
              aria-label={`Thêm việc vào ${column.name}`}
              className="inline-flex size-10 shrink-0 items-center justify-center rounded-full text-ink-500 hover:bg-white hover:text-mint-600"
            >
              <PlusIcon aria-hidden className="size-4" />
            </button>
          ) : null}
          {onCollapse ? (
            <button
              type="button"
              onClick={onCollapse}
              aria-expanded
              aria-label={`Thu gọn cột ${column.name}`}
              className="hidden size-10 shrink-0 items-center justify-center rounded-full text-ink-500 hover:bg-white hover:text-mint-600 min-[900px]:inline-flex"
            >
              <ChevronLeftIcon aria-hidden className="size-4" />
            </button>
          ) : null}
        </div>
      </div>
      <SortableContext items={tasks.map((task) => task.id)} strategy={verticalListSortingStrategy}>
        <div
          {...getColumnProps()}
          ref={setNodeRef}
          className="flex min-h-[60px] flex-1 flex-col gap-2 overflow-y-auto"
        >
          {tasks.length === 0 ? (
            <div className="flex min-h-[96px] flex-col items-center justify-center gap-2 rounded-[12px] border-2 border-dashed border-line-200 px-2 py-3">
              <p className="text-center text-[12px] text-ink-500">
                {filtering ? "Không có việc khớp lọc" : "Chưa có việc"}
              </p>
              {canCreate && !filtering ? (
                <button
                  type="button"
                  onClick={() => onCreateTask(column.id)}
                  className="rounded-md px-2 py-1 text-[12px] font-bold text-mint-600 hover:bg-white"
                >
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
