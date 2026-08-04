import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import {
  bulkSendResponseSchema,
  notificationRowSchema,
  type BulkSendInput,
  type BulkSendResponse,
  type MarkSentInput,
  type NotificationRow,
} from "../schemas/collections-schemas";

/**
 * `POST /billing-periods/:id/notifications/bulk`
 * (`apps/api/internal/features/notifications/handler.go`). There is no
 * `contact_ids` field on the real request — it always applies to every
 * eligible contact for `purpose` (statements = every contact with a
 * non-void invoice; reminder = further narrowed to outstanding > 0), which
 * is also why a family with several indebted children still gets exactly
 * one reminder row (one row per `contact_id`, never per child). Each call
 * inserts fresh notification rows; there is no dedup/idempotency on the
 * backend, so this must only run from an explicit teacher action, never a
 * background refetch.
 */
export async function bulkSendNotifications(
  periodId: string,
  input: BulkSendInput,
): Promise<BulkSendResponse> {
  const res = await apiClient.post<unknown>(
    `/billing-periods/${periodId}/notifications/bulk`,
    input,
  );
  return parseData(bulkSendResponseSchema, res.data);
}

export interface ListNotificationsParams {
  purpose?: "statements" | "reminder";
  status?: "queued" | "sent" | "delivered" | "failed";
  page?: number;
  per_page?: number;
}

/**
 * `GET /billing-periods/:id/notifications` — delivery bookkeeping only, no
 * `message_text`/`url`: the backend never persists rendered text (see
 * `notificationRowSchema`'s doc comment). Used to read back sent/queued
 * status after reload; the message body itself only ever comes from a fresh
 * `bulkSendNotifications` call.
 */
export async function listNotifications(
  periodId: string,
  params: ListNotificationsParams = {},
): Promise<Paginated<NotificationRow>> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/notifications`, {
    params,
  });
  return parseList(notificationRowSchema, res.data);
}

/** `POST /notifications/mark-sent` — idempotent; unknown/foreign/already-sent ids are silently skipped. */
export async function markNotificationsSent(input: MarkSentInput): Promise<void> {
  await apiClient.post("/notifications/mark-sent", input);
}
