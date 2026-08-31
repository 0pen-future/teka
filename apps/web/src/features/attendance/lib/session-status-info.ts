import type { AttendanceSummary, Session } from "../schemas/attendance-schemas";

export interface SessionStatusInfo {
  text: string;
  /** Text color for the status label on cards. */
  textClass: string;
  /** Background for the calendar day dot. */
  dotClass: string;
}

/**
 * One source of truth for how a session's lifecycle reads across the trio
 * cards and the month calendar dots, so the two surfaces can never disagree:
 * coral = overdue, mint = confirmed, gray = upcoming, muted = cancelled.
 */
export function sessionStatusInfo(session: Session, today: string): SessionStatusInfo {
  if (session.status === "cancelled") {
    return { text: "Đã huỷ", textClass: "text-ink-300", dotClass: "bg-line-200" };
  }
  if (session.attendance_confirmed_at) {
    return { text: "Đã điểm danh", textClass: "text-mint-600", dotClass: "bg-mint-400" };
  }
  if (session.session_date <= today) {
    return { text: "Chưa điểm danh", textClass: "text-coral-600", dotClass: "bg-coral-400" };
  }
  return { text: "Sắp tới", textClass: "text-ink-400", dotClass: "bg-line-300" };
}

/**
 * Badge line for a confirmed session's card. The three core counts always
 * show; "có lý do" joins only when non-zero so the common no-excused case
 * stays short.
 */
export function formatAttendanceSummary(summary: AttendanceSummary): string {
  const parts = [`${summary.present} đúng giờ`, `${summary.late} muộn`, `${summary.absent} vắng`];
  if (summary.excused > 0) {
    parts.push(`${summary.excused} có lý do`);
  }
  return parts.join(" · ");
}
