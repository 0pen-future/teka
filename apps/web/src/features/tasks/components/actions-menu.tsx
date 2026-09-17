import { MoreHorizontalIcon } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ColumnId, KanbanColumn } from "@/lib/kanban";
import { cn } from "@/lib/utils";

export interface ActionsMenuProps {
  currentColumnId: ColumnId;
  columns: KanbanColumn[];
  onMove: (columnId: ColumnId) => void;
  disabled?: boolean;
}

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
  disabled = false,
}: ActionsMenuProps) {
  const targets = [...columns]
    .filter((column) => column.id !== currentColumnId)
    .sort((a, b) => a.order - b.order);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        disabled={disabled || targets.length === 0}
        aria-label="Thao tác"
        className={cn(
          "inline-flex size-10 shrink-0 items-center justify-center rounded-full text-ink-500",
          "opacity-0 transition-opacity hover:bg-cream-100 group-hover:opacity-100",
          "group-focus-within:opacity-100 [@media(hover:none)]:opacity-100",
          "disabled:cursor-not-allowed disabled:opacity-50",
        )}
      >
        <MoreHorizontalIcon aria-hidden className="size-5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        <DropdownMenuLabel>Chuyển sang</DropdownMenuLabel>
        {targets.map((column) => (
          <DropdownMenuItem key={column.id} onSelect={() => onMove(column.id)}>
            {column.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
