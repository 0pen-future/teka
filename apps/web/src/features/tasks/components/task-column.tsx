import { CheckIcon, PlusIcon } from "lucide-react";
import type { ComponentProps } from "react";

import type { ColumnId, KanbanColumn, TaskId } from "@/lib/kanban";
import { cn } from "@/lib/utils";

import type { AppTask } from "../hooks/use-tasks-data-source";
import { TaskCard } from "./task-card";

export interface TaskColumnProps {
  column: KanbanColumn;
  tasks: AppTask[];
  columns: KanbanColumn[];
  assigneeNameFor: (assigneeId: string | null) => string | null;
  currentUserId: string | undefined;
  canCreate: boolean;
  canMove: boolean;
  onOpenTask: (taskId: TaskId) => void;
  onCreateTask: (columnId: ColumnId) => void;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => void;
  getColumnProps: () => ComponentProps<"div">;
  getTaskProps: (taskId: TaskId) => ComponentProps<"div">;
  className?: string;
}

/** One kanban column: header (name, count, done marker) plus its task list. */
export function TaskColumn({
  column,
  tasks,
  columns,
  assigneeNameFor,
  currentUserId,
  canCreate,
  canMove,
  onOpenTask,
  onCreateTask,
  onMoveTask,
  getColumnProps,
  getTaskProps,
  className,
}: TaskColumnProps) {
  return (
    <div
      className={cn(
        "flex min-h-0 flex-col rounded-[16px] border border-line-200 p-2.5",
        column.isDone ? "bg-mint-50" : "bg-cream-100",
        className,
      )}
    >
      <div className="flex items-center justify-between gap-2 px-1 pb-2">
        <div className="flex min-w-0 items-center gap-1.5">
          {column.isDone ? (
            <CheckIcon aria-hidden className="size-4 shrink-0 text-mint-600" />
          ) : null}
          <p className="truncate text-[13.5px] font-extrabold text-ink-900">{column.name}</p>
          <span className="shrink-0 rounded-full bg-white px-1.5 py-0.5 text-[11px] font-bold text-ink-400">
            {tasks.length}
          </span>
        </div>
        {canCreate ? (
          <button
            type="button"
            onClick={() => onCreateTask(column.id)}
            aria-label={`Thêm việc vào ${column.name}`}
            className="inline-flex size-6 shrink-0 items-center justify-center rounded-md text-ink-400 hover:bg-white hover:text-mint-600"
          >
            <PlusIcon aria-hidden className="size-4" />
          </button>
        ) : null}
      </div>
      <div
        {...getColumnProps()}
        className="flex min-h-[60px] flex-1 flex-col gap-2 overflow-y-auto"
      >
        {tasks.length === 0 ? (
          <p className="px-1 py-3 text-center text-[12px] text-ink-400">Chưa có việc</p>
        ) : (
          tasks.map((task) => (
            <TaskCard
              key={task.id}
              task={task}
              columns={columns}
              assigneeName={assigneeNameFor(task.assigneeId)}
              isAssignedToMe={currentUserId !== undefined && task.assigneeId === currentUserId}
              onOpen={() => onOpenTask(task.id)}
              onMove={(columnId) => onMoveTask(task.id, columnId)}
              moveDisabled={!canMove}
              taskProps={getTaskProps(task.id)}
            />
          ))
        )}
      </div>
    </div>
  );
}
