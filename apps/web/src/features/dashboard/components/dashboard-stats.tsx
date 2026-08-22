import { useCurrentPeriod } from "@/features/billing";
import { useCollectionsSummary } from "@/features/collections";
import { useClassesList } from "@/features/roster";
import { cn, formatMoney } from "@/lib/utils";

import { useClassPeriodSessions, usePeriodPreview, useStudentsTotal } from "../hooks/use-dashboard";

interface StatCardProps {
  label: string;
  value: string;
  sub: string;
  /** Tint the sub-line coral — a fetch failed and `value` is not real data. */
  error?: boolean;
}

function StatCard({ label, value, sub, error = false }: StatCardProps) {
  return (
    <div className="rounded-[var(--radius-lg)] bg-white px-[18px] py-4 shadow-soft-md">
      <p className="text-[12.5px] font-extrabold tracking-[0.3px] text-ink-400">{label}</p>
      <p className="mt-[2px] font-display text-[26px] font-extrabold text-ink-900">{value}</p>
      <p className={cn("text-[12.5px]", error ? "font-semibold text-coral-600" : "text-ink-500")}>
        {sub}
      </p>
    </div>
  );
}

/** Placeholder while a stat's backing query is still in flight. */
const LOADING = "…";
/** A stat whose backing fetch failed must not look like it is still loading. */
const failedStat = { value: "—", sub: "Không tải được", error: true };

/**
 * The prototype `home` screen's four stat cards: roster size, the current
 * period's attendance completion, its billable total (side-effect-free
 * preview read), and collections progress. Collections only exist after the
 * period closes, so the fourth card shows a call-to-close until then.
 */
export function DashboardStats({ className }: { className?: string }) {
  const { data: period, isError: periodError } = useCurrentPeriod();
  const { data: classesPage, isError: classesError } = useClassesList({
    status: "active",
    per_page: 100,
  });
  const classes = classesPage?.items ?? [];

  const studentsTotal = useStudentsTotal();
  const sessionQueries = useClassPeriodSessions(classes, period);
  const preview = usePeriodPreview(period);
  const isClosed = period?.status === "closed";
  const summary = useCollectionsSummary(isClosed ? period.id : undefined);

  const sessionsError = classesError || sessionQueries.some((query) => query.isError);
  const sessionsLoaded =
    Boolean(classesPage) && !sessionsError && sessionQueries.every((query) => query.data);
  const periodSessions = sessionQueries.flatMap((query) => query.data ?? []);
  const countable = periodSessions.filter((session) => session.status !== "cancelled");
  const confirmed = countable.filter((session) => session.attendance_confirmed_at).length;

  const monthLabel = period ? String(period.month) : "";
  const periodLabel = period ? String(period.month).padStart(2, "0") : "";

  const studentsStat: StatCardProps = {
    label: "HỌC SINH",
    ...(studentsTotal.isError || classesError
      ? failedStat
      : {
          value: studentsTotal.data != null ? String(studentsTotal.data) : LOADING,
          sub: classesPage ? `${classesPage.meta.total} lớp đang chạy` : LOADING,
        }),
  };

  const attendanceStat: StatCardProps = {
    label: `ĐIỂM DANH THÁNG ${monthLabel}`.trim(),
    ...(periodError || sessionsError
      ? failedStat
      : !sessionsLoaded
        ? { value: LOADING, sub: LOADING }
        : {
            value:
              countable.length === 0 ? "—" : `${Math.round((confirmed / countable.length) * 100)}%`,
            sub: `${confirmed}/${countable.length} buổi đã xác nhận`,
          }),
  };

  const dueStat: StatCardProps = {
    label: `PHẢI THU KỲ ${periodLabel}`.trim(),
    ...(periodError || preview.isError
      ? failedStat
      : {
          value: preview.data ? formatMoney(preview.data.totals.total_due) : LOADING,
          sub: period ? (isClosed ? "Đã chốt sổ" : "Chưa chốt sổ") : LOADING,
        }),
  };

  const collectedStat: StatCardProps = {
    label: "ĐÃ THU",
    ...(periodError || summary.isError
      ? failedStat
      : !period
        ? { value: LOADING, sub: LOADING }
        : !isClosed
          ? { value: "—", sub: "Chốt sổ để bắt đầu thu" }
          : summary.data
            ? {
                value: formatMoney(summary.data.total_paid),
                sub: `còn ${formatMoney(summary.data.total_outstanding)}`,
              }
            : { value: LOADING, sub: LOADING }),
  };

  return (
    <div className={cn("grid grid-cols-2 gap-[14px] lg:grid-cols-4", className)}>
      {[studentsStat, attendanceStat, dueStat, collectedStat].map((stat) => (
        <StatCard key={stat.label} {...stat} />
      ))}
    </div>
  );
}
