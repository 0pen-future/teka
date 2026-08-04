import { HvButton, hvToast } from "@/components/hv";
import { copyToClipboard } from "@/lib/utils";

import type { BulkSendRow } from "../schemas/collections-schemas";

export interface MessageCardProps {
  row: BulkSendRow;
  onMarkSent: (row: BulkSendRow) => void;
  marking?: boolean;
}

function initial(name: string): string {
  return name.trim().charAt(0).toUpperCase() || "?";
}

/**
 * One card per contact — never per child, satisfying R7 AC5. `message_text`
 * is rendered exactly as returned by `POST .../notifications/bulk`; this
 * component only ever displays and copies it, never recomputes it.
 */
export function MessageCard({ row, onMarkSent, marking }: MessageCardProps) {
  const sent = row.status === "sent";

  async function handleCopy() {
    const ok = await copyToClipboard(row.message_text);
    hvToast(ok ? "Đã sao chép" : "Không thể sao chép", { variant: ok ? "success" : "danger" });
  }

  return (
    <div className="flex flex-col gap-3 rounded-[var(--radius-lg)] bg-cream-0 p-4 shadow-card">
      <div className="flex items-center gap-3">
        <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full bg-sky-100">
          <span className="font-display text-[16px] font-bold text-sky-600">
            {initial(row.contact_name)}
          </span>
        </div>
        <div className="min-w-0 flex-1">
          <p className="truncate font-display text-[15px] font-bold text-ink-900">
            {row.contact_name}
          </p>
          <p className="text-[13px] text-ink-400">{row.phone}</p>
        </div>
        {sent ? (
          <span className="shrink-0 rounded-[var(--radius-pill)] bg-mint-50 px-3 py-1 text-[13px] font-bold text-mint-600">
            ✓ Đã gửi
          </span>
        ) : null}
      </div>
      <div className="whitespace-pre-wrap rounded-[var(--radius-lg)] bg-sky-50 p-3 text-[13px] text-ink-700">
        {row.message_text}
      </div>
      <div className="flex items-center gap-2">
        <HvButton
          variant={sent ? "ghost" : "secondary"}
          size="sm"
          onClick={() => void handleCopy()}
        >
          Sao chép
        </HvButton>
        <HvButton
          variant={sent ? "ghost" : "primary"}
          size="sm"
          disabled={sent || marking}
          onClick={() => onMarkSent(row)}
        >
          {sent ? "Đã gửi" : marking ? "Đang lưu…" : "Đã gửi"}
        </HvButton>
      </div>
    </div>
  );
}
