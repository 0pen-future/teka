import { formatMoney } from "@/lib/utils";

import type { StatementQr } from "../types/statement-types";
import { CopyField } from "./copy-field";

export interface PaymentQrProps {
  qr: StatementQr | null;
}

/**
 * The transfer QR for the grand total. When the teacher has no bank
 * configured, `qr` is `null` and only the copyable note text renders —
 * never a broken `<img>`.
 */
export function PaymentQr({ qr }: PaymentQrProps) {
  if (!qr) {
    return (
      <p className="text-[13px] text-white/70">
        Chưa có mã QR chuyển khoản. Vui lòng liên hệ thầy/cô để biết thông tin chuyển khoản.
      </p>
    );
  }

  return (
    <div className="flex flex-col items-center gap-3">
      <div className="flex size-[150px] items-center justify-center rounded-[var(--radius-lg)] bg-white p-2">
        <img
          src={qr.image_url}
          alt={`Mã QR chuyển khoản ${formatMoney(qr.amount)}`}
          width={140}
          height={140}
          loading="eager"
          className="size-full object-contain"
        />
      </div>
      <p className="font-display text-[17px] font-bold text-white">{formatMoney(qr.amount)}</p>
      <CopyField label="Nội dung chuyển khoản" value={qr.note} tone="dark" />
    </div>
  );
}
