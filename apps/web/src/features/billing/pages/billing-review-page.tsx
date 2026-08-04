import { useState } from "react";
import { Link, useParams } from "react-router";

import { HvButton, hvToast } from "@/components/hv";
import { cn, formatMoney } from "@/lib/utils";

import { AdjustmentDialog } from "../components/adjustment-dialog";
import { BlockingSessionsPanel } from "../components/blocking-sessions-panel";
import { ClosePeriodDialog } from "../components/close-period-dialog";
import { PeriodSwitcher } from "../components/period-switcher";
import { ReviewCardList } from "../components/review-card-list";
import { ReviewTable } from "../components/review-table";
import { useBlockingSessions, useClosePeriod, usePeriod, useReview } from "../hooks/use-billing";
import type { ReviewRow } from "../schemas/billing-schemas";

/**
 * Manually composed to match `HvButton` variant="secondary" size="sm" —
 * `HvButton` renders a `<button>`, which can't nest inside this footer's
 * `<Link>` to the notifications screen without invalid nested-interactive
 * markup (same constraint as `BlockingSessionsPanel`'s link-button).
 */
const secondaryLinkButtonClassName = cn(
  "inline-flex min-h-[44px] select-none items-center justify-center gap-2 rounded-[var(--radius-md)]",
  "border-0 bg-sky-300 px-[18px] font-display text-[length:var(--text-sm)] font-bold text-white",
  "shadow-press-sky transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] ease-[var(--ease-out)]",
  "hover:brightness-[1.04] active:translate-y-[var(--press-depth)] active:shadow-none",
  "focus-visible:outline-none focus-visible:ring-4",
);

/**
 * The chốt sổ (close) screen (PRD R3, R4). Composes the period switcher,
 * totals header, blocking-sessions gate, the review table/cards, and the
 * close action — mirroring `AttendancePage`'s loading/error/mutation
 * structure.
 */
export function BillingReviewPage() {
  const params = useParams<{ periodId: string }>();
  const periodId = params.periodId;

  const { data: period, isPending: periodPending, isError: periodError } = usePeriod(periodId);
  // Review reads through draft (open) or preview (closed), so it must wait for
  // the period's status; until then the query is disabled and reports pending.
  const {
    data: review,
    isPending: reviewPending,
    isError: reviewError,
  } = useReview(periodId, period?.status);
  const { data: blockingSessions } = useBlockingSessions(
    periodId,
    period?.period_start,
    period?.period_end,
  );
  const closeMutation = useClosePeriod(periodId ?? "");

  const [adjustRow, setAdjustRow] = useState<ReviewRow | null>(null);
  const [closeDialogOpen, setCloseDialogOpen] = useState(false);

  if (!periodId) {
    return null;
  }

  // Period errors are checked before the loading gate: while `period` is
  // erroring the review query stays disabled (and thus perpetually "pending"),
  // which would otherwise trap the page on the loading state.
  if (periodError) {
    return (
      <p className="p-4 text-[14px] font-semibold text-coral-600">
        Không tải được kỳ thu học phí này.
      </p>
    );
  }

  if (periodPending || reviewPending) {
    return <p className="p-4 text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (reviewError || !period || !review) {
    return (
      <p className="p-4 text-[14px] font-semibold text-coral-600">
        Không tải được kỳ thu học phí này.
      </p>
    );
  }

  const closed = period.status === "closed";
  const blocked = !closed && (blockingSessions?.length ?? 0) > 0;
  const contactCount = new Set(review.invoices.map((row) => row.contact_id)).size;

  function handleConfirmClose() {
    closeMutation.mutate(undefined, {
      onSuccess: () => {
        setCloseDialogOpen(false);
        hvToast("Đã chốt sổ kỳ này");
      },
      onError: () => {
        setCloseDialogOpen(false);
        hvToast("Không thể chốt sổ — vui lòng tải lại và thử lại", { variant: "danger" });
      },
    });
  }

  return (
    <div className="flex flex-col gap-4 pb-24">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <p className="font-display text-[18px] font-bold text-ink-900">
            Chốt sổ tháng {period.month}/{period.year}
          </p>
          <p className="text-[13px] text-ink-400">{closed ? "Kỳ đã khoá" : "Kỳ đang mở"}</p>
        </div>
        <PeriodSwitcher currentPeriodId={periodId} />
      </div>

      {blocked ? <BlockingSessionsPanel sessions={blockingSessions ?? []} /> : null}

      {review.invoices.length === 0 ? (
        <p className="rounded-[var(--radius-xl)] bg-white p-8 text-center text-[14px] text-ink-500">
          Kỳ này chưa có buổi học nào
        </p>
      ) : (
        <>
          <div className="hidden sm:block">
            <ReviewTable rows={review.invoices} closed={closed} onAdjust={setAdjustRow} />
          </div>
          <div className="sm:hidden">
            <ReviewCardList rows={review.invoices} closed={closed} onAdjust={setAdjustRow} />
          </div>
        </>
      )}

      <div className="fixed inset-x-0 bottom-14 z-10 flex flex-wrap items-center justify-between gap-3 border-t border-line-200 bg-white p-3 md:bottom-0">
        <span className="text-[13px] text-ink-400">
          {review.totals.student_count} học sinh · {contactCount} phụ huynh
        </span>
        <span className="font-display text-[22px] font-bold text-mint-600">
          {formatMoney(review.totals.total_due)}
        </span>
        {closed ? (
          <div className="flex items-center gap-2">
            <span className="rounded-[var(--radius-pill)] bg-mint-50 px-3 py-1.5 font-display text-[13px] font-bold text-mint-600">
              ✓ Đã chốt — kỳ đã khóa
            </span>
            <Link to={`/notifications/${periodId}`} className={secondaryLinkButtonClassName}>
              Gửi thông báo →
            </Link>
          </div>
        ) : (
          <HvButton
            type="button"
            variant="primary"
            disabled={blocked}
            onClick={() => setCloseDialogOpen(true)}
          >
            Chốt kỳ &amp; tạo phiếu thu
          </HvButton>
        )}
      </div>

      <AdjustmentDialog
        open={adjustRow !== null}
        onOpenChange={(open) => {
          if (!open) {
            setAdjustRow(null);
          }
        }}
        periodId={periodId}
        row={adjustRow}
      />

      <ClosePeriodDialog
        open={closeDialogOpen}
        onOpenChange={setCloseDialogOpen}
        studentCount={review.totals.student_count}
        contactCount={contactCount}
        grandTotal={review.totals.total_due}
        blocked={blocked}
        pending={closeMutation.isPending}
        onConfirm={handleConfirmClose}
      />
    </div>
  );
}
