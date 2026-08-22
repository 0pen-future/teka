import { HvBadge, HvCheckIcon, HvXIcon } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { AttendanceRow as AttendanceRowData } from "../schemas/attendance-schemas";

export interface AttendanceRowProps {
  row: AttendanceRowData;
  /** Absent state is local (`Set<string>`), not the row's stale server status. */
  absent: boolean;
  /** True when another roster row shares this student's `full_name` — promotes `display_note` to a badge. */
  duplicateName: boolean;
  onToggle: (studentId: string) => void;
}

/**
 * A full-width tappable row — not a checkbox with a separate label — so the
 * entire 52px-tall strip is the touch target (PRD R2's one-touch budget).
 * Toggling is a pure local state flip: no network call, no per-row spinner.
 */
export function AttendanceRow({ row, absent, duplicateName, onToggle }: AttendanceRowProps) {
  return (
    <button
      type="button"
      aria-pressed={absent}
      onClick={() => onToggle(row.student_id)}
      className={cn(
        "flex min-h-[52px] w-full items-center gap-3 rounded-[16px] border-2 px-3 py-2 text-left transition-colors",
        absent ? "border-coral-300 bg-coral-100" : "border-line-100 bg-cream-50",
      )}
    >
      <span
        aria-hidden
        className={cn(
          "flex size-[34px] shrink-0 items-center justify-center rounded-full",
          absent ? "bg-coral-400 text-white" : "bg-mint-100 text-mint-600",
        )}
      >
        {absent ? <HvXIcon size={18} /> : <HvCheckIcon size={18} />}
      </span>
      <span className="flex min-w-0 flex-1 flex-col">
        <span className="flex flex-wrap items-center gap-2">
          <span className="truncate text-[15px] font-extrabold text-ink-900">
            {row.student_name}
          </span>
          {duplicateName && row.display_note ? (
            <HvBadge variant="info" size="sm">
              {row.display_note}
            </HvBadge>
          ) : null}
        </span>
        {!duplicateName && row.display_note ? (
          <span className="truncate text-[12px] text-ink-400">{row.display_note}</span>
        ) : null}
      </span>
      {/* aria-hidden: aria-pressed already carries the state for AT, and
          keeping it out of the accessible name lets tests/readers address
          rows by student name alone. */}
      <span aria-hidden className="shrink-0 text-[12.5px] font-bold text-ink-400">
        {absent ? "Vắng" : "Có mặt"}
      </span>
    </button>
  );
}
