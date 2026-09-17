import { ChevronRightIcon } from "lucide-react";

import { cn } from "@/lib/utils";

import type { AppColumn } from "../hooks/use-tasks-data-source";
import { COLUMN_DOT, COLUMN_TINT } from "../lib/column-colors";

export interface CollapsedColumnRailProps {
  column: AppColumn;
  taskCount: number;
  onExpand: () => void;
  className?: string;
}

/**
 * A 44px rail standing in for a collapsed column: name rotated vertical,
 * task count, and its tint dot, so a crowded board can be narrowed to the
 * columns actually in play without losing track of what's hidden.
 */
export function CollapsedColumnRail({
  column,
  taskCount,
  onExpand,
  className,
}: CollapsedColumnRailProps) {
  return (
    <button
      type="button"
      onClick={onExpand}
      aria-expanded={false}
      aria-label={`Mở rộng cột ${column.name}`}
      className={cn(
        "flex w-11 shrink-0 flex-col items-center gap-2 rounded-[16px] border border-line-200 py-3",
        "hover:bg-white focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        COLUMN_TINT[column.color],
        className,
      )}
    >
      <ChevronRightIcon aria-hidden className="size-4 shrink-0 text-ink-500" />
      <span
        aria-hidden
        className={cn("size-2.5 shrink-0 rounded-full", COLUMN_DOT[column.color])}
      />
      <span className="shrink-0 rounded-full bg-white px-1.5 py-0.5 text-[11px] font-bold text-ink-500">
        {taskCount}
      </span>
      <span
        className="min-h-0 flex-1 truncate text-[12px] font-extrabold text-ink-900 [writing-mode:vertical-rl]"
        style={{ transform: "rotate(180deg)" }}
      >
        {column.name}
      </span>
    </button>
  );
}
