import { ArrowLeftRightIcon, MoreHorizontalIcon, SquareArrowOutUpRightIcon } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ColumnId, KanbanColumn } from "@/lib/kanban";
import { cn } from "@/lib/utils";

export interface ActionsMenuProps {
  currentColumnId: ColumnId;
  columns: KanbanColumn[];
  onMove: (columnId: ColumnId) => void;
  onOpen: () => void;
  /** Hides the move items only; "Mở chi tiết" stays reachable for a viewer who cannot move. */
  moveDisabled?: boolean;
}

const itemClassName =
  "min-h-10 gap-2 rounded-[9px] px-2.5 font-body text-[13.5px] font-bold text-ink-700 focus:bg-cream-100 focus:text-ink-900";

/**
 * The pointer-free way to change column (drag-and-drop and the lib's `[`/`]`
 * shortcut are the others); on phones it is also the only cross-column
 * path, since drag there stays inside the visible column. Columns render
 * in board order; the current column is excluded rather than shown
 * disabled, since "move here" never applies to where the task already is.
 *
 * The ⋯ trigger stays invisible until the card is hovered, focused, or (on
 * a touch device, which has no hover) always — `[@media(hover:none)]`
 * covers that last case so the only cross-column path on phones is never
 * hidden behind a gesture that doesn't exist there.
 */
export function ActionsMenu({
  currentColumnId,
  columns,
  onMove,
  onOpen,
  moveDisabled = false,
}: ActionsMenuProps) {
  const targets = moveDisabled
    ? []
    : [...columns]
        .filter((column) => column.id !== currentColumnId)
        .sort((a, b) => a.order - b.order);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        aria-label="Thao tác"
        className={cn(
          "-m-1.5 inline-flex size-9 shrink-0 items-center justify-center rounded-full text-ink-500",
          "opacity-0 transition-opacity hover:bg-cream-100 hover:text-ink-900 group-hover:opacity-100",
          "group-focus-within:opacity-100 data-[state=open]:opacity-100 [@media(hover:none)]:opacity-100",
          "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        )}
      >
        <MoreHorizontalIcon aria-hidden className="size-[18px]" />
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        className="min-w-[180px] rounded-[14px] border-[1.5px] border-line-200 p-1.5 shadow-lg"
      >
        {targets.length > 0 ? (
          <>
            <DropdownMenuLabel className="px-2.5 pb-1 pt-1.5 font-display text-[11px] font-extrabold uppercase tracking-[0.06em] text-ink-400">
              Chuyển tới
            </DropdownMenuLabel>
            {targets.map((column) => (
              <DropdownMenuItem
                key={column.id}
                onSelect={() => onMove(column.id)}
                className={itemClassName}
              >
                <ArrowLeftRightIcon aria-hidden className="size-[15px] text-ink-400" />
                {column.name}
              </DropdownMenuItem>
            ))}
            <DropdownMenuSeparator className="my-1 h-[1.5px] bg-line-100" />
          </>
        ) : null}
        <DropdownMenuItem onSelect={onOpen} className={itemClassName}>
          <SquareArrowOutUpRightIcon aria-hidden className="size-[15px] text-ink-400" />
          Mở chi tiết
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
