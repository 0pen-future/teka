import { HvButton } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

import { MoneyField } from "./money-field";
import type { AllocationResponse } from "../schemas/collections-schemas";

export interface AllocationLine {
  invoice_id: string;
  student_name: string;
  amount: number;
}

export interface AllocationEditorProps {
  /** The server's default D8 split — oldest debt first, then this period's charge. */
  defaultAllocations: AllocationResponse[];
  /** Current, possibly-edited lines. */
  value: AllocationLine[];
  /** The payment's total amount — the editor blocks submit unless lines sum to exactly this. */
  amountTotal: number;
  onChange: (next: AllocationLine[]) => void;
  disabled?: boolean;
}

/**
 * One row per child invoice, prefilled with the server's proposed split.
 * Editing any row is the caller's cue to flip the whole payment to
 * `allocated_by: "manual"` on submit — the default split itself is never
 * recomputed here, only ever requested from and returned by the server.
 */
export function AllocationEditor({
  defaultAllocations,
  value,
  amountTotal,
  onChange,
  disabled,
}: AllocationEditorProps) {
  const sum = value.reduce((total, line) => total + line.amount, 0);
  const remainder = amountTotal - sum;

  function setLineAmount(invoiceId: string, amount: number) {
    onChange(value.map((line) => (line.invoice_id === invoiceId ? { ...line, amount } : line)));
  }

  function resetToDefault() {
    onChange(
      defaultAllocations.map((allocation) => ({
        invoice_id: allocation.invoice_id,
        student_name: allocation.student_name,
        amount: allocation.amount,
      })),
    );
  }

  return (
    <div className="rounded-[var(--radius-lg)] bg-cream-100 p-3">
      <div className="flex items-center justify-between">
        <p className="text-[11px] font-bold uppercase tracking-wide text-ink-400">
          Phân bổ tự động — nợ cũ trước, rồi kỳ này
        </p>
        <HvButton
          type="button"
          variant="ghost"
          size="sm"
          onClick={resetToDefault}
          disabled={disabled}
        >
          Dùng phân bổ mặc định
        </HvButton>
      </div>
      <div className="mt-2 flex flex-col gap-2">
        {value.map((line) => (
          <div key={line.invoice_id} className="flex items-center justify-between gap-3">
            <span className="text-[14px] text-ink-700">{line.student_name}</span>
            <div className="w-36">
              <MoneyField
                id={`allocation-${line.invoice_id}`}
                value={line.amount}
                onChange={(next) => setLineAmount(line.invoice_id, next)}
                disabled={disabled}
              />
            </div>
          </div>
        ))}
      </div>
      <div className="mt-2 flex items-center justify-between border-t border-line-200 pt-2">
        <span className="text-[13px] text-ink-400">Còn lại chưa phân bổ</span>
        <span
          className={
            remainder === 0
              ? "font-display text-[14px] font-bold text-mint-600"
              : "font-display text-[14px] font-bold text-coral-600"
          }
        >
          {formatMoney(remainder)}
        </span>
      </div>
    </div>
  );
}
