import { Link } from "react-router";

import { HvBadge, HvCard } from "@/components/hv";
import { Spinner } from "@/components/shared/spinner";
import { useCurrentPeriod } from "@/features/billing";

/**
 * Manually composed to match `HvButton` variant="primary" size="sm" — see
 * the identical note in `pending-attendance-alert.tsx` on why a styled
 * `Link` replaces `HvButton` at navigation call sites.
 */
const primaryLinkButtonClassName =
  "inline-flex min-h-[44px] shrink-0 items-center justify-center rounded-[var(--radius-md)] " +
  "bg-mint-400 px-[18px] text-[length:var(--text-sm)] font-display font-bold text-white " +
  "shadow-press-mint transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] " +
  "ease-[var(--ease-out)] hover:brightness-[1.04] active:translate-y-[var(--press-depth)] " +
  "active:shadow-none focus-visible:outline-none focus-visible:ring-4";

/** The current billing period's status and its one primary next action. */
export function PeriodStatusCard() {
  const { data: period, isPending, isError } = useCurrentPeriod();

  if (isPending) {
    return (
      <HvCard className="flex items-center justify-center">
        <Spinner className="size-5" />
      </HvCard>
    );
  }

  // Same rationale as the pending-attendance alert: a blank card is
  // indistinguishable from "nothing to show" and hides a real fetch failure
  // from the teacher.
  if (isError || !period) {
    return (
      <HvCard>
        <p className="text-[14px] font-semibold text-coral-600">Không tải được kỳ hiện tại</p>
      </HvCard>
    );
  }

  const isOpen = period.status === "open";

  return (
    <HvCard className="flex flex-wrap items-center justify-between gap-4">
      <div>
        <p className="font-display text-[18px] font-bold text-ink-900">
          Tháng {period.month}/{period.year}
        </p>
        <HvBadge variant={isOpen ? "success" : "neutral"} size="sm" className="mt-2">
          {isOpen ? "Đang mở" : "Đã chốt"}
        </HvBadge>
      </div>
      {isOpen ? (
        <Link to={`/billing/${period.id}`} className={primaryLinkButtonClassName}>
          Chốt sổ
        </Link>
      ) : (
        <Link to={`/collections/${period.id}`} className={primaryLinkButtonClassName}>
          Xem thu tiền
        </Link>
      )}
    </HvCard>
  );
}
