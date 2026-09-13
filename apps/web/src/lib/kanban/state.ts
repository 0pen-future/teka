import type { ColumnId, KanbanBoard, KanbanColumn, KanbanTask, TaskId } from "./types";

/**
 * Discriminated-union actions for {@link kanbanReducer}. This is a pure
 * utility, not a `useReducer` state — the app (Teka: a TanStack Query cache)
 * owns the board and uses this reducer to compute optimistic state, e.g.
 * `setQueryData(key, (board) => kanbanReducer(board, action))`. See
 * README.md "Board is a prop".
 */
export type KanbanAction<TTask extends KanbanTask> =
  | { type: "board/replaced"; board: KanbanBoard<TTask> }
  | { type: "tasks/moved"; taskId: TaskId; columnId: ColumnId; position: number }
  | { type: "tasks/upserted"; task: TTask }
  | { type: "tasks/removed"; taskId: TaskId }
  | { type: "columns/reordered"; order: ColumnId[] }
  | { type: "columns/upserted"; column: KanbanColumn }
  | { type: "columns/removed"; columnId: ColumnId };

/**
 * Pure transform of one {@link KanbanBoard} into a new one. Never mutates
 * `board`. An action of an unrecognized shape returns the same `board`
 * reference unchanged (the exhaustive `switch` below makes the compiler
 * reject a new `KanbanAction` variant that forgets a `case`).
 */
export function kanbanReducer<TTask extends KanbanTask>(
  board: KanbanBoard<TTask>,
  action: KanbanAction<TTask>,
): KanbanBoard<TTask> {
  switch (action.type) {
    case "board/replaced": {
      return action.board;
    }

    case "tasks/moved": {
      return {
        ...board,
        tasks: board.tasks.map((task) =>
          task.id === action.taskId
            ? { ...task, columnId: action.columnId, position: action.position }
            : task,
        ),
      };
    }

    case "tasks/upserted": {
      const exists = board.tasks.some((task) => task.id === action.task.id);
      return {
        ...board,
        tasks: exists
          ? board.tasks.map((task) => (task.id === action.task.id ? action.task : task))
          : [...board.tasks, action.task],
      };
    }

    case "tasks/removed": {
      return { ...board, tasks: board.tasks.filter((task) => task.id !== action.taskId) };
    }

    case "columns/reordered": {
      const orderIndex = new Map(action.order.map((id, index) => [id, index]));
      return {
        ...board,
        columns: board.columns.map((column) => {
          const nextOrder = orderIndex.get(column.id);
          return nextOrder === undefined ? column : { ...column, order: nextOrder };
        }),
      };
    }

    case "columns/upserted": {
      const exists = board.columns.some((column) => column.id === action.column.id);
      return {
        ...board,
        columns: exists
          ? board.columns.map((column) => (column.id === action.column.id ? action.column : column))
          : [...board.columns, action.column],
      };
    }

    case "columns/removed": {
      return { ...board, columns: board.columns.filter((column) => column.id !== action.columnId) };
    }

    default: {
      const exhaustiveCheck: never = action;
      void exhaustiveCheck;
      return board;
    }
  }
}
