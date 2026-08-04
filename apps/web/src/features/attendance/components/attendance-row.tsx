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
        "flex min-h-[52px] w-full items-center justify-between gap-3 border-b border-line-200 px-4 py-2 text-left transition-colors last:border-b-0",
        absent ? "bg-coral-100" : "bg-white",
      )}
    >
      <span className="flex min-w-0 flex-col">
        <span className="flex flex-wrap items-center gap-2">
          <span className="truncate font-display text-[15px] font-bold text-ink-900">
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
      <span
        aria-hidden
        className={cn(
          "flex size-[34px] shrink-0 items-center justify-center rounded-full",
          absent ? "bg-coral-400 text-white" : "bg-mint-50 text-mint-600",
        )}
      >
        {absent ? <HvXIcon size={18} /> : <HvCheckIcon size={18} />}
      </span>
    </button>
  );
}
