import { Link } from "react-router";

import { HvBadge, HvModal } from "@/components/hv";

import { useReportPeriods } from "../hooks/use-reports";

export interface ClassSendPeriodsDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  classId: string;
  className: string;
}

/**
 * The class-scoped send's period picker: lists only the billing periods that
 * carry this class's charges (the server resolves them through the class's
 * invoice lines, so a handed-off class finds its charges under the current
 * teacher's period), each row opening the class-mode send screen. Callers
 * gate the trigger on `canSendClassReports` and mount this only while open —
 * the period query fires on mount.
 */
export function ClassSendPeriodsDialog({
  open,
  onOpenChange,
  classId,
  className,
}: ClassSendPeriodsDialogProps) {
  const { data, isPending, isError } = useReportPeriods(classId);
  const periods = data?.items ?? [];

  return (
    <HvModal open={open} onOpenChange={onOpenChange} title={`Gửi báo cáo — lớp ${className}`}>
      {isPending ? (
        <p className="text-[14px] text-ink-400">Đang tải danh sách kỳ…</p>
      ) : isError ? (
        <p className="text-[14px] text-ink-400">
          Không tải được danh sách kỳ. Đóng và thử lại sau.
        </p>
      ) : periods.length === 0 ? (
        <p className="text-[14px] text-ink-400">
          Chưa có kỳ học phí nào ghi nhận khoản phí của lớp này.
        </p>
      ) : (
        <ul className="flex flex-col divide-y divide-line-200">
          {periods.map((period) => (
            <li key={period.id}>
              <Link
                to={`/notifications/${period.id}?class_id=${classId}`}
                className="flex min-h-11 items-center justify-between gap-3 py-2.5 hover:bg-cream-100"
                aria-label={`Gửi báo cáo lớp ${className} — tháng ${period.month}/${period.year}`}
              >
                <span className="flex flex-col">
                  <span className="text-[14px] font-bold text-ink-900">
                    Tháng {period.month}/{period.year}
                  </span>
                  {period.teacher_name ? (
                    <span className="text-[12px] text-ink-400">{period.teacher_name}</span>
                  ) : null}
                </span>
                <HvBadge variant={period.status === "open" ? "success" : "neutral"} size="sm">
                  {period.status === "open" ? "Đang mở" : "Đã chốt"}
                </HvBadge>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </HvModal>
  );
}
