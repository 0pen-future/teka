import { HvCard, StatusPill } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import type { ClassCollectionRow } from "../schemas/collections-schemas";

export interface ClassCollectionGroupProps {
  className: string;
  rows: ClassCollectionRow[];
}

/**
 * One class's invoice lines — each row is one student's charge for this
 * class, with the whole invoice's paid/outstanding shown alongside so an
 * underpaying, multi-class family still shows where the shortfall landed
 * (R7 AC 4) even from a single-class view.
 */
export function ClassCollectionGroup({ className, rows }: ClassCollectionGroupProps) {
  const collected = rows.reduce((total, row) => total + row.invoice_paid_amount, 0);
  const due = rows.reduce((total, row) => total + row.invoice_total_due, 0);

  return (
    <HvCard variant="flat" padding="sm" className="flex flex-col gap-0 overflow-hidden p-0">
      <div className="flex items-center justify-between bg-mint-50 px-4 py-3">
        <p className="font-display text-[15px] font-bold text-mint-600">{className}</p>
        <p className="text-[13px] text-ink-500">
          {formatMoney(collected)} / {formatMoney(due)}
        </p>
      </div>
      <div className="flex flex-col divide-y divide-line-100">
        {rows.map((row) => (
          <div key={row.invoice_id} className="flex items-center justify-between gap-3 px-4 py-3">
            <div>
              <p className="text-[14px] font-bold text-ink-900">{row.student_name}</p>
              <p className="text-[13px] text-ink-400">{row.contact_name}</p>
              {row.invoice_outstanding > 0 ? (
                <p className="text-[13px] text-coral-600">
                  thiếu {formatMoney(row.invoice_outstanding)}
                </p>
              ) : null}
            </div>
            <StatusPill status={row.payment_status} />
          </div>
        ))}
      </div>
    </HvCard>
  );
}
