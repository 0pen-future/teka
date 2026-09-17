import { useDndMonitor } from "@dnd-kit/core";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import {
  useMemo,
  useRef,
  type ComponentProps,
  type CSSProperties,
  type MouseEvent,
  type TouchEvent,
} from "react";

import { HvBadge } from "@/components/hv";
import type { ColumnId, KanbanColumn, TaskPropsExtra } from "@/lib/kanban";
import { cn, formatDateTime, formatDayMonth } from "@/lib/utils";

import type { BoardDndData } from "../hooks/use-board-dnd";
import type { AppTask } from "../hooks/use-tasks-data-source";
import { textFromHtml } from "../lib/rich-text";
import { MoveMenu } from "./move-menu";
import {
  PRIORITY_LABELS,
  PRIORITY_VARIANTS,
  isOverdue,
  taskCardSurfaceClassName,
} from "./task-card-styles";

/**
 * How long after a drop the card ignores a click. Browsers fire `click`
 * after `mouseup`/`touchend` when the pointer lands on the element it
 * pressed, which is exactly what a drop back onto (or near) the source card
 * looks like. dnd-kit already swallows the click at document capture for
 * the 50ms after a mouse drag; this window covers the touch path, where the
 * synthesized click can arrive later, so the card does not open its editor
 * right after being moved.
 */
const CLICK_SUPPRESSION_MS = 300;

export interface TaskCardBodyProps {
  task: AppTask;
  assigneeName: string | null;
}

/**
 * Title + badges, shared by the live card and the drag-overlay preview.
 * `xong dd/mm` uses `formatDateTime` (completedAt is an RFC3339 instant, not
 * a bare DATE) and keeps only the leading `dd/MM` slice, since the exact time
 * of completion doesn't matter here.
 */
export function TaskCardBody({ task, assigneeName }: TaskCardBodyProps) {
  const done = task.completedAt !== null;
  const overdue = isOverdue(task);
  const preview = useMemo(() => textFromHtml(task.description), [task.description]);
  return (
    <>
      <p className="text-[13.5px] font-bold text-ink-900">{task.title}</p>
      {preview ? (
        <p className="mt-1 line-clamp-2 text-[12px] leading-snug text-ink-500">{preview}</p>
      ) : null}
      <div className="mt-2 flex flex-wrap items-center gap-1.5">
        <HvBadge size="sm" variant={PRIORITY_VARIANTS[task.priority]}>
          {PRIORITY_LABELS[task.priority]}
        </HvBadge>
        {done ? (
          <HvBadge size="sm" variant="success">
            Xong {formatDateTime(task.completedAt!).slice(0, 5)}
          </HvBadge>
        ) : task.dueOn ? (
          <HvBadge size="sm" variant={overdue ? "danger" : "neutral"}>
            Hạn {formatDayMonth(task.dueOn)}
          </HvBadge>
        ) : null}
        {assigneeName ? (
          <span className="truncate text-[11.5px] font-semibold text-ink-400">{assigneeName}</span>
        ) : null}
      </div>
    </>
  );
}

export interface TaskCardProps {
  task: AppTask;
  columns: KanbanColumn[];
  assigneeName: string | null;
  isAssignedToMe: boolean;
  onOpen: () => void;
  onMove: (columnId: ColumnId) => void;
  moveDisabled?: boolean;
  /** Skip the sort transition under `prefers-reduced-motion` (read once by `useBoardDnd`). */
  reducedMotion: boolean;
  /**
   * The lib's prop getter for this task, already bound to its id. The card
   * feeds dnd-kit's node ref and activator listeners through `extra` so the
   * lib merges them with its own roving-tabindex / focus-ref wiring.
   */
  getTaskProps: (extra: TaskPropsExtra) => ComponentProps<"div">;
}

/**
 * A task card is a dnd-kit sortable *and* the lib's `role="option"`.
 * dnd-kit's `attributes` are deliberately not spread: they would replace
 * the lib's roving `tabIndex` and `role` with `tabIndex=0` / `role="button"`
 * plus an English `aria-roledescription`, and keyboard moves already go
 * through the lib's `[`/`]` shortcut.
 */
export function TaskCard({
  task,
  columns,
  assigneeName,
  isAssignedToMe,
  onOpen,
  onMove,
  moveDisabled = false,
  reducedMotion,
  getTaskProps,
}: TaskCardProps) {
  const sortableData: BoardDndData = { type: "task", columnId: task.columnId };
  const { setNodeRef, listeners, transform, transition, isDragging } = useSortable({
    id: task.id,
    data: sortableData,
    disabled: moveDisabled,
  });

  // Stamped inside dnd-kit's own drag-end dispatch, i.e. during the
  // `mouseup`/`touchend` that ends the drag and before the click it spawns.
  const droppedAtRef = useRef(0);
  useDndMonitor({
    onDragEnd: ({ active }) => {
      if (active.id === task.id) droppedAtRef.current = Date.now();
    },
  });

  const handleClick = () => {
    if (Date.now() - droppedAtRef.current < CLICK_SUPPRESSION_MS) return;
    onOpen();
  };

  const style: CSSProperties = {
    transform: CSS.Translate.toString(transform),
    transition: reducedMotion ? undefined : transition,
    // Lets the browser scroll on a quick touch drag while dnd-kit's 250ms
    // hold still claims a deliberate one.
    touchAction: "manipulation",
  };

  const stopPointerActivation = (event: MouseEvent | TouchEvent) => {
    event.stopPropagation();
  };

  return (
    // `getTaskProps` spreads the headless lib's role="option"/tabIndex/roving
    // focus wiring, which the a11y rule can't see through a spread to verify;
    // the explicit `onKeyDown` below already covers Enter/Space activation.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      {...getTaskProps({ ref: setNodeRef, ...listeners })}
      onClick={handleClick}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      style={style}
      data-dragging={isDragging || undefined}
      className={cn(
        taskCardSurfaceClassName(task, isAssignedToMe),
        "cursor-pointer outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        !moveDisabled && "cursor-grab active:cursor-grabbing",
        isDragging && "opacity-40",
      )}
    >
      <TaskCardBody task={task} assigneeName={assigneeName} />
      {/* Bubbling barrier only: "Chuyển" must neither open the card (click)
          nor start a drag (mouse/touch press reaching dnd-kit's listeners). */}
      <div
        role="presentation"
        className="mt-2 flex justify-end"
        onClick={(event) => event.stopPropagation()}
        onMouseDown={stopPointerActivation}
        onTouchStart={stopPointerActivation}
      >
        <MoveMenu
          currentColumnId={task.columnId}
          columns={columns}
          onMove={onMove}
          disabled={moveDisabled}
        />
      </div>
    </div>
  );
}
