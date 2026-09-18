import { useDndMonitor } from "@dnd-kit/core";
import { useSortable } from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";
import { CalendarIcon, CheckIcon, ClockIcon, TriangleAlertIcon } from "lucide-react";
import { useMemo, useRef, type ComponentProps, type CSSProperties, type ReactNode } from "react";

import { HvBadge } from "@/components/hv";
import type { ColumnId, TaskPropsExtra } from "@/lib/kanban";
import { cn, formatDateTime } from "@/lib/utils";

import type { BoardDndData } from "../hooks/use-board-dnd";
import type { AppColumn, AppTask } from "../hooks/use-tasks-data-source";
import { dueState, type DueStateKind } from "../lib/due-state";
import { textFromHtml } from "../lib/rich-text";
import { ActionsMenu } from "./actions-menu";
import { AssigneeAvatar } from "./assignee-avatar";
import { CardControlBarrier } from "./card-control-barrier";
import { QuickDoneButton } from "./quick-done-button";
import { PRIORITY_LABELS, PRIORITY_VARIANTS, taskCardSurfaceClassName } from "./task-card-styles";

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

const DUE_ICONS: Record<Exclude<DueStateKind, "none">, typeof CalendarIcon> = {
  overdue: TriangleAlertIcon,
  today: ClockIcon,
  tomorrow: CalendarIcon,
  upcoming: CalendarIcon,
};

export interface TaskCardBodyProps {
  task: AppTask;
  assigneeName: string | null;
  /**
   * Slot for the live card's actions menu, rendered at the end of the
   * header row next to the title. The drag-overlay preview leaves this
   * unset — a static copy has nothing to act on.
   */
  actions?: ReactNode;
  /**
   * Slot for the quick-done button hanging over the card's leading edge.
   * Unset on the drag-overlay preview, same reasoning as `actions`.
   */
  quickDone?: ReactNode;
}

/**
 * Header row + chips, shared by the live card and the drag-overlay
 * preview. `Xong dd/MM` uses `formatDateTime` (completedAt is an RFC3339
 * instant, not a bare DATE) and keeps only the leading `dd/MM` slice, since
 * the exact time of completion doesn't matter here.
 */
export function TaskCardBody({ task, assigneeName, actions, quickDone }: TaskCardBodyProps) {
  const done = task.completedAt !== null;
  const completedLabel = task.completedAt ? formatDateTime(task.completedAt).slice(0, 5) : null;
  const due = useMemo(() => dueState(task), [task]);
  const preview = useMemo(() => textFromHtml(task.description), [task.description]);
  const DueIcon = due.kind === "none" ? null : DUE_ICONS[due.kind];
  return (
    <>
      {quickDone}
      <div className="flex items-start gap-1.5">
        <p
          className={cn(
            "min-w-0 flex-1 font-body text-[14.5px] font-extrabold leading-[1.3] text-ink-900 [overflow-wrap:anywhere]",
            done && "line-through decoration-ink-300 decoration-[1.5px]",
          )}
        >
          {due.kind === "overdue" ? (
            <span
              aria-hidden
              className="mr-1.5 inline-block size-2 rounded-full bg-coral-400 align-middle"
            />
          ) : null}
          {task.title}
        </p>
        {assigneeName ? <AssigneeAvatar name={assigneeName} size="sm" /> : null}
        {actions}
      </div>
      {preview ? (
        <p className="mt-1 line-clamp-2 text-[13px] leading-[1.45] text-ink-500">{preview}</p>
      ) : null}
      <div className="mt-2.5 flex flex-wrap items-center gap-1.5 empty:hidden">
        {task.priority !== "none" ? (
          <HvBadge size="sm" dot variant={PRIORITY_VARIANTS[task.priority]}>
            {PRIORITY_LABELS[task.priority]}
          </HvBadge>
        ) : null}
        {completedLabel ? (
          <HvBadge size="sm" variant="success">
            <CheckIcon aria-hidden className="size-3" />
            Xong {completedLabel}
          </HvBadge>
        ) : task.dueOn && DueIcon ? (
          <HvBadge size="sm" variant={due.variant}>
            <DueIcon aria-hidden className="size-3" />
            {due.label}
          </HvBadge>
        ) : null}
      </div>
    </>
  );
}

export interface TaskCardProps {
  task: AppTask;
  columns: AppColumn[];
  assigneeName: string | null;
  onOpen: () => void;
  onMove: (columnId: ColumnId) => void;
  /** Moves the task to the first "done" column; the button only renders while the task is still open. */
  onQuickDone: () => void;
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
  onOpen,
  onMove,
  onQuickDone,
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

  return (
    // `getTaskProps` spreads the headless lib's role="option"/tabIndex/roving
    // focus wiring, which the a11y rule can't see through a spread to verify;
    // the explicit `onKeyDown` below already covers Enter/Space activation.
    // eslint-disable-next-line jsx-a11y/no-static-element-interactions
    <div
      {...getTaskProps({ ref: setNodeRef, ...listeners })}
      onClick={handleClick}
      onKeyDown={(event) => {
        // Only the card itself opens on Enter/Space; a focused nested button
        // (quick-done, actions menu) keeps its native activation, since
        // `CardControlBarrier` stops pointer events but not keyboard ones.
        if (event.target !== event.currentTarget) return;
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onOpen();
        }
      }}
      style={style}
      data-dragging={isDragging || undefined}
      className={cn(
        "group",
        taskCardSurfaceClassName(task),
        "cursor-pointer outline-none hover:-translate-y-px hover:shadow-md",
        "focus-visible:ring-4 focus-visible:ring-mint-100",
        !moveDisabled && "cursor-grab active:cursor-grabbing",
        isDragging && "opacity-40",
      )}
    >
      <TaskCardBody
        task={task}
        assigneeName={assigneeName}
        quickDone={
          task.completedAt === null ? (
            // Same barrier reasoning as `actions` below: a click or press on
            // the button must not open the card or start a drag.
            <CardControlBarrier className="contents">
              <QuickDoneButton onDone={onQuickDone} disabled={moveDisabled} />
            </CardControlBarrier>
          ) : null
        }
        actions={
          // The barrier keeps the menu from opening the card (click) or
          // starting a drag (mouse/touch press reaching dnd-kit's listeners).
          <CardControlBarrier className="shrink-0">
            <ActionsMenu
              currentColumnId={task.columnId}
              columns={columns}
              onMove={onMove}
              onOpen={onOpen}
              moveDisabled={moveDisabled}
            />
          </CardControlBarrier>
        }
      />
    </div>
  );
}
