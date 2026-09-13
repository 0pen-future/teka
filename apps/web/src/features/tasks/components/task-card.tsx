import type { ComponentProps } from "react";

import { HvBadge } from "@/components/hv";
import type { ColumnId, KanbanColumn } from "@/lib/kanban";
import { cn, formatDateTime, formatDayMonth } from "@/lib/utils";

import type { AppTask } from "../hooks/use-tasks-data-source";
import type { TaskPriority } from "../schemas/task-schemas";
import { MoveMenu } from "./move-menu";

const PRIORITY_LABELS: Record<TaskPriority, string> = {
  none: "Không",
  low: "Thấp",
  medium: "Trung bình",
  high: "Cao",
};

const PRIORITY_VARIANTS: Record<TaskPriority, ComponentProps<typeof HvBadge>["variant"]> = {
  none: "neutral",
  low: "info",
  medium: "warning",
  high: "danger",
};

function isOverdue(task: AppTask): boolean {
  if (task.completedAt || !task.dueOn) {
    return false;
  }
  const today = new Date().toISOString().slice(0, 10);
  return task.dueOn < today;
}

export interface TaskCardProps {
  task: AppTask;
  columns: KanbanColumn[];
  assigneeName: string | null;
  isAssignedToMe: boolean;
  onOpen: () => void;
  onMove: (columnId: ColumnId) => void;
  moveDisabled?: boolean;
  /** Merges the lib's roving-tabindex / focus-ref props onto the card. */
  taskProps: ComponentProps<"div">;
}

/**
 * `xong dd/mm` uses `formatDateTime` (completedAt is an RFC3339 instant, not
 * a bare DATE) and keeps only the leading `dd/MM` slice, since the exact time
 * of completion doesn't matter here.
 */
export function TaskCard({
  task,
  columns,
  assigneeName,
  isAssignedToMe,
  onOpen,
  onMove,
  moveDisabled,
  taskProps,
}: TaskCardProps) {
  const done = task.completedAt !== null;
  const overdue = isOverdue(task);

  return (
    // `taskProps` spreads the headless lib's role="option"/tabIndex/roving
    // focus wiring, which the a11y rule can't see through a spread to verify;
    // the explicit `onKeyDown` below already covers Enter/Space activation.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      {...taskProps}
      onClick={onOpen}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      className={cn(
        "cursor-pointer rounded-[12px] border-l-4 bg-white p-3 shadow-sm outline-none",
        "focus-visible:ring-4 focus-visible:ring-mint-100",
        done && "opacity-[0.72]",
        done
          ? "border-l-line-200"
          : overdue
            ? "border-l-coral-400"
            : isAssignedToMe
              ? "border-l-mint-400"
              : "border-l-line-200",
      )}
    >
      <p className="text-[13.5px] font-bold text-ink-900">{task.title}</p>
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <HvBadge size="sm" variant={PRIORITY_VARIANTS[task.priority]}>
          {PRIORITY_LABELS[task.priority]}
        </HvBadge>
        {done ? (
          <HvBadge size="sm" variant="success">
            Xong {formatDateTime(task.completedAt!).slice(0, 5)}
          </HvBadge>
        ) : task.dueOn ? (
          <HvBadge size="sm" variant={overdue ? "danger" : "neutral"}>
            Hạn {formatDayMonth(task.dueOn)}
          </HvBadge>
        ) : null}
        {assigneeName ? (
          <span className="truncate text-[11.5px] font-semibold text-ink-400">{assigneeName}</span>
        ) : null}
      </div>
      {/* Decorative click-bubbling barrier only, so "Chuyển" doesn't also open the card. */}
      <div
        role="presentation"
        className="mt-2 flex justify-end"
        onClick={(event) => event.stopPropagation()}
      >
        <MoveMenu
          currentColumnId={task.columnId}
          columns={columns}
          onMove={onMove}
          disabled={moveDisabled}
        />
      </div>
    </div>
  );
}
