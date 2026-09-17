import { ChevronRightIcon } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import type { ColumnId, KanbanColumn } from "@/lib/kanban";

export interface MoveMenuProps {
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
 */
export function MoveMenu({ currentColumnId, columns, onMove, disabled = false }: MoveMenuProps) {
  const targets = [...columns]
    .filter((column) => column.id !== currentColumnId)
    .sort((a, b) => a.order - b.order);

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        disabled={disabled || targets.length === 0}
        aria-label="Chuyển cột"
        className="inline-flex h-7 items-center gap-0.5 rounded-md px-1.5 text-[12px] font-bold text-ink-500 hover:bg-cream-100 disabled:cursor-not-allowed disabled:opacity-50"
      >
        Chuyển
        <ChevronRightIcon aria-hidden className="size-3.5" />
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end">
        {targets.map((column) => (
          <DropdownMenuItem key={column.id} onSelect={() => onMove(column.id)}>
            {column.name}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
