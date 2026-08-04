import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";
import { ApiError } from "@/lib/api/errors";

import {
  adjustmentInputSchema,
  adjustmentResponseSchema,
  blockingSessionsResponseSchema,
  closeResponseSchema,
  periodSchema,
  reviewSchema,
  type Adjustment,
  type AdjustmentInput,
  type BlockingSession,
  type CloseResponse,
  type Period,
  type Review,
} from "../schemas/billing-schemas";

/**
 * There is no `GET .../current` endpoint — `POST /billing-periods`
 * (`billing.ensurePeriod`, `apps/api/internal/features/billing/handler.go`)
 * is an idempotent create-or-get keyed on `(teacher, year, month)`, so
 * calling it with today's calendar month is the sanctioned way to resolve
 * "the current period" from the client.
 */
export async function getCurrentPeriod(): Promise<Period> {
  const now = new Date();
  const res = await apiClient.post<unknown>("/billing-periods", {
    year: now.getFullYear(),
    month: now.getMonth() + 1,
  });
  return parseData(periodSchema, res.data);
}

export async function getPeriod(periodId: string): Promise<Period> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}`);
  return parseData(periodSchema, res.data);
}

/**
 * The period switcher only ever needs the current period plus the one before
 * it (plan open question 3) — `per_page=2` sorted newest-first is exactly
 * that page.
 */
export async function getPeriods(): Promise<Paginated<Period>> {
  const res = await apiClient.get<unknown>("/billing-periods", {
    params: { per_page: 2, sort: "-period_start" },
  });
  return parseList(periodSchema, res.data);
}

/**
 * The review payload. `GET .../preview` and `POST .../draft` return the
 * identical `PreviewResponse` shape, so the choice is only about side effects:
 *
 * - Open period → `POST .../draft` (`billing.Service.Draft`). Draft persists
 *   the computed rows as invoices/invoice_lines and is the only endpoint that
 *   returns a real `invoice_id` per student, which `AdjustmentDialog` needs to
 *   target `POST /invoices/:invoiceId/adjustments`. Idempotent.
 * - Closed period → `GET .../preview`. Draft rejects a closed period with 409
 *   ("period is closed"); preview is a pure read valid in any state and the
 *   read-only closed view never needs `invoice_id` (adjustments are disabled).
 *
 * The 409 fallback also self-heals the brief window right after a close, when a
 * review refetch can still fire with the pre-close `closed=false` before the
 * period query re-resolves as closed.
 */
export async function getReview(periodId: string, closed: boolean): Promise<Review> {
  if (!closed) {
    try {
      const res = await apiClient.post<unknown>(`/billing-periods/${periodId}/draft`);
      return parseData(reviewSchema, res.data);
    } catch (err) {
      if (!(err instanceof ApiError) || err.status !== 409) {
        throw err;
      }
      // Period was closed concurrently — fall through to the read-only preview.
    }
  }
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/preview`);
  return parseData(reviewSchema, res.data);
}

/**
 * The review response carries no `blocking_sessions` field server-side; this
 * mirrors the exact predicate `close.go`'s `blockingSessions()` applies
 * before allowing a close (`from=period_start`, `to=period_end`,
 * `GET /sessions/pending`), so the close button can be disabled proactively
 * instead of only after a 409. `limit=200` (the endpoint's cap) keeps the
 * gate accurate even for a roster whose blocking list is unusually long —
 * the server-side close check is authoritative regardless.
 */
export async function getBlockingSessions(
  periodStart: string,
  periodEnd: string,
): Promise<BlockingSession[]> {
  const res = await apiClient.get<unknown>("/sessions/pending", {
    params: { from: periodStart, to: periodEnd, limit: 200 },
  });
  return parseData(blockingSessionsResponseSchema, res.data).items;
}

/**
 * Irreversible: locks the period, hard-blocks (409) on any past session
 * without confirmed attendance, then issues every drafted invoice with money
 * owed and voids the rest. There is no reopen.
 */
export async function closePeriod(periodId: string): Promise<CloseResponse> {
  const res = await apiClient.post<unknown>(`/billing-periods/${periodId}/close`);
  return parseData(closeResponseSchema, res.data);
}

export async function createAdjustment(
  invoiceId: string,
  input: AdjustmentInput,
): Promise<Adjustment> {
  const body = adjustmentInputSchema.parse(input);
  const res = await apiClient.post<unknown>(`/invoices/${invoiceId}/adjustments`, body);
  return parseData(adjustmentResponseSchema, res.data);
}
