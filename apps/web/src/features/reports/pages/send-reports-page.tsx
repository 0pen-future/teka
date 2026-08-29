import { Link } from "react-router";

import { HvBadge, HvCard } from "@/components/hv";

import { useReportPeriods } from "../hooks/use-reports";
import type { ReportPeriod } from "../schemas/reports-schemas";

interface TeacherGroup {
  teacherId: string;
  teacherName: string;
  periods: ReportPeriod[];
}

/**
 * Groups the newest-first period list by owning teacher, keeping teachers in
 * first-appearance order so whoever has the most recent period leads the page.
 */
function groupByTeacher(periods: ReportPeriod[]): TeacherGroup[] {
  const groups = new Map<string, TeacherGroup>();
  for (const period of periods) {
    const group = groups.get(period.teacher_id);
    if (group) {
      group.periods.push(period);
    } else {
      groups.set(period.teacher_id, {
        teacherId: period.teacher_id,
        teacherName: period.teacher_name ?? "Giáo viên",
        periods: [period],
      });
    }
  }
  return [...groups.values()];
}

/**
 * "Gửi báo cáo" — the send-reports permission holder's entry surface: every
 * center teacher's billing periods grouped by teacher, each row opening the
 * existing send page (`/notifications/:periodId`), which handles generation,
 * channel choice, and the pre-send preview. Read-only here by design: no
 * close/payment/roster affordances for the secretary.
 */
export function SendReportsPage() {
  const { data, isPending, isError } = useReportPeriods();

  return (
    <div>
      <h1 className="font-display text-[26px] font-extrabold text-ink-900">Gửi báo cáo</h1>
      <p className="mt-1 text-[14px] text-ink-500">
        Chọn kỳ của một giáo viên để gửi thông báo học phí hoặc nhắc nợ cho phụ huynh.
      </p>

      <div className="mt-[18px] flex flex-col gap-4">
        {isPending ? (
          <p className="text-[14px] text-ink-400">Đang tải…</p>
        ) : isError ? (
          <p className="text-[14px] text-ink-500">Không tải được danh sách kỳ học phí.</p>
        ) : (data?.items.length ?? 0) === 0 ? (
          <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
            Chưa có kỳ học phí nào trong trung tâm.
          </HvCard>
        ) : (
          groupByTeacher(data?.items ?? []).map((group) => (
            <HvCard key={group.teacherId}>
              <p className="font-display text-[16px] font-bold text-ink-900">{group.teacherName}</p>
              <ul className="mt-2 flex flex-col divide-y divide-line-200">
                {group.periods.map((period) => (
                  <li key={period.id}>
                    <Link
                      to={`/notifications/${period.id}`}
                      aria-label={`Gửi báo cáo tháng ${period.month}/${period.year} của ${group.teacherName}`}
                      className="flex items-center justify-between gap-3 rounded-[10px] px-2 py-3 transition-colors hover:bg-cream-100"
                    >
                      <span className="font-display text-[14px] font-bold text-ink-900">
                        Tháng {period.month}/{period.year}
                      </span>
                      <span className="flex items-center gap-2">
                        <HvBadge
                          variant={period.status === "open" ? "success" : "neutral"}
                          size="sm"
                        >
                          {period.status === "open" ? "Đang mở" : "Đã chốt"}
                        </HvBadge>
                        <span aria-hidden className="text-ink-300">
                          →
                        </span>
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </HvCard>
          ))
        )}
      </div>
    </div>
  );
}
