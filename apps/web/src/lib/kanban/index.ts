/**
 * Public surface of the headless kanban core. See README.md for the ports,
 * a11y contract, boundary rules, and extraction procedure. Only `react` is
 * an external dependency — nothing here imports app features, components,
 * or the API client (enforced by the eslint.config.js override for this
 * directory).
 */

export type { AdjacentDirection } from "./selectors";
export { selectAdjacentColumnId, selectColumnCounts, selectTasksByColumn } from "./selectors";

export type { KanbanAction } from "./state";
export { kanbanReducer } from "./state";

export type { KanbanError, KanbanErrorEntity } from "./errors";
export { isKanbanError } from "./errors";

export type { KanbanDataSource } from "./data-source";

export type { DropEvent, DropTarget } from "./positions";
export { afterTaskIdAt, optimisticPositionFor, positionBetween, resolveDrop } from "./positions";

export type { ColumnId, KanbanBoard, KanbanColumn, KanbanTask, TaskId } from "./types";
export { asColumnId, asTaskId } from "./types";

export type { ColumnProps, TaskProps, UseKanbanOptions, UseKanbanResult } from "./use-kanban";
export { useKanban } from "./use-kanban";

export type {
  ColumnPropsExtra,
  KeyboardColumnProps,
  KeyboardTaskProps,
  TaskPropsExtra,
  UseKanbanKeyboardMessages,
  UseKanbanKeyboardOptions,
  UseKanbanKeyboardResult,
} from "./use-kanban-keyboard";
export { useKanbanKeyboard } from "./use-kanban-keyboard";
