import { useMutation, useQueryClient, type UseMutationResult } from "@tanstack/react-query";

import {
  afterTaskIdAt,
  asColumnId,
  asTaskId,
  kanbanReducer,
  optimisticPositionFor,
  selectTasksByColumn,
  type ColumnId,
  type KanbanAction,
  type KanbanBoard,
  type KanbanColumn,
  type KanbanDataSource,
  type KanbanError,
  type KanbanTask,
  type TaskId,
} from "@/lib/kanban";

import {
  createColumn as createColumnApi,
  deleteColumn as deleteColumnApi,
  reorderColumns as reorderColumnsApi,
  updateColumn as updateColumnApi,
} from "../api/task-columns-api";
import {
  createTask as createTaskApi,
  deleteTask as deleteTaskApi,
  getBoard,
  moveTask as moveTaskApi,
  restoreTask as restoreTaskApi,
  updateTask as updateTaskApi,
  type GetBoardParams,
} from "../api/tasks-api";
import { mapApiError } from "../lib/map-api-error";
import type {
  BoardCounts,
  BoardResponse,
  Task,
  TaskColumn,
  TaskPriority,
} from "../schemas/task-schemas";
import { tasksKeys } from "./tasks-keys";

/* eslint-disable @typescript-eslint/only-throw-error -- every mutation below
 * deliberately throws a plain `KanbanError` union value (see the headless
 * lib's `KanbanError` type), not an `Error` instance: it's the mutations'
 * declared `TError` generic, consumed via `isKanbanError`/`.kind` narrowing
 * by callers (form error mapping, toasts, `.rejects.toMatchObject` in
 * tests), exactly like a typed Result/Either error channel. */

/**
 * The board-rendering shape: the headless lib's `KanbanTask` fields plus the
 * app-specific task fields the lib doesn't know about (title, assignee, ...).
 */
export interface AppTask extends KanbanTask {
  title: string;
  description: string;
  priority: TaskPriority;
  dueOn: string | null;
  assigneeId: string | null;
  createdBy: string;
  completedAt: string | null;
  updatedAt: string;
}

export interface CreateTaskVariables {
  title: string;
  description?: string;
  columnId: string;
  assigneeId?: string | null;
  priority?: TaskPriority;
  dueOn?: string | null;
}

/**
 * `afterTaskId`/`position` are the same drop target in the two vocabularies
 * the move touches: the API wants "land after this task" (`null` = top of
 * column), the optimistic reducer wants a sortable `position` between the
 * neighbours. `dataSource.moveTask` derives both from a target index via the
 * lib's position helpers; callers with a different source of truth (none
 * today) can supply them directly.
 */
export interface MoveTaskVariables {
  taskId: TaskId;
  columnId: ColumnId;
  afterTaskId: TaskId | null;
  position: number;
}

export interface UpdateTaskVariables {
  taskId: TaskId;
  title?: string;
  description?: string;
  assigneeId?: string | null;
  priority?: TaskPriority;
  dueOn?: string | null;
}

export interface CreateColumnVariables {
  name: string;
  isDone: boolean;
}

export interface UpdateColumnVariables {
  columnId: ColumnId;
  name?: string;
  isDone?: boolean;
}

export interface DeleteColumnVariables {
  columnId: ColumnId;
  moveTasksTo: ColumnId;
}

/**
 * All 9 board mutations share this scope so TanStack Query serializes them —
 * a second mutation queues behind the first instead of racing its optimistic
 * `setQueryData` write against the other's. There is no optimistic
 * snapshot/rollback: `onSettled` always invalidates the board query, so a
 * failed (or succeeded) mutation converges on the server's truth via
 * refetch instead of trying to undo a partial local edit.
 */
const KANBAN_MUTATION_SCOPE = { id: "kanban-board" };

function toAppTask(task: Task): AppTask {
  return {
    id: asTaskId(task.id),
    columnId: asColumnId(task.column_id),
    position: task.position,
    createdAt: task.created_at,
    title: task.title,
    description: task.description,
    priority: task.priority,
    dueOn: task.due_on,
    assigneeId: task.assignee_id,
    createdBy: task.created_by,
    completedAt: task.completed_at,
    updatedAt: task.updated_at,
  };
}

