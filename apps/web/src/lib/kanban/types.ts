/**
 * Domain types for the headless kanban core. This module has zero
 * dependencies (not even `react`) — see README.md "Boundary rules".
 */

/** Branded string ID for a column. Construct only via {@link asColumnId}. */
export type ColumnId = string & { readonly __brand: "ColumnId" };

/** Branded string ID for a task. Construct only via {@link asTaskId}. */
export type TaskId = string & { readonly __brand: "TaskId" };

/**
 * Converts a raw string (e.g. a UUID parsed from an API response) into a
 * {@link ColumnId}. Centralizes the cast so adapters do one conversion at the
 * response boundary instead of scattering `as ColumnId` across the app.
 */
export function asColumnId(value: string): ColumnId {
  return value as ColumnId;
}

/** Converts a raw string into a {@link TaskId}. See {@link asColumnId}. */
export function asTaskId(value: string): TaskId {
  return value as TaskId;
}

export interface KanbanColumn {
  id: ColumnId;
  name: string;
  /** Zero-based rank used to order columns left-to-right. */
  order: number;
  /** Whether tasks moved into this column count as completed. */
  isDone: boolean;
}

export interface KanbanTask {
  id: TaskId;
  columnId: ColumnId;
  /** Zero-based rank used to order tasks top-to-bottom within a column. */
  position: number;
  /** ISO-8601 timestamp. Optional tie-break for {@link selectTasksByColumn}. */
  createdAt?: string;
}

export interface KanbanBoard<TTask extends KanbanTask> {
  columns: KanbanColumn[];
  tasks: TTask[];
}
