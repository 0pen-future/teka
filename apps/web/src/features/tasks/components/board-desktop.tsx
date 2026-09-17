import { DndContext, DragOverlay } from "@dnd-kit/core";

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

  return (
    <DndContext {...dnd.contextProps}>
      <div className="flex gap-3 overflow-x-auto pb-2">
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
            className="min-w-[230px] w-[280px] shrink-0"
          />
        ))}
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
