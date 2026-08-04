import { useState } from "react";
import { useNavigate } from "react-router";

import { HvButton, HvCard, StatusPill } from "@/components/hv";
import { formatMoney, formatPhoneLocal } from "@/lib/utils";

import type { ContactBalanceRow } from "../schemas/collections-schemas";

export interface ContactCollectionRowProps {
  row: ContactBalanceRow;
  periodId: string;
  onRecordPayment: (row: ContactBalanceRow) => void;
}

/**
 * One family per row — total due/paid/outstanding merged across every
 * child. Expanding reveals the per-child invoice breakdown so the
 * by-contact view can answer by-class questions without switching tabs.
 * "Nhắc nợ" links to the notifications screen rather than sending directly:
 * the real reminder endpoint (`POST .../notifications/bulk`,
 * `purpose=reminder`) has no `contact_ids` filter — it always targets every
 * contact with an outstanding balance in one call, so a single-contact
 * reminder trigger does not exist to wire here.
 */
export function ContactCollectionRow({
  row,
  periodId,
  onRecordPayment,
}: ContactCollectionRowProps) {
  const [expanded, setExpanded] = useState(false);
  const navigate = useNavigate();

  return (
    <HvCard variant="flat" className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <button
          type="button"
          onClick={() => setExpanded((value) => !value)}
          className="flex flex-1 items-center gap-3 text-left"
        >
          <div>
            <p className="font-display text-[15px] font-bold text-ink-900">{row.full_name}</p>
            <p className="text-[13px] text-ink-400">
              {row.student_count} con · {formatPhoneLocal(row.phone)}
            </p>
          </div>
        </button>
        <StatusPill status={row.payment_status} />
      </div>
      <div className="flex flex-wrap items-center gap-4 text-[14px]">
        <span className="text-ink-500">
          Phải thu{" "}
          <strong className="font-display text-ink-900">{formatMoney(row.total_due)}</strong>
        </span>
        <span className="text-ink-500">
          Đã thu{" "}
          <strong className="font-display text-mint-600">{formatMoney(row.total_paid)}</strong>
        </span>
        <span className="text-ink-500">
          Còn lại{" "}
          <strong className="font-display text-coral-600">{formatMoney(row.outstanding)}</strong>
        </span>
      </div>
      {expanded ? (
        <div className="flex flex-col gap-2 border-t border-line-200 pt-3">
          {row.invoices.map((invoice) => (
            <div key={invoice.invoice_id} className="flex items-center justify-between text-[13px]">
              <span className="text-ink-700">{invoice.student_name}</span>
              <span className="text-ink-500">
                {formatMoney(invoice.paid_amount)} / {formatMoney(invoice.total_due)}
              </span>
            </div>
          ))}
        </div>
      ) : null}
      <div className="flex flex-wrap items-center gap-2">
        <HvButton variant="primary" size="sm" onClick={() => onRecordPayment(row)}>
          Thu tiền
        </HvButton>
        <HvButton
          variant="ghost"
          size="sm"
          onClick={() => void navigate(`/notifications/${periodId}`)}
        >
          Nhắc nợ
        </HvButton>
      </div>
    </HvCard>
  );
}
