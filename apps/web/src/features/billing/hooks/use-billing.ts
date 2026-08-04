import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  closePeriod,
  createAdjustment,
  getBlockingSessions,
  getCurrentPeriod,
  getPeriod,
  getPeriods,
  getReview,
} from "../api/billing-api";
import type { AdjustmentInput, Period } from "../schemas/billing-schemas";

export const billingKeys = {
  all: ["billing"] as const,
  periods: () => [...billingKeys.all, "period"] as const,
  currentPeriod: () => [...billingKeys.periods(), "current"] as const,
  periodsList: () => [...billingKeys.periods(), "list"] as const,
  periodDetail: (periodId: string) => [...billingKeys.periods(), "detail", periodId] as const,
  reviews: () => [...billingKeys.all, "review"] as const,
  review: (periodId: string) => [...billingKeys.reviews(), periodId] as const,
  blockingSessions: (periodId: string) =>
    [...billingKeys.all, "blocking-sessions", periodId] as const,
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

/** One period's dates/status, for the review page header and closed-state gating. */
export function usePeriod(periodId: string | undefined) {
  return useQuery({
    queryKey: billingKeys.periodDetail(periodId ?? ""),
    queryFn: () => getPeriod(periodId!),
    enabled: Boolean(periodId),
  });
}

/** Current and previous period, for `PeriodSwitcher`. */
export function usePeriods() {
  return useQuery({
    queryKey: billingKeys.periodsList(),
    queryFn: getPeriods,
  });
}

/**
 * The chốt sổ review screen's rows and totals. `getReview` reads through
 * `POST .../draft` for an open period and `GET .../preview` for a closed one,
 * so the query stays disabled until `status` is known — firing draft against
 * an already-closed period would 409. Modeled as a query (not a mutation)
 * because the page treats it as read-driven data that refetches on
 * adjustment/attendance changes. `refetchOnWindowFocus` is off so merely
 * refocusing the tab never re-issues draft's write.
 */
export function useReview(periodId: string | undefined, status: Period["status"] | undefined) {
  return useQuery({
    queryKey: billingKeys.review(periodId ?? ""),
    queryFn: () => getReview(periodId!, status === "closed"),
    enabled: Boolean(periodId && status),
    staleTime: 30 * 1000,
    refetchOnWindowFocus: false,
  });
}

/**
 * Proactive client-side mirror of the server-side close gate
 * (`close.go`'s `blockingSessions()`), scoped to one period's date range.
 */
export function useBlockingSessions(
  periodId: string | undefined,
  periodStart?: string,
  periodEnd?: string,
) {
  return useQuery({
    queryKey: billingKeys.blockingSessions(periodId ?? ""),
    queryFn: () => getBlockingSessions(periodStart!, periodEnd!),
    enabled: Boolean(periodId && periodStart && periodEnd),
  });
}

export function useCreateAdjustment(invoiceId: string, periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: AdjustmentInput) => createAdjustment(invoiceId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: billingKeys.review(periodId) });
    },
  });
}

/**
 * Closing a period is irreversible and fans out beyond billing's own cache:
 * the review and current-period queries obviously go stale, and so do the
 * collections and notifications screens (phase 4's other half), which only
 * exist once a period is closed. Those keys are matched by predicate rather
 * than imported literally — `features/collections` is a sibling feature
 * built in parallel and billing does not depend on its query-key factory.
 */
export function useClosePeriod(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => closePeriod(periodId),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: billingKeys.review(periodId) });
      void queryClient.invalidateQueries({ queryKey: billingKeys.periodDetail(periodId) });
      void queryClient.invalidateQueries({ queryKey: billingKeys.currentPeriod() });
      void queryClient.invalidateQueries({ queryKey: billingKeys.periodsList() });
      void queryClient.invalidateQueries({
        predicate: (query) =>
          query.queryKey[0] === "collections" || query.queryKey[0] === "notifications",
      });
    },
  });
}
