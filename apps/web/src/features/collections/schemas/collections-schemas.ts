import { z } from "zod";

/**
 * Minimal `billing_periods` shape, duplicated from `features/billing` rather
 * than imported — the two features are owned by different phase-4 workstreams
 * and must not couple through source imports (see `adr.md`). Mirrors
 * `billing.PeriodResponse` (`apps/api/internal/features/billing/dto.go`).
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

/** `collections.ContactBalanceRow`/`ClassCollectionRow.payment_status` (`apps/api/internal/features/collections/model.go`). */
export const paymentStatusSchema = z.enum(["unpaid", "partial", "paid"]);

export type PaymentStatus = z.infer<typeof paymentStatusSchema>;

/** `collections.ContactChildInvoiceRow` (`apps/api/internal/features/collections/dto.go`). */
export const contactChildInvoiceRowSchema = z.object({
  invoice_id: z.string(),
  student_name: z.string(),
  total_due: z.number(),
  paid_amount: z.number(),
  outstanding: z.number(),
});

export type ContactChildInvoiceRow = z.infer<typeof contactChildInvoiceRowSchema>;

/**
 * `collections.ContactBalanceRow` — the by-contact view's row shape, one per
 * family merging every child's invoice under a single balance.
 */
export const contactBalanceRowSchema = z.object({
  contact_id: z.string(),
  full_name: z.string(),
  // Null when the caller may not see the contact's phone (phone privacy:
  // owner/oversight or an assigned hoc_vu see it, other staff do not).
  phone: z.string().nullable(),
  contact_archived: z.boolean(),
  student_count: z.number().int(),
  total_due: z.number(),
  total_paid: z.number(),
  outstanding: z.number(),
  payment_status: paymentStatusSchema,
  invoices: z.array(contactChildInvoiceRowSchema),
});

export type ContactBalanceRow = z.infer<typeof contactBalanceRowSchema>;

/**
 * `collections.ClassCollectionRow` — the by-class view's row shape, one per
 * invoice line for the requested class. `line_*` describes just this line;
 * the `invoice_*` fields describe the whole invoice the line belongs to.
 */
export const classCollectionRowSchema = z.object({
  invoice_id: z.string(),
  student_id: z.string(),
  student_name: z.string(),
  contact_id: z.string(),
  contact_name: z.string(),
  class_name: z.string(),
  billable_count: z.number().int(),
  absent_count: z.number().int(),
  line_amount: z.number(),
  invoice_opening_balance: z.number(),
  invoice_total_due: z.number(),
  invoice_paid_amount: z.number(),
  invoice_outstanding: z.number(),
  payment_status: paymentStatusSchema,
});

export type ClassCollectionRow = z.infer<typeof classCollectionRowSchema>;

/** `collections.SummaryResponse` — one unfiltered, unpaginated period total. */
export const collectionsSummarySchema = z.object({
  student_count: z.number().int(),
  contact_count: z.number().int(),
  total_due: z.number(),
  total_paid: z.number(),
  total_outstanding: z.number(),
  paid_contact_count: z.number().int(),
  unpaid_contact_count: z.number().int(),
  partial_contact_count: z.number().int(),
  unallocated_credit: z.number(),
});

export type CollectionsSummary = z.infer<typeof collectionsSummarySchema>;

/** `payments.RecordPaymentRequest.method` (`docs/schema_design.sql:365`). */
export const paymentMethodSchema = z.enum(["cash", "transfer", "other"]);

export type PaymentMethod = z.infer<typeof paymentMethodSchema>;

/** `payment_allocations.allocated_by` (`docs/schema_design.sql:392`). */
export const allocatedBySchema = z.enum(["auto", "manual"]);

export type AllocatedBy = z.infer<typeof allocatedBySchema>;

/** `payments.AllocationResponse`. */
export const allocationResponseSchema = z.object({
  invoice_id: z.string(),
  student_id: z.string(),
  student_name: z.string(),
  period_id: z.string(),
  amount: z.number(),
  allocated_by: allocatedBySchema,
  total_due: z.number(),
  paid_amount: z.number(),
  outstanding: z.number(),
});

export type AllocationResponse = z.infer<typeof allocationResponseSchema>;

