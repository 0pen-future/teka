import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  bulkSendNotifications,
  getNotificationRun,
  listNotifications,
  markNotificationsSent,
  resumeNotificationRun,
  type ListNotificationsParams,
} from "../api/notifications-api";
import type { BulkSendInput, MarkSentInput, RunSnapshot } from "../schemas/collections-schemas";

/** How often an in-flight zalo_personal run is polled for progress. */
export const RUN_POLL_INTERVAL_MS = 2000;

export const notificationsKeys = {
  all: ["notifications"] as const,
  lists: () => [...notificationsKeys.all, "list"] as const,
  list: (periodId: string, params: ListNotificationsParams) =>
    [...notificationsKeys.lists(), periodId, params] as const,
  run: (periodId: string) => [...notificationsKeys.all, "run", periodId] as const,
};

/**
 * Reads back persisted delivery bookkeeping only (`status`/`sent_at`) — the
 * real API never persists `message_text`, so this query cannot supply the
 * message body. It is used to decide whether a batch has ever been
 * generated for this period+purpose and to show the sent/total summary.
 */
export function useNotificationsList(
  periodId: string | undefined,
  params: ListNotificationsParams = {},
) {
  return useQuery({
    queryKey: notificationsKeys.list(periodId ?? "", params),
    queryFn: () => listNotifications(periodId!, params),
    enabled: Boolean(periodId),
  });
}

/**
 * `POST /billing-periods/:id/notifications/bulk` — the only source of
 * message text. Not idempotent: every call inserts fresh ledger rows for
 * every eligible contact, so this must only run from an explicit teacher
 * action (initial generate or an explicit "Tạo lại"), never a background
 * refetch or effect that could re-run silently.
 */
export function useBulkSendNotifications(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: BulkSendInput) => bulkSendNotifications(periodId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: notificationsKeys.lists() });
      // A zalo_personal send starts a background run; refetching the snapshot
      // here is what kicks the progress poll off.
      void queryClient.invalidateQueries({ queryKey: notificationsKeys.run(periodId) });
    },
  });
}

/**
 * The period's latest zalo_personal run. One fetch on mount restores a run
 * that survived a closed tab; the interval only exists while the snapshot
 * says the run is still sending, so a runless period is read exactly once
 * and a finished run never leaves a timer polling the API.
 */
export function useNotificationRun(periodId: string | undefined) {
  const queryClient = useQueryClient();
  return useQuery({
    queryKey: notificationsKeys.run(periodId ?? ""),
    queryFn: async () => {
      const previous = queryClient.getQueryData<RunSnapshot>(notificationsKeys.run(periodId ?? ""));
      const snapshot = await getNotificationRun(periodId!);
      // The run flips ledger rows to delivered/failed server-side; refresh
      // them while it sends — and once more on the poll that sees it finish —
      // so statuses update without a reload.
      const justFinished = previous?.status === "running" && snapshot.status !== "running";
      if (snapshot.status === "running" || justFinished) {
        void queryClient.invalidateQueries({ queryKey: notificationsKeys.lists() });
      }
      return snapshot;
    },
    enabled: Boolean(periodId),
    refetchInterval: (query) =>
      query.state.data?.status === "running" ? RUN_POLL_INTERVAL_MS : false,
  });
}

/** "Gửi tiếp" on an interrupted run — the run resumes over its still-queued rows. */
export function useResumeNotificationRun(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => resumeNotificationRun(periodId),
    onSuccess: (snapshot) => {
      // Seeding the snapshot (now running again) restarts the poll interval.
      queryClient.setQueryData(notificationsKeys.run(periodId), snapshot);
    },
  });
}

export function useMarkNotificationsSent(periodId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: MarkSentInput) => markNotificationsSent(input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: notificationsKeys.list(periodId, {}) });
      void queryClient.invalidateQueries({ queryKey: notificationsKeys.lists() });
    },
  });
}
