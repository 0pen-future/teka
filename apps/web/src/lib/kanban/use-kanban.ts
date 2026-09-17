import { useCallback, useMemo } from "react";

import type { KanbanDataSource } from "./data-source";
import { selectTasksByColumn, type AdjacentDirection } from "./selectors";
import type { ColumnId, KanbanBoard, KanbanColumn, KanbanTask, TaskId } from "./types";
import {
  useKanbanKeyboard,
  type ColumnPropsExtra,
  type KeyboardColumnProps,
  type KeyboardTaskProps,
  type TaskPropsExtra,
  type UseKanbanKeyboardMessages,
} from "./use-kanban-keyboard";

export interface ColumnProps extends KeyboardColumnProps {
  role: "listbox";
  "aria-label": string;
}

export interface TaskProps extends KeyboardTaskProps {
  role: "option";
}

export interface UseKanbanOptions<
  TTask extends KanbanTask,
  TColumn extends KanbanColumn = KanbanColumn,
> {
  /**
   * The board to render. Owned by the app (Teka: a TanStack Query cache),
   * returned back from this hook unchanged — see README.md "Board is a
   * prop". Passing a new `board` (e.g. after `kanbanReducer` or a refetch)
   * is the only way `tasksByColumn` and the prop getters change.
   */
  board: KanbanBoard<TTask, TColumn>;
  dataSource: KanbanDataSource<TTask>;
  /** Forwarded to {@link useKanbanKeyboard}; defaults to English. */
  messages?: UseKanbanKeyboardMessages;
}

export interface UseKanbanResult<
  TTask extends KanbanTask,
  TColumn extends KanbanColumn = KanbanColumn,
> {
  board: KanbanBoard<TTask, TColumn>;
  tasksByColumn: Map<ColumnId, TTask[]>;
  moveTask: (taskId: TaskId, columnId: ColumnId, position: number) => Promise<TTask>;
  reorderColumn: (columnId: ColumnId, direction: AdjacentDirection) => Promise<void>;
  getColumnProps: (columnId: ColumnId, extra?: ColumnPropsExtra) => ColumnProps;
  getTaskProps: (taskId: TaskId, extra?: TaskPropsExtra) => TaskProps;
  activeTaskId: TaskId | undefined;
  /** Latest keyboard-triggered outcome, for the app's own live region. */
  announcement: string;
}

/**
 * Headless board hook: derives `tasksByColumn` from `board`, exposes command
 * wrappers that call `dataSource` and resolve/reject without touching
 * `board` themselves, and prop getters wired for the verified a11y contract
 * (README.md). This hook holds no copy of `board` — only ephemeral UI state
 * (roving-tabindex focus, the latest announcement), delegated to
 * {@link useKanbanKeyboard}.
 */
export function useKanban<TTask extends KanbanTask, TColumn extends KanbanColumn = KanbanColumn>({
  board,
  dataSource,
  messages,
}: UseKanbanOptions<TTask, TColumn>): UseKanbanResult<TTask, TColumn> {
  const tasksByColumn = useMemo(() => selectTasksByColumn(board), [board]);
  const keyboard = useKanbanKeyboard({ board, dataSource, messages });

  const moveTask = useCallback(
    (taskId: TaskId, columnId: ColumnId, position: number) =>
      dataSource.moveTask(taskId, columnId, position),
    [dataSource],
  );

  const reorderColumn = useCallback(
    (columnId: ColumnId, direction: AdjacentDirection): Promise<void> => {
      const sorted = [...board.columns].sort((a, b) => a.order - b.order);
      const index = sorted.findIndex((column) => column.id === columnId);
      const swapWithIndex = direction === "next" ? index + 1 : index - 1;
      if (index === -1 || swapWithIndex < 0 || swapWithIndex >= sorted.length) {
        // Column not found, or already at the board's edge: no-op.
        return Promise.resolve();
      }
      const reordered = [...sorted];
      const moved = reordered[index];
      const swappedWith = reordered[swapWithIndex];
      if (!moved || !swappedWith) {
        return Promise.resolve();
      }
      reordered[index] = swappedWith;
      reordered[swapWithIndex] = moved;
      return dataSource.reorderColumns(reordered.map((column) => column.id));
    },
    [board.columns, dataSource],
  );

  const getColumnProps = useCallback(
    (columnId: ColumnId, extra?: ColumnPropsExtra): ColumnProps => {
      const column = board.columns.find((candidate) => candidate.id === columnId);
      return {
        ...keyboard.getColumnProps(columnId, extra),
        role: "listbox",
        "aria-label": column?.name ?? columnId,
      };
    },
    [board.columns, keyboard],
  );

  const getTaskProps = useCallback(
    (taskId: TaskId, extra?: TaskPropsExtra): TaskProps => ({
      ...keyboard.getTaskProps(taskId, extra),
      role: "option",
    }),
    [keyboard],
  );

  return {
    board,
    tasksByColumn,
    moveTask,
    reorderColumn,
    getColumnProps,
    getTaskProps,
    activeTaskId: keyboard.activeTaskId,
    announcement: keyboard.announcement,
  };
}