function toAppColumn(column: TaskColumn): KanbanColumn {
  return {
    id: asColumnId(column.id),
    name: column.name,
    order: column.position,
    isDone: column.is_done,
  };
}

function toAppBoard(response: BoardResponse): KanbanBoard<AppTask> {
  return {
    columns: response.columns.map(toAppColumn),
    tasks: response.columns.flatMap((column) => column.tasks.map(toAppTask)),
  };
}

/**
 * The shape cached at `tasksKeys.board(params)`. `board` is the
 * reducer-friendly `KanbanBoard<AppTask>` the mutations below optimistically
 * rewrite via `kanbanReducer`; `counts` is carried alongside it (not inside
 * it, since the headless lib's `KanbanBoard` type has no room for
 * app-specific fields) and stays as of the last fetch — no mutation below
 * rewrites it optimistically, so it settles once `onSettled`'s board
 * invalidation refetches.
 */
export interface BoardQueryData {
  counts: BoardCounts;
  board: KanbanBoard<AppTask>;
}

/** Exported so `use-task-board.ts`'s query and this file's mutations agree on one cache shape. */
export function toBoardQueryData(response: BoardResponse): BoardQueryData {
  return { counts: response.counts, board: toAppBoard(response) };
}

export interface UseTasksDataSourceResult {
  dataSource: KanbanDataSource<AppTask>;
  createTaskMutation: UseMutationResult<AppTask, KanbanError, CreateTaskVariables>;
  updateTaskMutation: UseMutationResult<AppTask, KanbanError, UpdateTaskVariables>;
  moveTaskMutation: UseMutationResult<AppTask, KanbanError, MoveTaskVariables>;
  deleteTaskMutation: UseMutationResult<void, KanbanError, TaskId>;
  restoreTaskMutation: UseMutationResult<AppTask, KanbanError, TaskId>;
  createColumnMutation: UseMutationResult<KanbanColumn, KanbanError, CreateColumnVariables>;
  updateColumnMutation: UseMutationResult<KanbanColumn, KanbanError, UpdateColumnVariables>;
  reorderColumnsMutation: UseMutationResult<void, KanbanError, ColumnId[]>;
  deleteColumnMutation: UseMutationResult<void, KanbanError, DeleteColumnVariables>;
}

/**
 * Implements `KanbanDataSource<AppTask>` on top of TanStack Query. Every
 * mutation optimistically applies `kanbanReducer` to the board query's cache
 * in `onMutate`, and `onSettled` always invalidates the board query (see
 * `src/lib/kanban/README.md` "Board is a prop" for why there is no
 * snapshot/rollback: refetch is the recovery path for both success and
 * failure).
 *
 * The returned mutation objects are also exposed directly (not just wrapped
 * inside `dataSource`) so components that aren't the headless board itself —
 * the task form modal, the board settings modal, the delete-column dialog —
 * can read `isPending`/`error` and call `.mutate`/`.mutateAsync` with a
 * friendlier variables shape than the lib's `KanbanDataSource` port requires.
 */
