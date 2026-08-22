import { Link } from "react-router";

import { HvButton, HvCard } from "@/components/hv";

import type { NotificationRow, RunSnapshot } from "../schemas/collections-schemas";

interface RunProgressBannerProps {
  snapshot: RunSnapshot;
  /** Contacts the personal send left for copy-paste; 0 hides the note. */
  fallbackManualCount: number;
  /** Ledger rows the run failed to deliver, each carrying its reason. */
  failedRows: NotificationRow[];
  onResume: () => void;
  resuming: boolean;
  /** Hides the banner of a finished run; only offered on terminal states. */
  onDismiss?: () => void;
}

/**
 * Progress of the period's zalo_personal run. The snapshot is the only
 * source: it survives reloads, so this banner also restores a run started in
 * a tab that has since been closed.
 */
export function RunProgressBanner({
  snapshot,
  fallbackManualCount,
  failedRows,
  onResume,
  resuming,
  onDismiss,
}: RunProgressBannerProps) {
  if (!snapshot.run_id) {
    return null;
  }

  const processed = snapshot.sent + snapshot.failed;
  const remaining = snapshot.total - processed;

  return (
    <HvCard variant="flat" className="flex flex-col gap-2">
      {snapshot.status === "running" ? (
        <p className="font-display text-[14px] font-bold text-ink-900">
          Đang gửi {processed}/{snapshot.total}…
        </p>
      ) : null}

      {snapshot.status === "interrupted" ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="font-display text-[14px] font-bold text-ink-900">
            Lượt gửi bị gián đoạn — còn {remaining} chưa gửi.
          </p>
          <HvButton variant="secondary" size="sm" onClick={onResume} disabled={resuming}>
            {resuming ? "Đang gửi…" : "Gửi tiếp"}
          </HvButton>
        </div>
      ) : null}

      {snapshot.status === "completed" || snapshot.status === "expired" ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="font-display text-[14px] font-bold text-ink-900">
            Đã gửi {snapshot.sent} · Lỗi {snapshot.failed}
          </p>
          {onDismiss ? (
            <HvButton variant="ghost" size="sm" onClick={onDismiss}>
              Ẩn
            </HvButton>
          ) : null}
        </div>
      ) : null}

      {snapshot.status === "expired" ? (
        <p className="text-[13px] text-coral">
          Phiên Zalo đã hết hạn.{" "}
          <Link to="/profile" className="font-bold underline">
            Quét lại mã
          </Link>
        </p>
      ) : null}

      {fallbackManualCount > 0 ? (
        <p className="text-[13px] text-ink-500">
          {fallbackManualCount} phụ huynh chưa liên kết — dùng copy-paste bên dưới.
        </p>
      ) : null}

      {failedRows.length > 0 ? (
        <ul className="flex flex-col gap-1">
          {failedRows.map((row) => (
            <li key={row.id} className="text-[13px] text-coral">
              {row.contact_name}: {row.error_message ?? "Không gửi được"}
            </li>
          ))}
        </ul>
      ) : null}
    </HvCard>
  );
}
