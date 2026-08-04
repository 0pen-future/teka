import { HvButton, HvModal } from "@/components/hv";
import { formatMoney } from "@/lib/utils";

export interface ClosePeriodDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  studentCount: number;
  contactCount: number;
  grandTotal: number;
  /** True while `blocking_sessions` is non-empty — disables the confirm action entirely. */
  blocked: boolean;
  pending: boolean;
  onConfirm: () => void;
}

/**
 * `ClosePeriodDialog` (`modalWarn`-adjacent recipe). Closing is irreversible
 * — there is no reopen (`billing.Service.Close`'s doc comment) — so the
 * confirmation states the exact numbers being locked in rather than a bare
 * "are you sure".
 */
export function ClosePeriodDialog({
  open,
  onOpenChange,
  studentCount,
  contactCount,
  grandTotal,
  blocked,
  pending,
  onConfirm,
}: ClosePeriodDialogProps) {
  return (
    <HvModal open={open} onOpenChange={onOpenChange} title="Chốt sổ kỳ này?">
      <div className="flex flex-col gap-3">
        <p className="text-[14px] text-ink-700">
          Sau khi chốt sổ, số liệu sẽ bị khoá và không thể mở lại. Mọi sai sót sau đó chỉ có thể sửa
          bằng điều chỉnh ở kỳ kế tiếp.
        </p>
        <div className="rounded-[var(--radius-lg)] bg-cream-100 p-4">
          <p className="text-[13px] text-ink-500">
            {studentCount} học sinh · {contactCount} phụ huynh sẽ nhận thông báo
          </p>
          <p className="mt-1 font-display text-[22px] font-bold text-mint-600">
            {formatMoney(grandTotal)}
          </p>
        </div>
        {blocked ? (
          <p className="text-[13px] font-semibold text-coral-600">
            Không thể chốt sổ khi còn buổi học chưa điểm danh.
          </p>
        ) : null}
        <HvButton
          type="button"
          variant="primary"
          size="lg"
          block
          disabled={blocked || pending}
          onClick={onConfirm}
        >
          {pending ? "Đang chốt sổ…" : "Chốt kỳ & tạo phiếu thu"}
        </HvButton>
      </div>
    </HvModal>
  );
}
