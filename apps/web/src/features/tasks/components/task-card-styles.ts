import type { HvBadgeVariant } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { AppTask } from "../hooks/use-tasks-data-source";
import { dueState } from "../lib/due-state";
import type { TaskPriority } from "../schemas/task-schemas";

export const PRIORITY_LABELS: Record<TaskPriority, string> = {
  none: "Không",
  low: "Thấp",
  medium: "Trung bình",
  high: "Cao",
};

export const PRIORITY_VARIANTS: Record<TaskPriority, HvBadgeVariant> = {
  none: "neutral",
  low: "info",
  medium: "warning",
  high: "danger",
};

/** Surface classes shared by the live card and its drag-overlay preview. */
export function taskCardSurfaceClassName(task: AppTask, isAssignedToMe: boolean): string {
  const done = task.completedAt !== null;
  const overdue = dueState(task).kind === "overdue";
  return cn(
    "rounded-[12px] border-l-4 bg-white p-3 shadow-sm",
    done && "opacity-[0.72]",
    done
      ? "border-l-line-200"
      : overdue
        ? "border-l-coral-400"
        : isAssignedToMe
          ? "border-l-mint-400"
          : "border-l-line-200",
  );
}
