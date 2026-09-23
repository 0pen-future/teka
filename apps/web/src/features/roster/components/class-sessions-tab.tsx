import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";

import { HvBadge, HvStateBlock, type HvBadgeVariant } from "@/components/hv";
import { listClassSessions, sessionsKeys, type Session } from "@/features/attendance";
import { cn } from "@/lib/utils";

import { addDays, isScheduledSession, OPEN_ENDED_HORIZON_DAYS } from "../lib/class-sessions";
import { formatFullDate, formatWeekday } from "../lib/roster-format";
import type { Class } from "../schemas/roster-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px]";

const statusLabel: Record<Session["status"], string> = {
  planned: "Sắp diễn ra",
  held: "Đã diễn ra",
  cancelled: "Đã huỷ",
};

const statusVariant: Record<Session["status"], HvBadgeVariant> = {
  planned: "info",
  held: "success",
  cancelled: "danger",
};

interface ClassSessionsTabProps {
  klass: Class;
  today: string;
}

/**
 * Every session of the class, oldest first, with its source (timetable or
 * manual). Read-only on purpose: browsing a class must never materialise
 * sessions, which the pending-attendance and period-close flows would then
 * pick up. Generation stays with the classbook and the calendar.
 */
export function ClassSessionsTab({ klass, today }: ClassSessionsTabProps) {
  const window = {
    from: klass.start_date,
    to: klass.end_date ?? addDays(today, OPEN_ENDED_HORIZON_DAYS),
    readonly: true,
  };
  const result = useQuery({
    queryKey: sessionsKeys.list(klass.id, window),
    queryFn: () => listClassSessions(klass.id, window),
  });
  const isPending = result.isPending;
  const isError = result.isError;
  const sessions = [...(result.data ?? [])].sort(
    (a, b) =>
      a.session_date.localeCompare(b.session_date) ||
      (a.start_time ?? "").localeCompare(b.start_time ?? ""),
  );

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-[16px] font-bold text-ink-900">
          Buổi học ({sessions.length})
        </h2>
        <Link
          to={`/classbook?class_id=${klass.id}`}
          className="font-display text-[13px] font-bold text-mint-600 hover:underline"
        >
          Điểm danh & nhận xét →
        </Link>
      </div>

      {isPending ? (
        <HvStateBlock state="loading" title="Đang tải buổi học" />
      ) : isError ? (
        <HvStateBlock state="error" title="Không tải được buổi học" />
      ) : sessions.length === 0 ? (
        <HvStateBlock
          state="empty"
          title="Chưa có buổi học. Áp dụng chương trình mẫu để sinh danh sách buổi theo lịch hàng tuần."
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Ngày</th>
                <th className={headCellClassName}>Giờ</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={headCellClassName}>Học viên</th>
                <th className={headCellClassName}>Nguồn</th>
              </tr>
            </thead>
            <tbody>
              {sessions.map((session) => {
                const weekday = new Date(`${session.session_date}T00:00:00Z`).getUTCDay();
                return (
                  <tr key={session.id}>
                    <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                      {formatWeekday(weekday)} · {formatFullDate(session.session_date)}
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {session.start_time ?? "—"}
                    </td>
                    <td className={cellClassName}>
                      <HvBadge variant={statusVariant[session.status]} size="sm" dot>
                        {statusLabel[session.status]}
                      </HvBadge>
                      {session.cancel_reason ? (
                        <span className="ml-2 text-[13px] text-ink-400">
                          {session.cancel_reason}
                        </span>
                      ) : null}
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>{session.student_count}</td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {isScheduledSession(session, klass.schedules) ? "Lịch tuần" : "Thêm tay"}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
