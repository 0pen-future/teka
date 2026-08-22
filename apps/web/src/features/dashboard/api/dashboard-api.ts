import { reviewSchema, type Review } from "@/features/billing";
import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  pendingSessionsResponseSchema,
  type PendingSessionsResponse,
} from "../schemas/dashboard-schemas";

/**
 * Held sessions whose attendance is not yet confirmed. The server caps
 * `items` (default 50, max 200) but `total` is the true unbounded count, so
 * both are kept: `total` drives the banner headline, `items` the rendered
 * list.
 */
export async function getPendingSessions(): Promise<PendingSessionsResponse> {
  const res = await apiClient.get<unknown>("/sessions/pending");
  return parseData(pendingSessionsResponseSchema, res.data);
}

/**
 * `GET /billing-periods/:id/preview` — the same `PreviewResponse` shape the
 * chốt sổ review screen consumes, but always through the pure read: unlike
 * `billing.getReview`, viewing the dashboard must never `POST .../draft`
 * (which persists invoices as a side effect).
 */
export async function getPeriodPreview(periodId: string): Promise<Review> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/preview`);
  return parseData(reviewSchema, res.data);
}
