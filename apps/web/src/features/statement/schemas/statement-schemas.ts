import { z } from "zod";

/**
 * `GET /public/statements/:token` response shapes. Every money field is a
 * whole-đồng integer computed server-side; the page never sums or multiplies
 * them — a client miscalculation here is how a parent ends up disagreeing
 * with the number their teacher already sent them.
 */

/** One session's attendance status, as it applies to this invoice line. */
export const statementSessionSchema = z.object({
  date: z.string(),
  status: z.string(),
  counted: z.boolean(),
});

export type StatementSession = z.infer<typeof statementSessionSchema>;

/** One enrollment's charge for the period. */
export const statementClassSchema = z.object({
  class_name: z.string(),
  unit_price: z.number().int(),
  billable_count: z.number().int(),
  absent_count: z.number().int(),
  amount: z.number().int(),
  sessions: z.array(statementSessionSchema),
});

export type StatementClass = z.infer<typeof statementClassSchema>;

/** A manual or corrective adjustment on the child's invoice. No reason field is ever exposed publicly. */
export const statementAdjustmentSchema = z.object({
  amount: z.number().int(),
  kind: z.enum(["manual", "correction"]),
});

export type StatementAdjustment = z.infer<typeof statementAdjustmentSchema>;

/** A balance carried in from a prior period's late edit, explained by the sessions it covers. */
export const statementCarriedAdjustmentSchema = z.object({
  amount: z.number().int(),
  session_dates: z.array(z.string()),
});

export type StatementCarriedAdjustment = z.infer<typeof statementCarriedAdjustmentSchema>;

/** One child's full breakdown for the period. */
export const statementChildSchema = z.object({
  student_name: z.string(),
  display_note: z.string().nullable(),
  opening_balance: z.number().int(),
  classes: z.array(statementClassSchema),
  adjustments: z.array(statementAdjustmentSchema),
  carried_adjustment: statementCarriedAdjustmentSchema.nullable(),
  subtotal: z.number().int(),
});

export type StatementChild = z.infer<typeof statementChildSchema>;

/** The family's grand totals — `total_due` is the single number the header/footer display verbatim. */
export const statementTotalsSchema = z.object({
  opening_balance: z.number().int(),
  current_charge: z.number().int(),
  adjustment_total: z.number().int(),
  total_due: z.number().int(),
  paid: z.number().int(),
  outstanding: z.number().int(),
});

export type StatementTotals = z.infer<typeof statementTotalsSchema>;

/** One invoice's payment status inside the family's payment summary. */
export const statementInvoicePaymentSchema = z.object({
  student_name: z.string(),
  total_due: z.number().int(),
  paid: z.number().int(),
  outstanding: z.number().int(),
});

export type StatementInvoicePayment = z.infer<typeof statementInvoicePaymentSchema>;

export const statementPaymentsSchema = z.object({
  total_paid: z.number().int(),
  by_invoice: z.array(statementInvoicePaymentSchema),
});

export type StatementPayments = z.infer<typeof statementPaymentsSchema>;

/**
 * Transfer QR, or `null` when the teacher has not configured a bank account.
 * There is no `bank_name`/`account_number`/`account_holder` — only the image,
 * the amount it encodes, and a copyable transfer note.
 */
export const statementQrSchema = z.object({
  image_url: z.string(),
  amount: z.number().int(),
  note: z.string(),
});

export type StatementQr = z.infer<typeof statementQrSchema>;

export const statementSchema = z.object({
  contact_name: z.string(),
  /** Plain display string, e.g. "08/2026" — not a structured year/month object. */
  period: z.string(),
  children: z.array(statementChildSchema),
  totals: statementTotalsSchema,
  payments: statementPaymentsSchema,
  qr: statementQrSchema.nullable(),
});

export type Statement = z.infer<typeof statementSchema>;
