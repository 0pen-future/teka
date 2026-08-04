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
function statusText(session: Session, unconfirmedPast: boolean) {
  if (session.status === "cancelled") {
    return {
      text: `Đã huỷ — ${session.cancel_reason ?? "không rõ lý do"}`,
      className: "text-ink-400",
    };
  }
  if (session.attendance_confirmed_at) {
    return { text: "Đã điểm danh", className: "text-mint-600" };
  }
  if (unconfirmedPast) {
    return { text: "Chưa điểm danh", className: "font-bold text-coral-600" };
  }
  return { text: "Sắp diễn ra", className: "text-ink-400" };
}

/** The whole row is the link target — thumb-friendly, min-height 48px. */
export function SessionListItem({ session, unconfirmedPast, selected }: SessionListItemProps) {
  const status = statusText(session, unconfirmedPast);
  return (
    <Link
      to={`/sessions/${session.id}/attendance`}
      className={cn(
        "flex min-h-12 items-center justify-between gap-3 rounded-[var(--radius-lg)] border-2 px-4 py-3 transition-colors",
        selected ? "border-mint-300 bg-mint-50" : "border-transparent bg-white hover:bg-cream-100",
      )}
    >
      <span className="flex min-w-0 flex-col">
        <span className="truncate font-display text-[14px] font-bold text-ink-900">
          {session.class_name} — {formatSessionDate(session.session_date)}
        </span>
        {session.start_time ? (
          <span className="text-[13px] text-ink-400">{session.start_time}</span>
        ) : null}
      </span>
      <span className={cn("shrink-0 text-[13px]", status.className)}>{status.text}</span>
    </Link>
  );
}
