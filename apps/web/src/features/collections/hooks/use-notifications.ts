import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  bulkSendNotifications,
  listNotifications,
  markNotificationsSent,
  type ListNotificationsParams,
} from "../api/notifications-api";
import type { BulkSendInput, MarkSentInput } from "../schemas/collections-schemas";

export const notificationsKeys = {
  all: ["notifications"] as const,
  lists: () => [...notificationsKeys.all, "list"] as const,
  list: (periodId: string, params: ListNotificationsParams) =>
    [...notificationsKeys.lists(), periodId, params] as const,
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
