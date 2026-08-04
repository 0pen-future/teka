import { formatMoney } from "@/lib/utils";

import type { StatementQr, StatementTotals } from "../types/statement-types";
import { PaymentQr } from "./payment-qr";

export interface GrandTotalProps {
  totals: StatementTotals;
  qr: StatementQr | null;
}

/**
 * The visually dominant `--surface-dark` block — the single number a parent
 * came for, `totals.total_due`, plus the transfer QR right underneath it so
 * paying is one glance away from seeing the amount.
 */
export function GrandTotal({ totals, qr }: GrandTotalProps) {
  return (
    <div className="flex flex-col gap-4 rounded-[var(--radius-xl)] bg-surface-dark p-5 text-white">
      <div className="flex flex-col gap-1">
        <span className="text-[12px] font-bold tracking-[var(--tracking-wide)] uppercase opacity-70">
          Tổng cộng cả gia đình
        </span>
        <span className="font-display text-[30px] font-extrabold text-sun-400">
          {formatMoney(totals.total_due)}
        </span>
        {totals.outstanding === 0 ? (
          <span className="mt-1 inline-flex w-fit items-center gap-1 rounded-[var(--radius-pill)] bg-mint-400/25 px-3 py-1 text-[12px] font-bold text-mint-100">
            ✓ Đã thanh toán
          </span>
        ) : totals.paid > 0 ? (
          // Partial payment: reconcile the headline with the QR, which encodes
          // only the outstanding amount. Without this line a family that has
          // paid some of the total sees a large "total" beside a QR asking for
          // a smaller number, with nothing to explain the gap. All figures come
          // from the server; nothing is summed here.
          <div className="mt-1 flex flex-col gap-0.5 text-[13px]">
            <span className="flex justify-between opacity-70">
              <span>Đã thanh toán</span>
              <span>{formatMoney(totals.paid)}</span>
            </span>
            <span className="flex justify-between font-bold">
              <span>Còn lại</span>
              <span className="text-sun-400">{formatMoney(totals.outstanding)}</span>
            </span>
          </div>
        ) : null}
      </div>
      <PaymentQr qr={qr} />
    </div>
  );
}
