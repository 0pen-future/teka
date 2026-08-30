import { useState } from "react";
import { Link, useParams, useSearchParams } from "react-router";

import { HvBadge, HvButton, HvCard, hvToast, type HvBadgeVariant } from "@/components/hv";
import { ConfirmDialog } from "@/components/shared/confirm-dialog";
import { useZaloStatus } from "@/features/profile";
import { canSendClassReports, useClass } from "@/features/roster";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { copyToClipboard, cn, formatDateTime } from "@/lib/utils";

import { MessageCard } from "../components/message-card";
import { RunProgressBanner } from "../components/run-progress-banner";
import {
  useBulkSendNotifications,
  useMarkNotificationsSent,
  useNotificationRun,
  useNotificationsList,
  useResumeNotificationRun,
  useSendPreview,
} from "../hooks/use-notifications";
import type { BulkSendRow, NotificationRow } from "../schemas/collections-schemas";

type Purpose = "statements" | "reminder";
type SendChannel = "zalo_manual" | "zalo_personal";

const purposeOptions: { value: Purpose; label: string }[] = [
  { value: "statements", label: "Thông báo học phí" },
  { value: "reminder", label: "Nhắc nợ" },
];

const generateLabel: Record<Purpose, string> = {
  statements: "Tạo thông báo học phí",
  reminder: "Tạo nhắc nợ",
};

// Both zalo_personal conflicts come back as a plain 409 CONFLICT; the only
// wire-level discriminator is the server's fixed message text. Keep the
// coupled substring in one visible place.
const EXPIRED_SESSION_409 = "session has expired";

/**
 * The server's overlap warning (the OTHER statement dimension — family vs
 * class copy — already sent this period) arrives as fixed English text; the
 * substring is the only wire discriminator, mirrored from the API messages.
 */
function overlapMessage(warning: string | undefined): string | null {
  if (!warning) {
    return null;
  }
  if (warning.includes("family statements")) {
    return "Kỳ này đã gửi báo cáo học phí cho cả nhà — phụ huynh lớp này có thể nhận hai bản.";
  }
  return "Kỳ này đã có lớp gửi bản báo cáo riêng — một số phụ huynh có thể nhận hai bản.";
}

const ledgerStatusMeta: Record<
  NotificationRow["status"],
  { label: string; variant: HvBadgeVariant }
> = {
  queued: { label: "Chờ gửi", variant: "neutral" },
  sent: { label: "Đã gửi", variant: "success" },
  delivered: { label: "Đã nhận", variant: "success" },
  failed: { label: "Không gửi được", variant: "danger" },
};

const ledgerChannelLabels: Record<NotificationRow["channel"], string> = {
  zalo_manual: "Zalo thủ công",
  zalo_personal: "Zalo tự động",
  zalo_zns: "Zalo ZNS",
  sms: "SMS",
};

