import { cn } from "@/lib/utils";

import type { AppTask } from "../hooks/use-tasks-data-source";
import { TaskCardBody } from "./task-card";
import { taskCardSurfaceClassName } from "./task-card-styles";

export interface TaskCardPreviewProps {
  task: AppTask;
  assigneeName: string | null;
}

/**
 * Static copy of a card for `<DragOverlay>`: same surface and body, none of
 * the interactive wiring (no lib props, no sortable, no move menu), so the
 * overlay never competes with the source card for focus or events.
 */
export function TaskCardPreview({ task, assigneeName }: TaskCardPreviewProps) {
  return (
    <div
      aria-hidden
      className={cn(
        taskCardSurfaceClassName(task),
        "cursor-grabbing shadow-lg ring-2 ring-mint-200",
      )}
    >
      <TaskCardBody task={task} assigneeName={assigneeName} />
    </div>
  );
}
