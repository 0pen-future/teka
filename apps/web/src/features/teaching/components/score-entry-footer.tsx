import type * as React from "react";

import { HvButton } from "@/components/hv";
import { cn } from "@/lib/utils";

export interface ScoreEntryFooterProps {
  scoredStudents: number;
  total: number;
  dirtyCount: number;
  invalidCount: number;
  isSaving: boolean;
  onSave: () => void;
  /** "sticky" pins the bar under a scrolling list; "plain" fits a modal footer slot. */
  variant?: "sticky" | "plain";
  /** Extra buttons rendered before "Lưu điểm" (e.g. a modal's close). */
  actions?: React.ReactNode;
}

/**
 * Sticky status bar under a score-entry surface: grading progress, unsaved
 * cell count, and the explicit "Lưu điểm" that flushes the autosave now.
 * Saving is blocked while any cell still holds unreadable text so a typo
 * never silently disappears from the batch.
 */
export function ScoreEntryFooter({
  scoredStudents,
  total,
  dirtyCount,
  invalidCount,
  isSaving,
  onSave,
  variant = "sticky",
  actions,
}: ScoreEntryFooterProps) {
  return (
    <div
      className={cn(
        "flex w-full items-center justify-between gap-3",
        variant === "sticky" && "sticky bottom-0 border-t border-line-200 bg-white pt-2.5 pb-1",
      )}
    >
      <p role="status" aria-live="polite" className="text-[12.5px] font-bold text-ink-500">
        {scoredStudents}/{total} học sinh đã chấm
        <span aria-hidden="true"> · </span>
        <span className={cn(dirtyCount > 0 && "text-sun-600")}>
          {isSaving ? "Đang lưu…" : `${dirtyCount} ô chưa lưu`}
        </span>
        {invalidCount > 0 ? (
          <span className="text-coral-600">
            <span aria-hidden="true"> · </span>
            {invalidCount} ô không hợp lệ
          </span>
        ) : null}
      </p>
      <div className="flex shrink-0 items-center gap-2">
        {actions}
        <HvButton
          type="button"
          variant="primary"
          size="sm"
          onClick={onSave}
          disabled={dirtyCount === 0 || invalidCount > 0 || isSaving}
        >
          Lưu điểm
        </HvButton>
      </div>
    </div>
  );
}
