import type { ColumnId, KanbanBoard, KanbanColumn, KanbanTask, TaskId } from "./types";

/**
 * Port the app implements to connect the headless core to a real backend.
 * The lib only ever calls these 9 methods — it knows nothing about HTTP,
 * caching, or optimistic updates (see README.md "Ports"). Every method
 * settles to a value or rejects with a `KanbanError` (see errors.ts).
 */
export interface KanbanDataSource<TTask extends KanbanTask> {
  // Function-property syntax (not method shorthand) so implementations and
  // test fakes are plain objects of closures with no `this` binding — this
  // also keeps `@typescript-eslint/unbound-method` from flagging bare
  // references like `expect(dataSource.moveTask).toHaveBeenCalledWith(...)`.
  loadBoard: () => Promise<KanbanBoard<TTask>>;

  createColumn: (input: { name: string; isDone: boolean }) => Promise<KanbanColumn>;
  updateColumn: (
    columnId: ColumnId,
    patch: { name?: string; isDone?: boolean },
  ) => Promise<KanbanColumn>;
  reorderColumns: (order: ColumnId[]) => Promise<void>;
  deleteColumn: (columnId: ColumnId, moveTasksTo: ColumnId) => Promise<void>;

  createTask: (input: Omit<TTask, "id">) => Promise<TTask>;
  updateTask: (taskId: TaskId, patch: Partial<Omit<TTask, "id">>) => Promise<TTask>;
  moveTask: (taskId: TaskId, columnId: ColumnId, position: number) => Promise<TTask>;
  deleteTask: (taskId: TaskId) => Promise<void>;
}
