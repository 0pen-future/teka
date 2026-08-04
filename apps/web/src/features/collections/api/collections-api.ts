import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import type {
  ListClassCollectionsParams,
  ListContactCollectionsParams,
} from "../types/collections-types";
import {
  classCollectionRowSchema,
  collectionsSummarySchema,
  contactBalanceRowSchema,
  paymentResponseSchema,
  periodSchema,
  type ClassCollectionRow,
  type CollectionsSummary,
  type ContactBalanceRow,
  type Period,
  type PaymentResponse,
  type ReallocateInput,
  type RecordPaymentInput,
} from "../schemas/collections-schemas";

/**
 * `GET /billing-periods/:id` (`apps/api/internal/features/billing/handler.go`).
 * Duplicated here rather than imported from `features/billing` — see
 * `periodSchema`'s doc comment.
 */
export async function getPeriod(periodId: string): Promise<Period> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}`);
  return parseData(periodSchema, res.data);
}

/**
 * `GET /billing-periods/:id/collections?view=contact` (default view) —
 * `apps/api/internal/features/collections/handler.go`. One row per family.
 */
export async function listContactCollections(
  periodId: string,
  params: ListContactCollectionsParams = {},
): Promise<Paginated<ContactBalanceRow>> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/collections`, {
    params: { view: "contact", ...params },
  });
  return parseList(contactBalanceRowSchema, res.data);
}

/**
 * `GET /billing-periods/:id/collections?view=class&class_id=` — `class_id` is
 * required server-side; the service returns a 422 without it.
 */
export async function listClassCollections(
  periodId: string,
  params: ListClassCollectionsParams,
): Promise<Paginated<ClassCollectionRow>> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/collections`, {
    params: { view: "class", ...params },
  });
  return parseList(classCollectionRowSchema, res.data);
}

/** `GET /billing-periods/:id/collections/summary` — unfiltered period totals. */
export async function getCollectionsSummary(periodId: string): Promise<CollectionsSummary> {
  const res = await apiClient.get<unknown>(`/billing-periods/${periodId}/collections/summary`);
  return parseData(collectionsSummarySchema, res.data);
}

/**
 * `POST /payments` (`apps/api/internal/features/payments/handler.go`). There
 * is no separate preview endpoint: recording always auto-allocates via the
 * server's D8 rule and returns the resulting split on this same response —
 * that response doubles as the "preview" `RecordPaymentDialog` shows before
 * the teacher optionally overrides it with `reallocatePayment`.
 */
export async function recordPayment(input: RecordPaymentInput): Promise<PaymentResponse> {
  const res = await apiClient.post<unknown>("/payments", input);
  return parseData(paymentResponseSchema, res.data);
}

/**
 * `PUT /payments/:id/allocations` — a teacher's manual override of the
 * default split. Every line's `allocated_by` flips to `"manual"` server-side.
 */
export async function reallocatePayment(
  paymentId: string,
  input: ReallocateInput,
): Promise<PaymentResponse> {
  const res = await apiClient.put<unknown>(`/payments/${paymentId}/allocations`, input);
  return parseData(paymentResponseSchema, res.data);
}
