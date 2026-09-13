import { useState } from "react";

import { HvSegmented, HvSelect } from "@/components/hv";
import type { ColumnId, KanbanColumn, TaskId } from "@/lib/kanban";

import type { AppTask } from "../hooks/use-tasks-data-source";
import { TaskColumn } from "./task-column";

/** Below this column count, a segmented tab strip fits; above it, a select avoids overflow. */
const SEGMENTED_MAX_COLUMNS = 4;

export interface BoardMobileProps {
  columns: KanbanColumn[];
  tasksByColumn: Map<ColumnId, AppTask[]>;
  assigneeNameFor: (assigneeId: string | null) => string | null;
  currentUserId: string | undefined;
  canCreate: boolean;
  canMove: boolean;
  onOpenTask: (taskId: TaskId) => void;
  onCreateTask: (columnId: ColumnId) => void;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => void;
  getColumnProps: (columnId: ColumnId) => Record<string, unknown>;
  getTaskProps: (taskId: TaskId) => Record<string, unknown>;
}

/** Renders one column at a time, switched via a segmented strip (≤4 columns) or a select (more). */
export function BoardMobile({
  columns,
  tasksByColumn,
  assigneeNameFor,
  currentUserId,
  canCreate,
  canMove,
  onOpenTask,
  onCreateTask,
  onMoveTask,
  getColumnProps,
  getTaskProps,
}: BoardMobileProps) {
  const sorted = [...columns].sort((a, b) => a.order - b.order);
  const [activeColumnId, setActiveColumnId] = useState<ColumnId | undefined>(sorted[0]?.id);

  // A column may be renamed, added, or removed under the current selection
  // (settings modal, another session): falling back to the first column here
  // (rather than syncing `activeColumnId` itself in an effect) keeps a stale
  // id from ever rendering an empty panel.
  const active = sorted.find((column) => column.id === activeColumnId) ?? sorted[0];

  if (!active) {
    return <p className="py-6 text-center text-[13px] text-ink-400">Chưa có cột nào.</p>;
  }

  return (
    <div className="flex flex-col gap-3">
      {sorted.length <= SEGMENTED_MAX_COLUMNS ? (
        <HvSegmented
          aria-label="Chọn cột"
          variant="tabs"
          idBase="task-column-tab"
          block
          value={active.id}
          onValueChange={setActiveColumnId}
          options={sorted.map((column) => ({
            value: column.id,
            label: `${column.name} (${tasksByColumn.get(column.id)?.length ?? 0})`,
          }))}
        />
      ) : (
        <HvSelect
          sheetTitle="Chọn cột"
          aria-label="Chọn cột"
          value={active.id}
          onValueChange={(value) => setActiveColumnId(value as ColumnId)}
          options={sorted.map((column) => ({
            value: column.id,
            label: column.name,
            meta: String(tasksByColumn.get(column.id)?.length ?? 0),
          }))}
        />
      )}
      <TaskColumn
        column={active}
        tasks={tasksByColumn.get(active.id) ?? []}
        columns={columns}
        assigneeNameFor={assigneeNameFor}
        currentUserId={currentUserId}
        canCreate={canCreate}
        canMove={canMove}
        onOpenTask={onOpenTask}
        onCreateTask={onCreateTask}
        onMoveTask={onMoveTask}
        getColumnProps={() => getColumnProps(active.id)}
        getTaskProps={getTaskProps}
      />
    </div>
  );
}
