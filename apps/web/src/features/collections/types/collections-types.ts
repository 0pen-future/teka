import type { PaymentStatus } from "../schemas/collections-schemas";

/** `?view=` on `/collections/:periodId` — contact is the PRD-stated default. */
export const COLLECTIONS_VIEWS = ["contact", "class"] as const;
export type CollectionsView = (typeof COLLECTIONS_VIEWS)[number];

/** `?status=` filter chip value; `undefined`/absent means "Tất cả". */
export type CollectionsStatusFilter = PaymentStatus | undefined;

/** `GET /billing-periods/:id/collections` query params, `view=contact` (default). */
export interface ListContactCollectionsParams {
  status?: PaymentStatus;
  q?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** Same endpoint, `view=class` — `class_id` is required server-side (422 without it). */
export interface ListClassCollectionsParams {
  class_id: string;
  status?: PaymentStatus;
  q?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}
