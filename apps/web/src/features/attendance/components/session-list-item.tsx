import { Link } from "react-router";

import { cn, formatSessionDate } from "@/lib/utils";

import type { Session } from "../schemas/attendance-schemas";

export interface SessionListItemProps {
  session: Session;
  /** Flags a past session that still has no attendance recorded. */
  unconfirmedPast: boolean;
  /** Two-pane (`lg+`) row highlight for the currently open session. */
  selected: boolean;
}

/**
 * `sessions.SessionResponse` carries no present/absent split (only
 * `student_count`, the roster size, not a confirmed breakdown) — a session
 * already held renders "Đã điểm danh" here; the exact "N có mặt · M vắng"
 * split only exists once the attendance panel itself loads `rows`.
 */
function statusText(session: Session, unconfirmedPast: boolean): string {
  if (session.status === "cancelled") {
    return `Đã huỷ — ${session.cancel_reason ?? "không rõ lý do"}`;
  }
  if (session.attendance_confirmed_at) {
    return "Đã điểm danh";
  }
  if (unconfirmedPast) {
    return "Chưa điểm danh";
  }
  return "Sắp diễn ra";
}

/**
 * Prototype status coloring tints the whole row, not only the status label —
 * mint for confirmed, coral for overdue, muted for cancelled/upcoming.
 */
function rowColor(session: Session, unconfirmedPast: boolean) {
  if (session.status === "cancelled") {
    return "text-ink-400";
  }
  if (session.attendance_confirmed_at) {
    return "text-mint-600";
  }
  if (unconfirmedPast) {
    return "text-coral-600";
  }
  return "text-ink-400";
}

/** The whole row is the link target — thumb-friendly, min-height 44px. */
export function SessionListItem({ session, unconfirmedPast, selected }: SessionListItemProps) {
  const status = statusText(session, unconfirmedPast);
  return (
    <Link
      to={`/sessions/${session.id}/attendance`}
      className={cn(
        "flex min-h-11 items-center gap-2 rounded-[12px] border-2 px-3 py-[9px] text-[13.5px] transition-colors",
        rowColor(session, unconfirmedPast),
        selected ? "border-mint-300 bg-mint-50" : "border-transparent hover:bg-cream-100",
      )}
    >
      <span className="flex min-w-0 flex-col">
        <span className="truncate font-extrabold">
          {session.class_name} — {formatSessionDate(session.session_date)}
        </span>
        {session.start_time ? (
          <span className="text-[12.5px] font-semibold text-ink-400">{session.start_time}</span>
        ) : null}
      </span>
      <span className="ml-auto shrink-0 text-[12.5px] font-bold">{status}</span>
    </Link>
  );
}