/**
 * `payments.PaymentResponse`. There is no allocation *preview* endpoint on
 * the real API — `POST /payments` always auto-allocates via the D8 rule
 * (oldest debt first) and returns the resulting split directly on this
 * response; a preview is this same shape requested, then optionally
 * corrected through `PUT /payments/:id/allocations`.
 */
export const paymentResponseSchema = z.object({
  id: z.string(),
  contact_id: z.string(),
  amount: z.number(),
  method: paymentMethodSchema,
  received_on: z.string(),
  reference_code: z.string().nullable().optional(),
  note: z.string().nullable().optional(),
  reverses_payment_id: z.string().nullable().optional(),
  reversed_at: z.string().nullable().optional(),
  allocations: z.array(allocationResponseSchema),
  unallocated_amount: z.number(),
  created_at: z.string(),
});

export type PaymentResponse = z.infer<typeof paymentResponseSchema>;

/** `payments.RecordPaymentRequest` (`POST /payments` body). */
export const recordPaymentInputSchema = z.object({
  contact_id: z.string().min(1),
  amount: z.number().int().positive("Số tiền phải lớn hơn 0"),
  method: paymentMethodSchema,
  received_on: z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Ngày phải theo định dạng YYYY-MM-DD"),
  reference_code: z.string().trim().max(50, "Tối đa 50 ký tự").optional(),
  note: z.string().trim().max(1000, "Tối đa 1000 ký tự").optional(),
});

export type RecordPaymentInput = z.infer<typeof recordPaymentInputSchema>;

/** One line of `payments.ReallocateRequest` (`PUT /payments/:id/allocations`). */
export const reallocationLineSchema = z.object({
  invoice_id: z.string().min(1),
  amount: z.number().int().positive(),
});

export type ReallocationLine = z.infer<typeof reallocationLineSchema>;

export const reallocateInputSchema = z.object({
  allocations: z.array(reallocationLineSchema).min(1),
});

export type ReallocateInput = z.infer<typeof reallocateInputSchema>;

/**
 * `notifications.BulkSendRequest.purpose` accepts the singular "statement"
 * spelling too, normalized server-side onto "statements" — the app only ever
 * sends the plural form, but the type stays permissive to match the real
 * binding contract.
 */
export const notificationPurposeSchema = z.enum(["statement", "statements", "reminder"]);

export type NotificationPurpose = z.infer<typeof notificationPurposeSchema>;

/**
 * `notifications.channel` CHECK constraint (`docs/schema_design.sql:439`);
 * `zalo_personal` rows are auto-delivered by a background run instead of the
 * teacher's copy-paste.
 */
export const notificationChannelSchema = z.enum([
  "zalo_manual",
  "zalo_zns",
  "sms",
  "zalo_personal",
]);

export type NotificationChannel = z.infer<typeof notificationChannelSchema>;

/** `notifications.status` CHECK constraint (`docs/schema_design.sql:442`). */
export const notificationStatusSchema = z.enum(["queued", "sent", "delivered", "failed"]);

export type NotificationStatus = z.infer<typeof notificationStatusSchema>;

/**
 * `notifications.BulkSendRow` — the only shape the real API ever returns
 * `message_text` on. The list endpoint (`notificationRowSchema` below)
 * intentionally has no text field; the backend never persists rendered text.
 */
export const bulkSendRowSchema = z.object({
  notification_id: z.string(),
  contact_id: z.string(),
  contact_name: z.string(),
  phone: z.string(),
  channel: notificationChannelSchema,
  purpose: notificationPurposeSchema,
  status: notificationStatusSchema,
  message_text: z.string(),
  url: z.string(),
  collapsed: z.boolean(),
});

export type BulkSendRow = z.infer<typeof bulkSendRowSchema>;

/**
 * `notifications.BulkSendResponse`. `run_id` is null unless a `zalo_personal`
 * send actually queued mapped contacts into a background run;
 * `personal_queued_count`/`fallback_manual_count` split that send's rows into
 * auto-delivered vs left-for-copy-paste (both zero on other channels).
 */
