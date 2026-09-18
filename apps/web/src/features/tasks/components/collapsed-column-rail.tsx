import { ChevronRightIcon } from "lucide-react";

import { cn } from "@/lib/utils";

import type { AppColumn } from "../hooks/use-tasks-data-source";
import { columnTintClassName } from "../lib/column-colors";

export interface CollapsedColumnRailProps {
  column: AppColumn;
  taskCount: number;
  onExpand: () => void;
  className?: string;
}

/**
 * A 56px rail standing in for a collapsed column: name rotated vertical and
 * task count on the column's own tint, so a crowded board can be narrowed to
 * the columns actually in play without losing track of what's hidden.
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
        "flex min-h-[360px] w-14 shrink-0 cursor-pointer flex-col items-center gap-2 rounded-[16px] px-1 py-2",
        "transition-[filter] hover:brightness-[0.98] focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        columnTintClassName(column),
        className,
      )}
    >
      <span className="inline-flex size-9 shrink-0 items-center justify-center rounded-full text-ink-500">
        <ChevronRightIcon aria-hidden className="size-[18px]" />
      </span>
      <span
        className="min-h-0 flex-1 truncate text-[14px] font-extrabold text-ink-900 [writing-mode:vertical-rl]"
        style={{ transform: "rotate(180deg)" }}
      >
        {column.name}
      </span>
      <span className="shrink-0 rounded-full bg-white px-2 py-[3px] font-body text-[11.5px] font-extrabold tabular-nums text-ink-500">
        {taskCount}
      </span>
    </button>
  );
}
