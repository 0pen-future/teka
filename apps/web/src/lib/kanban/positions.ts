import { selectTasksByColumn } from "./selectors";
import { asColumnId, type ColumnId, type KanbanBoard, type KanbanTask, type TaskId } from "./types";

/**
 * Where a dragged task should land.
 *
 * Invariants:
 * - `index` counts positions in the target column's sorted task list
 *   **after the moving task itself has been removed** from it, so `0` is the
 *   top and `tasks.length` (of the remaining tasks) is the bottom.
 * - `afterTaskId === null` means "top of the column"; otherwise it is the id
 *   of the task the moved one should sit directly behind. It is the same
 *   information as `index`, expressed in the way a server that stores
 *   "insert after X" wants it.
 */
export interface DropTarget {
  columnId: ColumnId;
  index: number;
  afterTaskId: TaskId | null;
}

/**
 * What a drag ended on, as reported by the pointer drag-and-drop layer the app
 * chose. The lib never sees that library: the adapter tags `overType` itself
 * from whatever metadata it attached to each droppable.
 */
export interface DropEvent {
  /** The task being dragged. */
  taskId: TaskId;
  /** Id of the droppable the pointer was released over (a task or a column). */
  overId: string;
  overType: "task" | "column";
}

function withoutTask<TTask extends KanbanTask>(tasks: readonly TTask[], movingId: TaskId): TTask[] {
  return tasks.filter((task) => task.id !== movingId);
}

/**
 * Id of the task that would sit directly above a task inserted at `index`
 * (counted with `movingId` removed from `tasks`), or `null` when `index` is
 * `0` (top of the column). An `index` past the end resolves to the last
 * remaining task, i.e. the bottom of the column.
 */
export function afterTaskIdAt(
  tasks: readonly KanbanTask[],
  index: number,
  movingId: TaskId,
): TaskId | null {
  if (index <= 0) {
    return null;
  }
  const remaining = withoutTask(tasks, movingId);
  const before = remaining[Math.min(index, remaining.length) - 1];
  return before ? before.id : null;
}

/**
 * A `position` that sorts between two neighbours, for optimistic updates via
 * `kanbanReducer`. Gaps can get arbitrarily small (or negative when moving to
 * the top): `selectTasksByColumn` sorts by number, so any float works locally,
 * and renormalizing the stored values is the server's job, not the lib's.
 */
export function positionBetween(prev: number | undefined, next: number | undefined): number {
  if (prev === undefined) {
    return next === undefined ? 0 : next - 1;
  }
  return next === undefined ? prev + 1 : (prev + next) / 2;
}

/**
 * `positionBetween` applied to the neighbours a task inserted at `index`
 * (counted with `movingId` removed from `tasks`) would have. `index` is
 * clamped to the remaining tasks exactly like `afterTaskIdAt`, so both
 * helpers always describe the same slot.
 */
export function optimisticPositionFor(
  tasks: readonly KanbanTask[],
  index: number,
  movingId: TaskId,
): number {
  const remaining = withoutTask(tasks, movingId);
  const slot = Math.max(0, Math.min(index, remaining.length));
  return positionBetween(remaining[slot - 1]?.position, remaining[slot]?.position);
}

/**
 * Turns the end of a drag into a {@link DropTarget}, mirroring what a vertical
 * sortable list shows while dragging (`arrayMove(items, activeIndex,
 * overIndex)`):
 *
 * - over a task in the **same** column: `index` is that task's index in the
 *   column *before* removing the moving one — dragging down lands the task
 *   just below `over`, dragging up lands it just above;
 * - over a task in **another** column: the task is inserted right before
 *   `over`;
 * - over a column: the task goes to the bottom of that column.
 *
 * Returns `null` when nothing should happen: the task is dropped on itself
 * or on the slot it already occupies (e.g. the last task of a column dropped
 * on that column), or `taskId`/`overId` is not on the board.
 */
export function resolveDrop<TTask extends KanbanTask>(
  board: KanbanBoard<TTask>,
  { taskId, overId, overType }: DropEvent,
): DropTarget | null {
  const moving = board.tasks.find((task) => task.id === taskId);
  if (!moving) {
    return null;
  }
  const tasksByColumn = selectTasksByColumn(board);

  let columnId: ColumnId;
  let index: number;
  if (overType === "column") {
    columnId = asColumnId(overId);
    const columnTasks = tasksByColumn.get(columnId);
    if (!columnTasks) {
      return null;
    }
    index = withoutTask(columnTasks, taskId).length;
  } else {
    const over = board.tasks.find((task) => task.id === overId);
    if (!over || over.id === taskId) {
      return null;
    }
    columnId = over.columnId;
    // Same column: the index of `over` in the un-filtered list equals the
    // slot the moving task ends up in after arrayMove. Other column: the
    // moving task is not in the list, so this is simply "before `over`".
    index = (tasksByColumn.get(columnId) ?? []).indexOf(over);
  }

  // selectTasksByColumn has an entry for every column any task references,
  // so the moving task's own column is always present.
  const columnTasks = tasksByColumn.get(columnId) ?? [];
  if (columnId === moving.columnId && columnTasks.indexOf(moving) === index) {
    return null;
  }
  return { columnId, index, afterTaskId: afterTaskIdAt(columnTasks, index, taskId) };
}
