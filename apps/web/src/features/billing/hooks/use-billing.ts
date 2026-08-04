import { useQuery } from "@tanstack/react-query";

import { getCurrentPeriod } from "../api/billing-api";

export const billingKeys = {
  all: ["billing"] as const,
  periods: () => [...billingKeys.all, "period"] as const,
  currentPeriod: () => [...billingKeys.periods(), "current"] as const,
};

/**
 * Resolves the teacher's current billing period, creating it on first call
 * of a new month (`getCurrentPeriod` is an idempotent ensure, safe to call
 * repeatedly). A five-minute staleTime avoids re-issuing the create-or-get
 * request on every remount of a component that reads it.
 */
export function useCurrentPeriod() {
  return useQuery({
    queryKey: billingKeys.currentPeriod(),
    queryFn: getCurrentPeriod,
    staleTime: 5 * 60 * 1000,
  });
}
