import { cn } from "@/lib/utils";

import { ATTENDANCE_STATUSES, type AttendanceStatus } from "./attendance-status-meta";

export interface StatusCountChipsProps {
  counts: Record<AttendanceStatus, number>;
}

/**
 * "Đúng giờ n · Muộn n · Vắng n · Có lý do n" above the sheet. Zero-count
 * chips are hidden — except Đúng giờ, which anchors the row so the teacher
 * always sees the class size confirmed back at them.
 */
export function StatusCountChips({ counts }: StatusCountChipsProps) {
  return (
    <div className="flex flex-wrap items-center gap-[8px]">
      {ATTENDANCE_STATUSES.filter(
        (status) => status.value === "present" || counts[status.value] > 0,
      ).map((status) => (
        <span
          key={status.value}
          className={cn("rounded-full px-3 py-[5px] text-[13px] font-extrabold", status.chipClass)}
        >
          {status.label} {counts[status.value]}
        </span>
      ))}
    </div>
  );
}
