import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  bulkSendResponseSchema,
  notificationRowSchema,
  runSnapshotSchema,
  sendPreviewSchema,
  type BulkSendInput,
  type BulkSendResponse,
  type MarkSentInput,
  type NotificationRow,
  type RunSnapshot,
  type SendPreview,
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
}

/**
 * `GET /billing-periods/:id/notifications` — delivery bookkeeping only, no
 * `message_text`/`url`: the backend never persists rendered text (see
 * `notificationRowSchema`'s doc comment). Used to read back sent/queued
 * status after reload; the message body itself only ever comes from a fresh
 * `bulkSendNotifications` call. Unlike the other list endpoints this one is
 * unpaginated — the handler answers a bare array with no `meta` block, so it
 * only accepts filters, never `page`/`per_page`.
 */
export async function listNotifications(
  periodId: string,
  params: ListNotificationsParams = {},
): Promise<NotificationRow[]> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/notifications`, {
    params,
  });
  return parseArray(notificationRowSchema, res.data);
}

/**
 * `GET /billing-periods/:id/notifications/preview` — pure read; guarded like
 * BulkSend (reports oversight required, so never call it for a plain member)
 * and it intersects with the caller's LIVE Zalo friend list, so only fetch it
 * when the confirm dialog actually needs the buckets.
 */
export async function getSendPreview(
  periodId: string,
  purpose: "statements" | "reminder",
): Promise<SendPreview> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/notifications/preview`, {
    params: { purpose },
  });
  return parseData(sendPreviewSchema, res.data);
}

/** `POST /notifications/mark-sent` — idempotent; unknown/foreign/already-sent ids are silently skipped. */
export async function markNotificationsSent(input: MarkSentInput): Promise<void> {
  await apiClient.post("/notifications/mark-sent", input);
}

/**
 * `GET /billing-periods/:id/notifications/run` — the period's latest
 * `zalo_personal` run with progress counters. A period without any run
 * answers `active: false`, not a 404.
 */
export async function getNotificationRun(periodId: string): Promise<RunSnapshot> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/notifications/run`);
  return parseData(runSnapshotSchema, res.data);
}

/**
 * `POST /billing-periods/:id/notifications/run/resume` — restarts the
 * period's interrupted run over its still-queued rows only. 409 when the run
 * is not interrupted, another run is sending, or the Zalo session expired.
 */
export async function resumeNotificationRun(periodId: string): Promise<RunSnapshot> {
  const res = await apiClient.post<unknown>(
    `/billing-periods/${periodId}/notifications/run/resume`,
  );
  return parseData(runSnapshotSchema, res.data);
}
