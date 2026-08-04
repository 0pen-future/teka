import { z } from "zod";

/**
 * `billing.PeriodResponse` (`apps/api/internal/features/billing/dto.go`).
 * `status` is `"open" | "closed"` (`docs/schema_design.sql:263`).
 */
export const periodSchema = z.object({
  id: z.string(),
  year: z.number().int(),
  month: z.number().int(),
  period_start: z.string(),
  period_end: z.string(),
  status: z.enum(["open", "closed"]),
  closed_at: z.string().nullable(),
});

export type Period = z.infer<typeof periodSchema>;

/**
 * `billing.PreviewLine` — one enrollment's class line inside a review row.
 * `class_id`/`present_count` come from the compute step's attendance tally,
 * not the persisted `invoice_lines` row. Lines with zero `billable_count`
 * and zero `absent_count` are already omitted server-side
 * (`buildPreviewResponse`'s doc comment), so every line here is meaningful.
 */
export const invoiceLineSchema = z.object({
  enrollment_id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  billable_count: z.number().int(),
  absent_count: z.number().int(),
  present_count: z.number().int(),
  unit_price: z.number().int(),
  amount: z.number().int(),
});

export type InvoiceLine = z.infer<typeof invoiceLineSchema>;

/**
 * `billing.PreviewInvoice` — one student's computed fee for the period,
 * shared by `GET .../preview` (read-only) and `POST .../draft` (persists).
 * `invoice_id` is null until draft has persisted it; the review screen calls
 * draft, so an authenticated review row always carries a real invoice id to
 * target with adjustments.
 */
export const reviewRowSchema = z.object({
  invoice_id: z.string().nullable(),
  student_id: z.string(),
  contact_id: z.string(),
  student_name: z.string(),
  contact_name: z.string(),
  lines: z.array(invoiceLineSchema),
  opening_balance: z.number().int(),
  current_charge: z.number().int(),
  adjustment_total: z.number().int(),
  total_due: z.number().int(),
});

export type ReviewRow = z.infer<typeof reviewRowSchema>;

/** `billing.PreviewTotals` — the review screen's grand-total footer. */
export const reviewTotalsSchema = z.object({
  student_count: z.number().int(),
  total_opening: z.number().int(),
  total_charge: z.number().int(),
  total_adjustment: z.number().int(),
  total_due: z.number().int(),
});

export type ReviewTotals = z.infer<typeof reviewTotalsSchema>;

/**
 * `billing.PreviewResponse` — the chốt sổ review payload. Neither the
 * period nor a `blocking_sessions` field is part of this shape server-side;
 * the review page fetches the period separately (`usePeriod`) and the
 * blocking gate separately (`useBlockingSessions`, mirroring the exact
 * predicate `close.go`'s `blockingSessions()` applies server-side via
 * `GET /sessions/pending?from=&to=`).
 */
export const reviewSchema = z.object({
  invoices: z.array(reviewRowSchema),
  totals: reviewTotalsSchema,
});

export type Review = z.infer<typeof reviewSchema>;

/**
 * `sessions.PendingSessionResponse`
 * (`apps/api/internal/features/sessions/pending.go`) scoped to one period's
 * date range. Reimplemented locally rather than imported from the dashboard
 * feature — dashboard has no barrel `index.ts`, so its schema isn't part of
 * any feature's public surface.
 */
export const blockingSessionSchema = z.object({
  session_id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  session_date: z.string(),
  start_time: z.string().nullable(),
  status: z.enum(["planned", "held", "cancelled"]),
  expected_student_count: z.number().int(),
  days_overdue: z.number().int(),
});

export type BlockingSession = z.infer<typeof blockingSessionSchema>;

/** `sessions.PendingResponse` — the wire body of `GET /sessions/pending`. */
export const blockingSessionsResponseSchema = z.object({
  total: z.number().int(),
  items: z.array(blockingSessionSchema),
});

/**
 * `billing.UnconfirmedSession` — the shape of both a 409 close-blocked
 * response's `unconfirmed_sessions` detail and a successful close's
 * `future_unconfirmed_sessions` warning. Distinct from
 * `blockingSessionSchema` (`sessions.PendingSessionResponse`): this one has
 * no `start_time`/`expected_student_count`/`days_overdue`.
 */
export const unconfirmedSessionSchema = z.object({
  session_id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  session_date: z.string(),
  status: z.string(),
});

export type UnconfirmedSession = z.infer<typeof unconfirmedSessionSchema>;

/** `billing.CloseWarnings`. */
export const closeWarningsSchema = z.object({
  future_unconfirmed_sessions: z.array(unconfirmedSessionSchema),
});

/** `billing.CloseResponse` — `POST /billing-periods/:id/close`'s success payload. */
export const closeResponseSchema = z.object({
  period: periodSchema,
  issued_count: z.number().int(),
  voided_count: z.number().int(),
  total_due: z.number().int(),
  warnings: closeWarningsSchema,
});

export type CloseResponse = z.infer<typeof closeResponseSchema>;

/**
 * Client-side form validation for `POST /invoices/:id/adjustments`, mirroring
 * `billing.AdjustmentRequest`'s server rules exactly: `amount` is signed and
 * rejects zero (`AddAdjustment`'s `{"amount": "must not be zero"}`), `reason`
 * is required and length-bounded 3–500 runes (the `invoice_adjustments.reason`
 * non-blank CHECK, `docs/schema_design.sql:343,351`).
 */
export const adjustmentInputSchema = z.object({
  amount: z
    .number()
    .int()
    .refine((value) => value !== 0, "Số tiền không được bằng 0"),
  reason: z
    .string()
    .trim()
    .min(3, "Lý do phải có ít nhất 3 ký tự")
    .max(500, "Lý do tối đa 500 ký tự"),
});

export type AdjustmentInput = z.infer<typeof adjustmentInputSchema>;

/** `billing.AdjustmentResponse` — one row of the append-only adjustment audit trail. */
export const adjustmentResponseSchema = z.object({
  id: z.string(),
  invoice_id: z.string(),
  amount: z.number().int(),
  reason: z.string(),
  source_session_id: z.string().nullable(),
  created_at: z.string(),
});

export type Adjustment = z.infer<typeof adjustmentResponseSchema>;
