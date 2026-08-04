import { useState } from "react";
import { useParams, useSearchParams } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { copyToClipboard, cn } from "@/lib/utils";

import { MessageCard } from "../components/message-card";
import {
  useBulkSendNotifications,
  useMarkNotificationsSent,
  useNotificationsList,
} from "../hooks/use-notifications";
import type { BulkSendRow } from "../schemas/collections-schemas";

type Purpose = "statements" | "reminder";

const purposeOptions: { value: Purpose; label: string }[] = [
  { value: "statements", label: "Thông báo học phí" },
  { value: "reminder", label: "Nhắc nợ" },
];

const generateLabel: Record<Purpose, string> = {
  statements: "Tạo thông báo học phí",
  reminder: "Tạo nhắc nợ",
};

/**
 * "Gửi thông báo". The real API never persists `message_text` — it exists
 * only on a fresh `POST .../notifications/bulk` response — so the source of
 * truth for what's rendered here is local state (`rows`) populated from that
 * call, not `useNotificationsList` (which only ever carries status/sent_at).
 * `POST .../notifications/bulk` is non-idempotent (always inserts fresh,
 * non-deduplicated ledger rows), so generation is NEVER fired from an effect
 * — an on-mount auto-generate would silently double-write every time this
 * page is re-entered. Both the first generate (empty-ledger empty state) and
 * every regenerate ("Tạo lại") require an explicit click.
 */
export function NotificationsPage() {
  const { periodId } = useParams<{ periodId: string }>();
  const [searchParams, setSearchParams] = useSearchParams();
  const purpose = (searchParams.get("purpose") as Purpose | null) ?? "statements";

  const [rows, setRows] = useState<BulkSendRow[] | null>(null);

  // Reset `rows` when the period or purpose changes, following React's
  // "adjusting state when a prop changes" pattern: comparing during render
  // and calling setState there avoids the extra commit an effect would cost.
  const scopeKey = `${periodId}:${purpose}`;
  const [rowsScopeKey, setRowsScopeKey] = useState(scopeKey);
  if (scopeKey !== rowsScopeKey) {
    setRowsScopeKey(scopeKey);
    setRows(null);
  }

  const { data: ledger, isPending: ledgerPending } = useNotificationsList(periodId, { purpose });
  const bulkSend = useBulkSendNotifications(periodId ?? "");
  const markSent = useMarkNotificationsSent(periodId ?? "");

  function generate() {
    if (!periodId) {
      return;
    }
    bulkSend.mutate(
      { purpose },
      {
        onSuccess: (result) => setRows(result.rows),
        onError: () => hvToast("Không thể tạo thông báo", { variant: "danger" }),
      },
    );
  }

  if (!periodId) {
    return null;
  }

  // Nothing has ever been generated for this period+purpose and this session
  // hasn't generated it either — the only way in is the explicit button
  // below, never an effect.
  const showEmptyState = rows === null && !ledgerPending && (ledger?.items.length ?? 0) === 0;

  const sentCount = rows?.filter((row) => row.status === "sent").length ?? 0;
  const totalCount = rows?.length ?? 0;

  function handleMarkSent(row: BulkSendRow) {
    markSent.mutate(
      { ids: [row.notification_id] },
      {
        onSuccess: () => {
          setRows(
            (current) =>
              current?.map((line) =>
                line.notification_id === row.notification_id ? { ...line, status: "sent" } : line,
              ) ?? null,
          );
          hvToast("Đã đánh dấu đã gửi", { variant: "success" });
        },
        onError: () => hvToast("Không thể đánh dấu đã gửi", { variant: "danger" }),
      },
    );
  }

  async function copyAllUnsent() {
    const unsent = rows?.filter((row) => row.status !== "sent") ?? [];
    if (unsent.length === 0) {
      hvToast("Không có tin nào chưa gửi", { variant: "default" });
      return;
    }
    const blob = unsent.map((row) => `— ${row.contact_name} —\n${row.message_text}`).join("\n\n");
    const ok = await copyToClipboard(blob);
    hvToast(ok ? `Đã sao chép ${unsent.length} tin` : "Không thể sao chép", {
      variant: ok ? "success" : "danger",
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-[22px] font-bold text-ink-900">Gửi thông báo</h1>
        <div className="inline-flex rounded-[var(--radius-pill)] border border-line-200 bg-white p-1">
          {purposeOptions.map((option) => (
            <button
              key={option.value}
              type="button"
              role="tab"
              aria-selected={purpose === option.value}
              onClick={() => {
                const next = new URLSearchParams(searchParams);
                next.set("purpose", option.value);
                setSearchParams(next, { replace: true });
              }}
              className={cn(
                "min-h-9 rounded-[var(--radius-pill)] px-4 font-display text-[14px] font-bold transition-colors",
                purpose === option.value
                  ? "bg-mint-400 text-white"
                  : "text-ink-500 hover:bg-cream-100",
              )}
            >
              {option.label}
            </button>
          ))}
        </div>
      </div>

      <HvCard variant="flat" className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="text-[13px] text-ink-400">Số phụ huynh</p>
          <p className="font-display text-[18px] font-bold text-ink-900">{totalCount}</p>
        </div>
        <p className="text-[14px] text-ink-500">
          <strong className="font-display text-mint-600">{sentCount}</strong>/{totalCount} đã gửi
        </p>
        <div className="flex gap-2">
          <HvButton variant="secondary" size="sm" onClick={() => void copyAllUnsent()}>
            Sao chép tất cả chưa gửi
          </HvButton>
          {!showEmptyState ? (
            <HvButton variant="ghost" size="sm" onClick={generate} disabled={bulkSend.isPending}>
              {bulkSend.isPending ? "Đang tạo…" : rows ? "Tạo lại" : generateLabel[purpose]}
            </HvButton>
          ) : null}
        </div>
      </HvCard>

      {showEmptyState ? (
        <HvCard variant="flat" className="flex flex-col items-center gap-3 py-6 text-center">
          <p className="text-[13px] text-ink-400">Chưa có thông báo nào cho kỳ này.</p>
          <HvButton variant="primary" onClick={generate} disabled={bulkSend.isPending}>
            {bulkSend.isPending ? "Đang tạo…" : generateLabel[purpose]}
          </HvButton>
        </HvCard>
      ) : null}

      {rows !== null && rows.length === 0 ? (
        <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
          Không có phụ huynh nào phù hợp.
        </HvCard>
      ) : null}

      {rows ? (
        <div className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          {rows.map((row) => (
            <MessageCard
              key={row.notification_id}
              row={row}
              onMarkSent={handleMarkSent}
              marking={markSent.isPending}
            />
          ))}
        </div>
      ) : null}
    </div>
  );
}