export function useTasksDataSource(params: GetBoardParams): UseTasksDataSourceResult {
  const queryClient = useQueryClient();
  const boardKey = tasksKeys.board(params);

  const applyOptimistic = (action: KanbanAction<AppTask>): void => {
    queryClient.setQueryData<BoardQueryData>(boardKey, (data) =>
      data ? { ...data, board: kanbanReducer(data.board, action) } : data,
    );
  };

  const invalidateBoard = (): void => {
    void queryClient.invalidateQueries({ queryKey: tasksKeys.boards() });
  };

  const createColumnMutation = useMutation<KanbanColumn, KanbanError, CreateColumnVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (variables) => {
      try {
        const column = await createColumnApi({ name: variables.name, is_done: variables.isDone });
        return toAppColumn(column);
      } catch (error) {
        throw mapApiError(error, "column");
      }
    },
    onSuccess: (column) => {
      applyOptimistic({ type: "columns/upserted", column });
    },
    onSettled: invalidateBoard,
  });

  const updateColumnMutation = useMutation<KanbanColumn, KanbanError, UpdateColumnVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (variables) => {
      try {
        const column = await updateColumnApi(variables.columnId, {
          name: variables.name,
          is_done: variables.isDone,
        });
        return toAppColumn(column);
      } catch (error) {
        throw mapApiError(error, "column");
      }
    },
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: tasksKeys.boards() });
      const current = queryClient.getQueryData<BoardQueryData>(boardKey);
      const existing = current?.board.columns.find((column) => column.id === variables.columnId);
      if (existing) {
        applyOptimistic({
          type: "columns/upserted",
          column: {
            ...existing,
            name: variables.name ?? existing.name,
            isDone: variables.isDone ?? existing.isDone,
          },
        });
      }
    },
    onSuccess: (column) => {
      applyOptimistic({ type: "columns/upserted", column });
    },
    onSettled: invalidateBoard,
  });

  const reorderColumnsMutation = useMutation<void, KanbanError, ColumnId[]>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (order) => {
      try {
        await reorderColumnsApi(order);
      } catch (error) {
        const mapped = mapApiError(error, "column");
        // A stale order list surfaces as either 409 or 422 from this
        // endpoint — both mean "the board changed under you", not "this
        // field is invalid", so both collapse to `conflict` here rather
        // than `validation`.
        throw mapped.kind === "validation" ? { kind: "conflict" as const } : mapped;
      }
    },
    onMutate: async (order) => {
      await queryClient.cancelQueries({ queryKey: tasksKeys.boards() });
      applyOptimistic({ type: "columns/reordered", order });
    },
    onSettled: invalidateBoard,
  });

  const deleteColumnMutation = useMutation<void, KanbanError, DeleteColumnVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (variables) => {
      try {
        await deleteColumnApi(variables.columnId, variables.moveTasksTo);
      } catch (error) {
        throw mapApiError(error, "column");
      }
    },
    onMutate: async (variables) => {
      await queryClient.cancelQueries({ queryKey: tasksKeys.boards() });
      // Move the deleted column's tasks into the destination column first —
      // dispatching `columns/removed` alone would orphan them (their
      // `columnId` would no longer match any column in `board.columns`),
      // making them disappear from every column until the refetch below
      // replaces this optimistic state with the server's.
      const current = queryClient.getQueryData<BoardQueryData>(boardKey);
      const orphaned = current?.board.tasks.filter((task) => task.columnId === variables.columnId);
      orphaned?.forEach((task) => {
        applyOptimistic({
          type: "tasks/moved",
          taskId: task.id,
          columnId: variables.moveTasksTo,
          position: 0,
        });
      });
      applyOptimistic({ type: "columns/removed", columnId: variables.columnId });
    },
    onSettled: invalidateBoard,
  });

  const createTaskMutation = useMutation<AppTask, KanbanError, CreateTaskVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (variables) => {
      try {
        const task = await createTaskApi({
          title: variables.title,
          description: variables.description,
          column_id: variables.columnId,
          assignee_id: variables.assigneeId ?? undefined,
          priority: variables.priority,
          due_on: variables.dueOn ?? undefined,
        });
        return toAppTask(task);
      } catch (error) {
        throw mapApiError(error, "task");
      }
    },
    onSuccess: (task) => {
      applyOptimistic({ type: "tasks/upserted", task });
    },
    onSettled: invalidateBoard,
  });

  const updateTaskMutation = useMutation<AppTask, KanbanError, UpdateTaskVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (variables) => {
      try {
        const task = await updateTaskApi(variables.taskId, {
          title: variables.title,
          description: variables.description,
          assignee_id: variables.assigneeId,
          priority: variables.priority,
          due_on: variables.dueOn,
        });
        return toAppTask(task);
      } catch (error) {
        throw mapApiError(error, "task");
      }
    },
    onSuccess: (task) => {
      applyOptimistic({ type: "tasks/upserted", task });
    },
    onSettled: invalidateBoard,
  });

  const moveTaskMutation = useMutation<AppTask, KanbanError, MoveTaskVariables>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async ({ taskId, columnId, afterTaskId }) => {
      try {
        const task = await moveTaskApi(taskId, {
          column_id: columnId,
          after_task_id: afterTaskId ?? null,
        });
        return toAppTask(task);
      } catch (error) {
        throw mapApiError(error, "task");
      }
    },
    onMutate: async ({ taskId, columnId, position }) => {
      await queryClient.cancelQueries({ queryKey: tasksKeys.boards() });
      // `position` is a client-side midpoint between the drop slot's
      // neighbours: the server picks its own value (and may renormalize the
      // column), but the optimistic order is the one the user just saw.
      applyOptimistic({ type: "tasks/moved", taskId, columnId, position });
    },
    onSuccess: (task) => {
      applyOptimistic({ type: "tasks/upserted", task });
    },
    onSettled: invalidateBoard,
  });

  const deleteTaskMutation = useMutation<void, KanbanError, TaskId>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (taskId) => {
      try {
        await deleteTaskApi(taskId);
      } catch (error) {
        throw mapApiError(error, "task");
      }
    },
    onMutate: async (taskId) => {
      await queryClient.cancelQueries({ queryKey: tasksKeys.boards() });
      applyOptimistic({ type: "tasks/removed", taskId });
    },
    onSettled: invalidateBoard,
  });

  // No optimistic apply in `onMutate`: the client no longer holds the
  // deleted row to rebuild it from, so the restored task only enters the
  // board once the server confirms it exists again.
  const restoreTaskMutation = useMutation<AppTask, KanbanError, TaskId>({
    scope: KANBAN_MUTATION_SCOPE,
    mutationFn: async (taskId) => {
      try {
        const task = await restoreTaskApi(taskId);
        return toAppTask(task);
      } catch (error) {
        throw mapApiError(error, "task");
      }
    },
    onSuccess: (task) => {
      applyOptimistic({ type: "tasks/upserted", task });
    },
    onSettled: invalidateBoard,
  });

  // Not memoized: the page passes `params` as a fresh object literal every
  // render, so a `useMemo` here would recompute every render anyway. Nothing
  // downstream needs `dataSource` to keep a stable identity — `useKanban`'s
  // `tasksByColumn` depends only on `board`, not on `dataSource`.
  const dataSource: KanbanDataSource<AppTask> = {
    loadBoard: async () => toAppBoard(await getBoard(params)),
    createColumn: (input) => createColumnMutation.mutateAsync(input),
    updateColumn: (columnId, patch) => updateColumnMutation.mutateAsync({ columnId, ...patch }),
    reorderColumns: (order) => reorderColumnsMutation.mutateAsync(order),
    deleteColumn: (columnId, moveTasksTo) =>
      deleteColumnMutation.mutateAsync({ columnId, moveTasksTo }),
    createTask: (input) =>
      createTaskMutation.mutateAsync({
        title: input.title,
        description: input.description,
        columnId: input.columnId,
        assigneeId: input.assigneeId,
        priority: input.priority,
        dueOn: input.dueOn,
      }),
    updateTask: (taskId, patch) =>
      updateTaskMutation.mutateAsync({
        taskId,
        title: patch.title,
        description: patch.description,
        assigneeId: patch.assigneeId,
        priority: patch.priority,
        dueOn: patch.dueOn,
      }),
    // The port's `position` is a target *index* in the destination column
    // (0 = top; the lib's `[`/`]` shortcut and the move menu both send 0,
    // drag-and-drop sends the drop slot). Translate it against the cached
    // board into the API's `after_task_id` and a reducer-friendly midpoint
    // position; an unknown/empty column degrades to "top of column".
    moveTask: (taskId, columnId, index) => {
      const cachedBoard = queryClient.getQueryData<BoardQueryData>(boardKey)?.board;
      const columnTasks = cachedBoard ? (selectTasksByColumn(cachedBoard).get(columnId) ?? []) : [];
      return moveTaskMutation.mutateAsync({
        taskId,
        columnId,
        afterTaskId: afterTaskIdAt(columnTasks, index, taskId),
        position: optimisticPositionFor(columnTasks, index, taskId),
      });
    },
    deleteTask: (taskId) => deleteTaskMutation.mutateAsync(taskId),
  };

  return {
    dataSource,
    createTaskMutation,
    updateTaskMutation,
    moveTaskMutation,
    deleteTaskMutation,
    restoreTaskMutation,
    createColumnMutation,
    updateColumnMutation,
    reorderColumnsMutation,
    deleteColumnMutation,
  };
}
