import { HvBadge } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { AttendanceRow as AttendanceRowData } from "../schemas/attendance-schemas";
import {
  ATTENDANCE_STATUSES,
  attendanceGridClass,
  type AttendanceStatus,
} from "./attendance-status-meta";

export interface AttendanceRowProps {
  row: AttendanceRowData;
  /** Resolved local status (`present` when the student has no exception mark). */
  status: AttendanceStatus;
  /** The excused note being edited — local state, not the row's stale server note. */
  note: string;
  /** True when another roster row shares this student's `full_name` — promotes `display_note` to a badge. */
  duplicateName: boolean;
  /** Read-only viewers see the sheet but get no note input; selects are guarded by the page. */
  canWrite: boolean;
  onSelect: (studentId: string, status: AttendanceStatus) => void;
  onNoteChange: (studentId: string, note: string) => void;
}

/**
 * One student's strip of the 4-column sheet: a radiogroup named after the
 * student, with four 44px round radio cells aligned under the header's
 * columns. Selecting is a pure local state change — no network call, no
 * per-row spinner; tapping the already-selected exception returns the
 * student to Đúng giờ (handled by the page's select handler).
 */
export function AttendanceRow({
  row,
  status,
  note,
  duplicateName,
  canWrite,
  onSelect,
  onNoteChange,
}: AttendanceRowProps) {
  const meta = ATTENDANCE_STATUSES.find((s) => s.value === status)!;
  const excused = status === "excused";
  return (
    <div
      className={cn("rounded-[16px] border-2 border-line-100 transition-colors", meta.rowTintClass)}
    >
      <div
        role="radiogroup"
        aria-label={
          duplicateName && row.display_note
            ? `${row.student_name} (${row.display_note})`
            : row.student_name
        }
        className={cn(attendanceGridClass, "min-h-[54px] px-2 py-[5px]")}
      >
        <span className="flex min-w-0 flex-col pl-1">
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
          {excused ? (
            <span className="truncate text-[12px] font-bold text-sky-500">
              Vắng có phép{note.trim() ? ` — ${note.trim()}` : ""}
            </span>
          ) : null}
        </span>
        {ATTENDANCE_STATUSES.map((option) => {
          const checked = option.value === status;
          const Icon = option.icon;
          return (
            <button
              key={option.value}
              type="button"
              role="radio"
              aria-checked={checked}
              aria-label={option.label}
              onClick={() => onSelect(row.student_id, option.value)}
              className={cn(
                "mx-auto flex size-11 items-center justify-center rounded-full transition-[background-color,box-shadow,color]",
                checked ? option.selectedClass : "border-2 border-line-200 bg-white text-ink-300",
              )}
            >
              <Icon size={18} aria-hidden />
            </button>
          );
        })}
      </div>
      {excused && canWrite ? (
        <div className="px-3 pb-2">
          <input
            type="text"
            aria-label={`Lý do của ${row.student_name}`}
            placeholder="Lý do (vd: mẹ báo ốm)"
            maxLength={500}
            value={note}
            onChange={(event) => onNoteChange(row.student_id, event.target.value)}
            className="w-full rounded-[10px] border border-line-200 bg-white px-3 py-[6px] text-[13px] text-ink-900 outline-none placeholder:text-ink-300 focus:border-sky-300"
          />
        </div>
      ) : null}
    </div>
  );
}