/** One read-only ledger line for the plain-member view (D8): who, how, when, status. */
function LedgerRow({ row }: { row: NotificationRow }) {
  const status = ledgerStatusMeta[row.status];
  return (
    <div className="flex items-center justify-between gap-3 py-2.5 first:pt-0 last:pb-0">
      <div className="min-w-0">
        <p className="truncate text-[14px] font-bold text-ink-900">{row.contact_name}</p>
        <p className="text-[12px] text-ink-400">
          {ledgerChannelLabels[row.channel]}
          {row.sent_at ? ` · ${formatDateTime(row.sent_at)}` : ""}
        </p>
      </div>
      <HvBadge variant={status.variant} size="sm">
        {status.label}
      </HvBadge>
    </div>
  );
}

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
  // Class mode: the send switches onto the class dimension — that class's
  // statement copies instead of the family statements — and the send right
  // comes from the caller's hoc_vu stint on the class, not the center-wide
  // send permission. The tab switcher preserves the param.
  const classId = searchParams.get("class_id") ?? undefined;

  const [rows, setRows] = useState<BulkSendRow[] | null>(null);
  const [channel, setChannel] = useState<SendChannel>("zalo_manual");
  const [confirmOpen, setConfirmOpen] = useState(false);
  const [manualConfirmOpen, setManualConfirmOpen] = useState(false);
  const [dismissedRunId, setDismissedRunId] = useState<string | null>(null);

  const bulkSend = useBulkSendNotifications(periodId ?? "", classId);

  // Reset `rows` when the period or purpose changes, following React's
  // "adjusting state when a prop changes" pattern: comparing during render
  // and calling setState there avoids the extra commit an effect would cost.
  // The bulk-send mutation resets alongside: its success/error state belongs
  // to the previous scope's generate, and left alone it would leak that
  // scope's result banner into the fresh tab.
  const scopeKey = `${periodId}:${purpose}:${classId ?? ""}`;
  const [rowsScopeKey, setRowsScopeKey] = useState(scopeKey);
  if (scopeKey !== rowsScopeKey) {
    setRowsScopeKey(scopeKey);
    setRows(null);
    bulkSend.reset();
  }

  const { data: ledger, isPending: ledgerPending } = useNotificationsList(periodId, { purpose });
  const markSent = useMarkNotificationsSent(periodId ?? "");
  const run = useNotificationRun(periodId, classId);
  const resumeRun = useResumeNotificationRun(periodId ?? "", classId);

  const { data: zalo, isError: zaloStatusError } = useZaloStatus();
  const personalReady = zalo?.linked === true && zalo.status === "linked";
  const zaloExpired = zalo?.status === "expired";
  // The link status can flip underneath a picked radio (focus refetch after
  // the session expires) — derive the channel that actually applies instead
  // of trusting stale state, so a disabled option can never fire a send.
  const effectiveChannel: SendChannel = personalReady ? channel : "zalo_manual";

  // This period+purpose's latest run, if any. Everything run-related below
  // (banner, failed rows, duplicate-send warning) keys off this so a run from
  // the other purpose tab never bleeds in.
  const activeRun = run.data?.purpose === purpose && run.data.run_id ? run.data : undefined;

  // Send-affordance gating (D8): only reports oversight (owner or
  // can_send_reports holder) may create sends; a plain member keeps this
  // page read-only over the ledger. In class mode a hoc_vu stint on the
  // class opens the send instead. UX-only — the server is the authority.
  const { canRunSends, isResolved } = useCenterContext();
  const classQuery = useClass(classId);
  const klass = classQuery.data;
  const canSend = classId
    ? canSendClassReports(canRunSends, klass ?? { my_staff_roles: [] })
    : canRunSends;
  // In class mode the stint check needs the class row, so the read-only
  // branch must also wait for it — otherwise the send UI flashes away from a
  // hoc_vu while the class loads.
  const accessResolved = isResolved && (!classId || !classQuery.isPending);

  // The confirm dialog's auto/manual split comes from the server's pre-send
  // preview — the full target set intersected with the caller's live Zalo
  // friend list — fetched only while the dialog is open (each fetch is a live
  // friend-list call, and the endpoint 403s callers without send access).
  const preview = useSendPreview(periodId, purpose, confirmOpen && canSend, classId);
  const autoCount = preview.data?.auto_send.length ?? 0;
  const notFriendCount = preview.data?.mapped_not_friend.length ?? 0;
  const unmappedCount = preview.data?.unmapped.length ?? 0;
  // Mirrors BulkSend's cap: every mapped contact (friend or not) queues into
  // the run, and the server rejects a run larger than max_run_size (0 = no cap).
  const maxRunSize = preview.data?.max_run_size ?? 0;
  const overRunCap = maxRunSize > 0 && autoCount + notFriendCount > maxRunSize;

  function generate() {
    if (!periodId) {
      return;
    }
    if (effectiveChannel === "zalo_personal") {
      setConfirmOpen(true);
      return;
    }
    if (activeRun) {
      // A run already auto-messaged the mapped parents; a full manual batch
      // would hand the teacher copy-paste texts for them a second time.
      setManualConfirmOpen(true);
      return;
    }
    sendManual();
  }

  function sendManual() {
    bulkSend.mutate(
      { purpose, channel: "zalo_manual" },
      {
        onSuccess: (result) => {
          setManualConfirmOpen(false);
          setRows(result.rows);
        },
        onError: () => {
          setManualConfirmOpen(false);
          hvToast("Không thể tạo thông báo", { variant: "danger" });
        },
      },
    );
  }

  function sendPersonal() {
    bulkSend.mutate(
      { purpose, channel: "zalo_personal" },
      {
        onSuccess: (result) => {
          setConfirmOpen(false);
          // The run banner owns the auto-sent rows; only copy-paste rows
          // enter the manual copy/mark-sent flow below.
          setRows(result.rows.filter((row) => row.channel !== "zalo_personal"));
        },
        onError: (error) => {
          setConfirmOpen(false);
          if (error instanceof ApiError && error.status === 409) {
            hvToast(
              error.message.includes(EXPIRED_SESSION_409)
                ? "Phiên Zalo đã hết hạn — quét lại mã ở trang cá nhân"
                : "Đang có lượt gửi chạy, đợi xong đã",
              { variant: "danger" },
            );
            return;
          }
          hvToast("Không thể tạo thông báo", { variant: "danger" });
        },
      },
    );
  }

  function resume() {
    resumeRun.mutate(undefined, {
      onError: (error) =>
        hvToast(
          error instanceof ApiError && error.message.includes(EXPIRED_SESSION_409)
            ? "Phiên Zalo đã hết hạn — quét lại mã ở trang cá nhân"
            : "Không thể gửi tiếp. Thử lại sau.",
          { variant: "danger" },
        ),
    });
  }

  if (!periodId) {
    return null;
  }

  // Nothing has ever been generated for this period+purpose and this session
  // hasn't generated it either — the only way in is the explicit button
  // below, never an effect.
  const showEmptyState = rows === null && !ledgerPending && (ledger?.length ?? 0) === 0;

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

  const header = (
    <div className="flex flex-wrap items-center justify-between gap-3">
      <h1 className="font-display text-[22px] font-bold text-ink-900">
        {classId ? `Gửi thông báo — lớp ${klass?.name ?? "…"}` : "Gửi thông báo"}
      </h1>
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
  );

  // D8: a plain member keeps this page as a read-only ledger — what was sent
  // for their period, by whom-ever held the send right — with every send
  // affordance gone. Gated on `accessResolved` so the send UI never flashes
  // for a member while `/centers/me` (and, in class mode, the class row)
  // loads (the server 403s them regardless).
  if (accessResolved && !canSend) {
    return (
      <div className="flex flex-col gap-4">
        {header}
        <p className="text-[13px] text-ink-400">
          Việc gửi báo cáo do người được giao quyền hoặc chủ trung tâm thực hiện. Dưới đây là các
          thông báo đã tạo cho kỳ này.
        </p>
        {ledgerPending ? (
          <p className="text-[14px] text-ink-400">Đang tải…</p>
        ) : (ledger?.length ?? 0) === 0 ? (
          <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
            Chưa có thông báo nào cho kỳ này.
          </HvCard>
        ) : (
          <HvCard variant="flat" className="flex flex-col divide-y divide-line-200">
            {(ledger ?? []).map((row) => (
              <LedgerRow key={row.id} row={row} />
            ))}
          </HvCard>
        )}
      </div>
    );
  }

  const sentOverlap = overlapMessage(bulkSend.data?.overlap_warning);

  return (
    <div className="flex flex-col gap-4">
      {header}

      {sentOverlap ? (
        <HvCard variant="flat" className="text-[13px] font-bold text-coral-600">
          {sentOverlap}
        </HvCard>
      ) : null}

      <HvCard variant="flat" className="flex flex-col gap-3">
        <div className="flex flex-wrap items-center justify-between gap-3">
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
        </div>
        <fieldset className="flex flex-col gap-1 border-t border-line-200 pt-3">
          <legend className="sr-only">Kênh gửi</legend>
          <div className="flex flex-wrap items-center gap-4">
            <label className="flex items-center gap-2 text-[14px] text-ink-900">
              <input
                type="radio"
                name="send-channel"
                checked={effectiveChannel === "zalo_manual"}
                onChange={() => setChannel("zalo_manual")}
              />
              Zalo thủ công
            </label>
            <label
              className={cn(
                "flex items-center gap-2 text-[14px]",
                personalReady ? "text-ink-900" : "text-ink-400",
              )}
            >
              <input
                type="radio"
                name="send-channel"
                checked={effectiveChannel === "zalo_personal"}
                onChange={() => setChannel("zalo_personal")}
                disabled={!personalReady}
                aria-describedby={personalReady ? undefined : "send-channel-note"}
              />
              Gửi qua Zalo (tự động)
            </label>
          </div>
          {zaloExpired ? (
            <p id="send-channel-note" className="text-[13px] text-coral">
              Phiên Zalo đã hết hạn.{" "}
              <Link to="/profile" className="font-bold underline">
                Quét lại mã
              </Link>
            </p>
          ) : zalo && !zalo.linked ? (
            <p id="send-channel-note" className="text-[13px] text-ink-400">
              <Link to="/profile" className="underline">
                Kết nối Zalo để gửi tự động
              </Link>
            </p>
          ) : zaloStatusError ? (
            <p id="send-channel-note" className="text-[13px] text-ink-400">
              Không kiểm tra được trạng thái Zalo — chỉ gửi thủ công được.
            </p>
          ) : null}
        </fieldset>
      </HvCard>

      {activeRun && activeRun.run_id !== dismissedRunId ? (
        <RunProgressBanner
          snapshot={activeRun}
          fallbackManualCount={bulkSend.data?.fallback_manual_count ?? 0}
          failedRows={(ledger ?? []).filter(
            (row) => row.run_id === activeRun.run_id && row.status === "failed",
          )}
          onResume={resume}
          resuming={resumeRun.isPending}
          onDismiss={() => setDismissedRunId(activeRun.run_id)}
        />
      ) : null}

      {showEmptyState ? (
        <HvCard variant="flat" className="flex flex-col items-center gap-3 py-6 text-center">
          <p className="text-[13px] text-ink-400">Chưa có thông báo nào cho kỳ này.</p>
          <HvButton variant="primary" onClick={generate} disabled={bulkSend.isPending}>
            {bulkSend.isPending ? "Đang tạo…" : generateLabel[purpose]}
          </HvButton>
        </HvCard>
      ) : null}

      {rows !== null && rows.length === 0 && !(bulkSend.data?.personal_queued_count ?? 0) ? (
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

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        title="Gửi tự động qua Zalo?"
        description={
          preview.isPending ? (
            "Đang kiểm tra danh sách bạn bè Zalo…"
          ) : preview.isError ? (
            "Không kiểm tra được danh sách bạn bè Zalo — vẫn gửi được, nhưng tin tới người chưa là bạn có thể không đến nơi."
          ) : (
            <span className="flex flex-col gap-1.5 text-left">
              <span>
                <strong className="font-display text-ink-900">{autoCount}</strong> phụ huynh gửi tự
                động (đã là bạn Zalo).
              </span>
              {notFriendCount > 0 ? (
                <span className="text-coral-600">
                  <strong className="font-display">{notFriendCount}</strong> phụ huynh đã ghép Zalo
                  nhưng chưa là bạn bè của bạn — tin có thể rơi vào hộp thư người lạ hoặc không đến
                  nơi.{" "}
                  <Link to="/contacts" className="font-bold underline">
                    Kết bạn trước
                  </Link>
                </span>
              ) : null}
              <span>
                <strong className="font-display text-ink-900">{unmappedCount}</strong> phụ huynh
                chưa ghép Zalo — dùng copy thủ công.
              </span>
              {overRunCap ? (
                <span className="font-bold text-coral-600">
                  Vượt giới hạn {maxRunSize} tin tự động mỗi lượt — hãy gửi thành các đợt nhỏ hơn.
                </span>
              ) : null}
              {overlapMessage(preview.data?.overlap_warning) ? (
                <span className="font-bold text-coral-600">
                  {overlapMessage(preview.data?.overlap_warning)}
                </span>
              ) : null}
            </span>
          )
        }
        confirmLabel="Gửi"
        pending={bulkSend.isPending}
        confirmDisabled={preview.isPending || overRunCap}
        onConfirm={sendPersonal}
      />

      <ConfirmDialog
        open={manualConfirmOpen}
        onOpenChange={setManualConfirmOpen}
        title="Tạo lại thủ công?"
        description="Kỳ này đã có lượt gửi tự động — tạo lại sẽ tạo tin copy-paste cho cả phụ huynh đã nhận qua Zalo. Tiếp tục?"
        confirmLabel="Tạo lại"
        pending={bulkSend.isPending}
        onConfirm={sendManual}
      />
    </div>
  );
}
