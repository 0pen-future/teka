import type { HvBadgeVariant } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { AppTask } from "../hooks/use-tasks-data-source";
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

/**
 * Surface classes shared by the live card and its drag-overlay preview.
 * The wide left padding leaves room for the quick-done button that hangs
 * over the card's leading edge; overdue and "mine" are no longer signalled
 * by a border stripe but by the title dot and the assignee avatar.
 */
export function taskCardSurfaceClassName(task: AppTask): string {
  const done = task.completedAt !== null;
  return cn(
    "relative rounded-[12px] bg-white pb-2.5 pl-[18px] pr-2.5 pt-3 shadow-xs",
    "transition-[transform,box-shadow] duration-[var(--dur-fast)] ease-[var(--ease-out)]",
    done && "opacity-[0.72]",
  );
}
