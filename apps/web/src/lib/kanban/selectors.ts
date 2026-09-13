import type { ColumnId, KanbanBoard, KanbanTask } from "./types";

export type AdjacentDirection = "previous" | "next";

function compareTasks<TTask extends KanbanTask>(a: TTask, b: TTask): number {
  if (a.position !== b.position) {
    return a.position - b.position;
  }
  if (a.createdAt !== undefined && b.createdAt !== undefined) {
    return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime();
  }
  // Equal position, no comparable createdAt: Array.prototype.sort is stable
  // (ES2019+), so returning 0 preserves the caller's input order.
  return 0;
}

/**
 * Groups tasks by column and sorts each group by `position`, tie-broken by
 * `createdAt`. Does not mutate `board`. Columns with no tasks still get an
 * entry with an empty array.
 */
export function selectTasksByColumn<TTask extends KanbanTask>(
  board: KanbanBoard<TTask>,
): Map<ColumnId, TTask[]> {
  const byColumn = new Map<ColumnId, TTask[]>();
  for (const column of board.columns) {
    byColumn.set(column.id, []);
  }
  for (const task of board.tasks) {
    const bucket = byColumn.get(task.columnId);
    if (bucket) {
      bucket.push(task);
    } else {
      // Task references a column not present in board.columns (stale data).
      byColumn.set(task.columnId, [task]);
    }
  }
  for (const bucket of byColumn.values()) {
    bucket.sort(compareTasks);
  }
  return byColumn;
}

/** Counts tasks per column, including zero-count entries for empty columns. */
export function selectColumnCounts<TTask extends KanbanTask>(
  board: KanbanBoard<TTask>,
): Map<ColumnId, number> {
  const counts = new Map<ColumnId, number>();
  for (const column of board.columns) {
    counts.set(column.id, 0);
  }
  for (const task of board.tasks) {
    counts.set(task.columnId, (counts.get(task.columnId) ?? 0) + 1);
  }
  return counts;
}

/**
 * Returns the ID of the column adjacent to `columnId` in the given
 * direction, ranked by `order`. Returns `undefined` at the board's edge or
 * when `columnId` is not found.
 */
export function selectAdjacentColumnId<TTask extends KanbanTask>(
  board: KanbanBoard<TTask>,
  columnId: ColumnId,
  direction: AdjacentDirection,
): ColumnId | undefined {
  const sorted = [...board.columns].sort((a, b) => a.order - b.order);
  const index = sorted.findIndex((column) => column.id === columnId);
  if (index === -1) {
    return undefined;
  }
  const targetIndex = direction === "next" ? index + 1 : index - 1;
  return sorted[targetIndex]?.id;
}