export const bulkSendResponseSchema = z.object({
  queued_count: z.number().int(),
  skipped_paid_count: z.number().int(),
  collapsed_count: z.number().int(),
  run_id: z.string().nullable(),
  personal_queued_count: z.number().int(),
  fallback_manual_count: z.number().int(),
  bulk_text: z.string(),
  rows: z.array(bulkSendRowSchema),
  // Set when the other statement dimension (family vs class copy) already
  // sent this period — parents may get both. Informational, never blocking;
  // omitempty on the wire.
  overlap_warning: z.string().optional(),
});

export type BulkSendResponse = z.infer<typeof bulkSendResponseSchema>;

/**
 * `notifications.RunSnapshotResponse` (`GET /billing-periods/:id/notifications/run`).
 * A period that never had a run answers `active: false` with a null `run_id`
 * — an ordinary answer, not an error. `status`/`purpose` are omitempty on the
 * wire, so they are optional here, never null.
 */
export const runSnapshotSchema = z.object({
  active: z.boolean(),
  run_id: z.string().nullable(),
  status: z.enum(["running", "completed", "interrupted", "expired"]).optional(),
  purpose: notificationPurposeSchema.optional(),
  total: z.number().int(),
  sent: z.number().int(),
  failed: z.number().int(),
});

export type RunSnapshot = z.infer<typeof runSnapshotSchema>;

/**
 * `notifications.NotificationResponse` (`GET /billing-periods/:id/notifications`).
 * No `message_text`/`url` — those exist only on a fresh `bulkSendRowSchema`
 * row, never replayable from the ledger.
 */
export const notificationRowSchema = z.object({
  id: z.string(),
  contact_id: z.string(),
  contact_name: z.string(),
  // Null when the caller may not see the contact's phone — see
  // contactBalanceRowSchema.phone.
  phone: z.string().nullable(),
  channel: notificationChannelSchema,
  purpose: notificationPurposeSchema,
  status: notificationStatusSchema,
  // A failed row's teacher-facing reason; omitempty on the wire, so optional
  // here, never null.
  error_message: z.string().optional(),
  // The paced run this row was queued into; absent on manual/copy-paste rows.
  // Lets the UI pin a run's banner to exactly its own rows.
  run_id: z.string().optional(),
  sent_at: z.string().nullable(),
  created_at: z.string(),
});

export type NotificationRow = z.infer<typeof notificationRowSchema>;

/** `notifications.BulkSendRequest` (`POST /billing-periods/:id/notifications/bulk` body). */
export const bulkSendInputSchema = z.object({
  purpose: z.enum(["statements", "reminder"]),
  channel: notificationChannelSchema.optional(),
  // Switches the send onto the class dimension: that class's class-scoped
  // statement copies go out instead of the family statements. Gated by the
  // caller's class-send access, not the center-wide send permission.
  class_id: z.string().optional(),
});

export type BulkSendInput = z.infer<typeof bulkSendInputSchema>;

/** `notifications.MarkSentRequest` (`POST /notifications/mark-sent` body). */
export const markSentInputSchema = z.object({
  ids: z.array(z.string()).min(1),
});

export type MarkSentInput = z.infer<typeof markSentInputSchema>;

/** `notifications.SendPreviewContact` — one contact of a pre-send preview bucket. */
export const sendPreviewContactSchema = z.object({
  contact_id: z.string(),
  contact_name: z.string(),
});

export type SendPreviewContact = z.infer<typeof sendPreviewContactSchema>;

/**
 * `notifications.SendPreviewResponse`
 * (`GET /billing-periods/:id/notifications/preview`) — the server-computed
 * buckets a `zalo_personal` bulk send would produce right now: mapped+friend
 * (auto-send), mapped but not a friend of the caller's Zalo (may not reach),
 * unmapped (manual copy-paste fallback). Computed from the FULL target set
 * intersected with the caller's live friend list, so the numbers hold past
 * any roster page cap; `max_run_size` is the server's cap on one run's
 * auto-send count.
 */
export const sendPreviewSchema = z.object({
  auto_send: z.array(sendPreviewContactSchema),
  mapped_not_friend: z.array(sendPreviewContactSchema),
  unmapped: z.array(sendPreviewContactSchema),
  max_run_size: z.number().int(),
  // Mirrors bulkSendResponseSchema.overlap_warning for the pre-send dialog.
  overlap_warning: z.string().optional(),
});

export type SendPreview = z.infer<typeof sendPreviewSchema>;
