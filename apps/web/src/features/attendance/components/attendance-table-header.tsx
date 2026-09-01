import { cn } from "@/lib/utils";

import { ATTENDANCE_STATUSES, attendanceGridClass } from "./attendance-status-meta";

/**
 * The sheet's column titles: HỌC SINH plus the four colored status columns.
 * Sticky inside the row scroller so a 30-student class keeps its column
 * meanings visible. Presentation-only (`aria-hidden`): every radio cell
 * already carries its status label, so AT users never need this row.
 */
export function AttendanceTableHeader() {
  return (
    <div
      aria-hidden
      className={cn(
        attendanceGridClass,
        "sticky top-0 z-10 border-b border-line-100 bg-white px-3 pb-2 pt-1",
      )}
    >
      <span className="text-[11px] font-extrabold uppercase tracking-wide text-ink-400">
        Học sinh
      </span>
      {ATTENDANCE_STATUSES.map((status) => (
        <span
          key={status.value}
          className={cn("text-center text-[10.5px] font-extrabold leading-tight", status.inkClass)}
        >
          {status.label}
        </span>
      ))}
    </div>
  );
}
