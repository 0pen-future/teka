import { useCallback, useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";

import type { KanbanDataSource } from "./data-source";
import { selectAdjacentColumnId, selectTasksByColumn } from "./selectors";
import type { ColumnId, KanbanBoard, KanbanTask, TaskId } from "./types";

/**
 * Extra props the app wants merged into a column's element. `onKeyDown` is
 * composed with the lib's internal handler (app handler runs first; the
 * internal handler is skipped if the app calls `event.preventDefault()`).
 * Any other key is passed through unchanged.
 */
export interface ColumnPropsExtra {
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void;
  [key: string]: unknown;
}

export interface KeyboardColumnProps extends ColumnPropsExtra {
  onKeyDown: (event: KeyboardEvent<HTMLElement>) => void;
}

/**
 * Extra props for a task's element. `ref` is merged (both the app's and the
 * lib's callback ref run); the lib's `ref` is required to move real DOM
 * focus after a roving-tabindex change (see README.md "a11y contract" for
 * why plain `tabIndex` state is not sufficient on its own).
 */
export interface TaskPropsExtra {
  onKeyDown?: (event: KeyboardEvent<HTMLElement>) => void;
  ref?: (node: HTMLElement | null) => void;
  [key: string]: unknown;
}

export interface KeyboardTaskProps extends TaskPropsExtra {
  tabIndex: number;
  "aria-selected": boolean;
  ref: (node: HTMLElement | null) => void;
}

/**
 * The `[`/`]` move outcomes the hook announces via its `announcement` return
 * value. Defaults to plain English (see README.md "Locale") — pass this
 * option to have the app supply its own locale's copy instead of
 * post-processing the returned string.
 */
export interface UseKanbanKeyboardMessages {
  moved: (columnName: string) => string;
  moveFailed: string;
}

const DEFAULT_MESSAGES: UseKanbanKeyboardMessages = {
  moved: (columnName) => `Moved task to ${columnName}.`,
  moveFailed: "Could not move task.",
};

export interface UseKanbanKeyboardOptions<TTask extends KanbanTask> {
  board: KanbanBoard<TTask>;
  /** Only `moveTask` is needed — narrower dependency than the full port. */
  dataSource: Pick<KanbanDataSource<TTask>, "moveTask">;
  /** Defaults to English (see {@link UseKanbanKeyboardMessages}). */
  messages?: UseKanbanKeyboardMessages;
}

export interface UseKanbanKeyboardResult {
  activeTaskId: TaskId | undefined;
  getColumnProps: (columnId: ColumnId, extra?: ColumnPropsExtra) => KeyboardColumnProps;
  getTaskProps: (taskId: TaskId, extra?: TaskPropsExtra) => KeyboardTaskProps;
  /** Latest keyboard-triggered outcome, for the app's own live region. */
  announcement: string;
}

function callAllHandlers<E extends { defaultPrevented: boolean }>(
  first: ((event: E) => void) | undefined,
  second: (event: E) => void,
): (event: E) => void {
  return (event: E) => {
    first?.(event);
    if (event.defaultPrevented) {
      return;
    }
    second(event);
  };
}

function mergeRefs(
  extra: ((node: HTMLElement | null) => void) | undefined,
  internal: (node: HTMLElement | null) => void,
): (node: HTMLElement | null) => void {
  return (node) => {
    extra?.(node);
    internal(node);
  };
}

/**
 * Roving tabindex + keyboard interaction for a kanban board, independent of
 * data fetching. Each column is an independent single-select listbox:
 * ArrowUp/ArrowDown move focus within the column, Home/End jump to its
 * edges, and `[` / `]` move the focused task to the adjacent column via
 * `dataSource.moveTask` (no-op at the board's edge). See README.md for the
 * verified a11y contract this implements.
 *
 * Board ownership follows the same rule as {@link useKanban}: `board` is a
 * prop, this hook only holds ephemeral focus/announcement state.
 */
export function useKanbanKeyboard<TTask extends KanbanTask>({
  board,
  dataSource,
  messages = DEFAULT_MESSAGES,
}: UseKanbanKeyboardOptions<TTask>): UseKanbanKeyboardResult {
  const tasksByColumn = useMemo(() => selectTasksByColumn(board), [board]);
  const [focusedByColumn, setFocusedByColumn] = useState<Map<ColumnId, TaskId>>(new Map());
  const [activeTaskId, setActiveTaskId] = useState<TaskId | undefined>(undefined);
  const [announcement, setAnnouncement] = useState("");
  const taskElements = useRef(new Map<TaskId, HTMLElement>());
  const pendingFocusTaskId = useRef<TaskId | undefined>(undefined);
  const pendingMoveFocus = useRef<{ taskId: TaskId; columnId: ColumnId } | undefined>(undefined);

  const resolveFocusedTaskId = useCallback(
    (columnId: ColumnId): TaskId | undefined => {
      const tasks = tasksByColumn.get(columnId);
      if (!tasks || tasks.length === 0) {
        return undefined;
      }
      const explicit = focusedByColumn.get(columnId);
      if (explicit !== undefined && tasks.some((task) => task.id === explicit)) {
        return explicit;
      }
      return tasks[0]?.id;
    },
    [tasksByColumn, focusedByColumn],
  );

  const focusTask = useCallback((columnId: ColumnId, taskId: TaskId) => {
    setFocusedByColumn((previous) => {
      const next = new Map(previous);
      next.set(columnId, taskId);
      return next;
    });
    setActiveTaskId(taskId);
    pendingFocusTaskId.current = taskId;
  }, []);

  // Runs after every commit; only does work when a focus move was requested
  // this render, e.g. by an Arrow/Home/End key or a completed `[`/`]` move.
  // A dependency-free effect (rather than depending on activeTaskId) is
  // required because a `[`/`]` move keeps the same taskId but remounts its
  // DOM node under a different column, so the ref map entry changes even
  // though activeTaskId does not.
  useEffect(() => {
    const targetTaskId = pendingFocusTaskId.current;
    if (targetTaskId === undefined) {
      return;
    }
    pendingFocusTaskId.current = undefined;
    taskElements.current.get(targetTaskId)?.focus();
  });

  // After a `[`/`]` move settles, `dataSource.moveTask` has resolved but
  // `board` itself only changes once the app feeds a new one back (this
  // hook never mutates it). Re-check on every board change and follow focus
  // to the moved task's new column as soon as `board` confirms it landed
  // there — this is what keeps focus on the card instead of losing it when
  // the task remounts under a different column's DOM subtree.
  useEffect(() => {
    const pending = pendingMoveFocus.current;
    if (!pending) {
      return;
    }
    const task = board.tasks.find((candidate) => candidate.id === pending.taskId);
    if (!task || task.columnId !== pending.columnId) {
      return;
    }
    pendingMoveFocus.current = undefined;
    focusTask(pending.columnId, pending.taskId);
  }, [board, focusTask]);

  const columnName = useCallback(
    (columnId: ColumnId): string =>
      board.columns.find((column) => column.id === columnId)?.name ?? columnId,
    [board.columns],
  );

  const handleColumnKeyDown = useCallback(
    (columnId: ColumnId) =>
      (event: KeyboardEvent<HTMLElement>): void => {
        const tasks = tasksByColumn.get(columnId) ?? [];
        if (tasks.length === 0) {
          return;
        }
        const currentTaskId = resolveFocusedTaskId(columnId);
        const currentIndex = tasks.findIndex((task) => task.id === currentTaskId);

        switch (event.key) {
          case "ArrowDown": {
            event.preventDefault();
            const next = tasks[Math.min(currentIndex + 1, tasks.length - 1)];
            if (next) {
              focusTask(columnId, next.id);
            }
            return;
          }
          case "ArrowUp": {
            event.preventDefault();
            const previous = tasks[Math.max(currentIndex - 1, 0)];
            if (previous) {
              focusTask(columnId, previous.id);
            }
            return;
          }
          case "Home": {
            event.preventDefault();
            const first = tasks[0];
            if (first) {
              focusTask(columnId, first.id);
            }
            return;
          }
          case "End": {
            event.preventDefault();
            const last = tasks[tasks.length - 1];
            if (last) {
              focusTask(columnId, last.id);
            }
            return;
          }
          case "[":
          case "]": {
            event.preventDefault();
            if (currentIndex === -1) {
              return;
            }
            const direction = event.key === "]" ? "next" : "previous";
            const targetColumnId = selectAdjacentColumnId(board, columnId, direction);
            if (!targetColumnId) {
              // At the board's edge: no adjacent column, no-op.
              return;
            }
            const task = tasks[currentIndex];
            if (!task) {
              return;
            }
            const targetTasks = tasksByColumn.get(targetColumnId) ?? [];
            dataSource
              .moveTask(task.id, targetColumnId, targetTasks.length)
              .then(() => {
                // `board` still reflects the pre-move state here (this hook
                // never mutates it — see README.md "Board is a prop"), so
                // the focus-follow itself is deferred to the effect below,
                // which fires once the app feeds back a board where the
                // task has actually landed in targetColumnId.
                pendingMoveFocus.current = { taskId: task.id, columnId: targetColumnId };
                setAnnouncement(messages.moved(columnName(targetColumnId)));
              })
              .catch(() => {
                setAnnouncement(messages.moveFailed);
              });
            return;
          }
          default:
            return;
        }
      },
    [board, tasksByColumn, resolveFocusedTaskId, focusTask, dataSource, columnName, messages],
  );

  const getColumnProps = useCallback(
    (columnId: ColumnId, extra?: ColumnPropsExtra): KeyboardColumnProps => ({
      ...extra,
      onKeyDown: callAllHandlers(extra?.onKeyDown, handleColumnKeyDown(columnId)),
    }),
    [handleColumnKeyDown],
  );

  const getTaskProps = useCallback(
    (taskId: TaskId, extra?: TaskPropsExtra): KeyboardTaskProps => {
      const task = board.tasks.find((candidate) => candidate.id === taskId);
      const isFocused = task ? resolveFocusedTaskId(task.columnId) === taskId : false;
      return {
        ...extra,
        tabIndex: isFocused ? 0 : -1,
        "aria-selected": isFocused,
        ref: mergeRefs(extra?.ref, (node) => {
          if (node) {
            taskElements.current.set(taskId, node);
          } else {
            taskElements.current.delete(taskId);
          }
        }),
      };
    },
    [board.tasks, resolveFocusedTaskId],
  );

  return { activeTaskId, getColumnProps, getTaskProps, announcement };
}
