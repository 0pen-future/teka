import { HvButton, HvCard } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import type { ReviewRow } from "../schemas/billing-schemas";

export interface ReviewCardListProps {
  rows: ReviewRow[];
  closed: boolean;
  onAdjust: (row: ReviewRow) => void;
}

/**
 * `< sm` rendering of the same review rows `ReviewTable` shows — both
 * components consume the identical `ReviewRow[]` shape, no second data
 * shape (Implementation Step 4).
 */
export function ReviewCardList({ rows, closed, onAdjust }: ReviewCardListProps) {
  return (
    <div className="flex flex-col gap-3">
      {rows.map((row) => (
        <HvCard key={row.student_id} variant="flat" padding="sm">
          <div className="flex items-start justify-between gap-2">
            <div>
              <p className="font-display text-[15px] font-bold text-ink-900">{row.student_name}</p>
              <p className="text-[12px] text-ink-400">{row.contact_name}</p>
            </div>
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              disabled={closed || !row.invoice_id}
              onClick={() => onAdjust(row)}
            >
              Sửa
            </HvButton>
          </div>

          {row.lines.length > 0 ? (
            <ul className="mt-3 flex flex-col gap-1.5">
              {row.lines.map((line) => (
                <li
                  key={line.enrollment_id}
                  className="flex items-center justify-between rounded-[var(--radius-md)] bg-mint-50 px-2.5 py-1.5"
                >
                  <span className="font-display text-[13px] font-bold text-mint-600">
                    {line.class_name}
                  </span>
                  <span className="text-[12px] text-ink-500">
                    {line.billable_count} buổi · {line.absent_count} vắng
                  </span>
                  <span className="text-[13px] font-semibold text-ink-700">
                    {formatMoney(line.amount)}
                  </span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="mt-3 text-[13px] text-ink-400">Kỳ này chưa có buổi học nào</p>
          )}

          <div className="mt-3 flex flex-col gap-1 border-t border-line-100 pt-3 text-[13px]">
            <div className="flex justify-between">
              <span className="text-ink-500">Nợ cũ</span>
              <span className={row.opening_balance !== 0 ? "font-semibold text-coral-600" : ""}>
                {formatMoney(row.opening_balance)}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="text-ink-500">Điều chỉnh</span>
              <span className={row.adjustment_total !== 0 ? "font-semibold text-sun-600" : ""}>
                {row.adjustment_total > 0 ? "+" : ""}
                {formatMoney(row.adjustment_total)}
              </span>
            </div>
            <div className="flex justify-between">
              <span className="font-display font-bold text-ink-900">Tổng</span>
              <span className="font-display font-bold text-ink-900">
                {formatMoney(row.total_due)}
              </span>
            </div>
          </div>
        </HvCard>
      ))}
    </div>
  );
}
