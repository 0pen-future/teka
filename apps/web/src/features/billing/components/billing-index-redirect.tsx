import { Navigate } from "react-router";

import { useCurrentPeriod } from "../hooks/use-billing";

/**
 * `billing` (no period id) redirects to `/billing/:currentPeriodId` once the
 * current period resolves — `useCurrentPeriod` is the idempotent
 * create-or-get for today's calendar month, so this always lands somewhere.
 */
export function BillingIndexRedirect() {
  const { data: period, isPending, isError } = useCurrentPeriod();

  if (isPending) {
    return <p className="p-4 text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (isError || !period) {
    return (
      <p className="p-4 text-[14px] font-semibold text-coral-600">
        Không tải được kỳ thu học phí hiện tại.
      </p>
    );
  }

  return <Navigate to={`/billing/${period.id}`} replace />;
}
