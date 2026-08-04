import { http, HttpResponse } from "msw";

import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  Adjustment,
  BlockingSession,
  CloseResponse,
  InvoiceLine,
  Period,
  ReviewRow,
} from "../schemas/billing-schemas";

/**
 * Fixture dates key off the current month so the period stays "open" and
 * inside a stable window as real time moves on, following
 * `attendance-handlers.ts`'s `daysAgo()` convention.
 */
function monthRange(): { start: string; end: string } {
  const now = new Date();
  const year = now.getFullYear();
  const month = now.getMonth();
  const start = new Date(Date.UTC(year, month, 1)).toISOString().slice(0, 10);
  const end = new Date(Date.UTC(year, month + 1, 0)).toISOString().slice(0, 10);
  return { start, end };
}

function daysAgo(count: number): string {
  const date = new Date();
  date.setDate(date.getDate() - count);
  return date.toISOString().slice(0, 10);
}

const { start: periodStart, end: periodEnd } = monthRange();

export const fixturePeriodOpen: Period = {
  id: "80000000-0000-4000-8000-000000000001",
  year: new Date().getFullYear(),
  month: new Date().getMonth() + 1,
  period_start: periodStart,
  period_end: periodEnd,
  status: "open",
  closed_at: null,
};

/** Two-class student (R1 AC 2 / R4): one row, two class lines, one total. */
const multiClassLines: InvoiceLine[] = [
  {
    enrollment_id: "enrollment-toan",
    class_id: "class-toan",
    class_name: "Toán 6A",
    billable_count: 4,
    absent_count: 1,
    present_count: 3,
    unit_price: 150000,
    amount: 600000,
  },
  {
    enrollment_id: "enrollment-van",
    class_id: "class-van",
    class_name: "Văn 6A",
    billable_count: 3,
    absent_count: 0,
    present_count: 3,
    unit_price: 120000,
    amount: 360000,
  },
];

/** Single-class student carrying nợ cũ from a previous closed period. */
const carriedDebtLines: InvoiceLine[] = [
  {
    enrollment_id: "enrollment-toan-b",
    class_id: "class-toan",
    class_name: "Toán 6A",
    billable_count: 4,
    absent_count: 0,
    present_count: 4,
    unit_price: 150000,
    amount: 600000,
  },
];

function buildRows(adjustmentByInvoice: Map<string, number>): ReviewRow[] {
  const multiClassAdjustment = adjustmentByInvoice.get("invoice-multi") ?? 0;
  const carriedDebtAdjustment = adjustmentByInvoice.get("invoice-debt") ?? 0;
  return [
    {
      invoice_id: "invoice-multi",
      student_id: "student-multi",
      contact_id: "contact-multi",
      student_name: "Nguyễn Văn An",
      contact_name: "Nguyễn Thị Bình",
      lines: multiClassLines,
      opening_balance: 0,
      current_charge: 960000,
      adjustment_total: multiClassAdjustment,
      total_due: 960000 + multiClassAdjustment,
    },
    {
      invoice_id: "invoice-debt",
      student_id: "student-debt",
      contact_id: "contact-debt",
      student_name: "Trần Thị Cúc",
      contact_name: "Trần Văn Dũng",
      lines: carriedDebtLines,
      opening_balance: 200000,
      current_charge: 600000,
      adjustment_total: carriedDebtAdjustment,
      total_due: 200000 + 600000 + carriedDebtAdjustment,
    },
  ];
}

/**
 * The `PreviewResponse` both `/draft` and `/preview` return — identical shape,
 * differing only in whether `invoice_id` is populated (`false` = the preview
 * read, which nulls it as the real backend does).
 */
function buildReviewResponse(nullInvoiceIds: boolean) {
  const rows = buildRows(store.adjustmentsByInvoice);
  const invoices = nullInvoiceIds ? rows.map((row) => ({ ...row, invoice_id: null })) : rows;
  const totals = invoices.reduce(
    (acc, row) => ({
      student_count: acc.student_count + 1,
      total_opening: acc.total_opening + row.opening_balance,
      total_charge: acc.total_charge + row.current_charge,
      total_adjustment: acc.total_adjustment + row.adjustment_total,
      total_due: acc.total_due + row.total_due,
    }),
    { student_count: 0, total_opening: 0, total_charge: 0, total_adjustment: 0, total_due: 0 },
  );
  return { invoices, totals };
}

export const fixtureBlockingSession: BlockingSession = {
  session_id: "session-blocking-001",
  class_id: "class-toan",
  class_name: "Toán 6A",
  session_date: daysAgo(3),
  start_time: "18:00",
  status: "planned",
  expected_student_count: 28,
  days_overdue: 3,
};

interface Store {
  periodStatus: "open" | "closed";
  blockingSessions: BlockingSession[];
  adjustmentsByInvoice: Map<string, number>;
}

function seedStore(): Store {
  return {
    periodStatus: "open",
    blockingSessions: [],
    adjustmentsByInvoice: new Map(),
  };
}

let store = seedStore();

export function resetBillingStore() {
  store = seedStore();
}

/** Test hook: makes the close gate report a blocking session. */
export function seedBlockingSession() {
  store.blockingSessions = [fixtureBlockingSession];
}

/** Test hook: starts the period already closed (drives the read-only view). */
export function seedClosedPeriod() {
  store.periodStatus = "closed";
}

let idCounter = 0;
function nextId(prefix: string) {
  idCounter += 1;
  return `${prefix}${String(idCounter).padStart(8, "0")}`;
}

export const billingHandlers = [
  http.get(`${API_URL}/billing-periods/:id`, ({ params }) => {
    if (params.id !== fixturePeriodOpen.id) {
      return HttpResponse.json(fail("NOT_FOUND", "billing period not found"), { status: 404 });
    }
    return HttpResponse.json(ok({ ...fixturePeriodOpen, status: store.periodStatus }));
  }),

  http.get(`${API_URL}/billing-periods`, () => {
    const items = [{ ...fixturePeriodOpen, status: store.periodStatus }];
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),

  http.post(`${API_URL}/billing-periods/:id/draft`, ({ params }) => {
    if (params.id !== fixturePeriodOpen.id) {
      return HttpResponse.json(fail("NOT_FOUND", "billing period not found"), { status: 404 });
    }
    // The real `billing.Service.Draft` rejects a closed period with 409; the
    // client must fall back to `GET /preview` rather than surfacing an error.
    if (store.periodStatus === "closed") {
      return HttpResponse.json(fail("CONFLICT", "period is closed"), { status: 409 });
    }
    return HttpResponse.json(ok(buildReviewResponse(false)));
  }),

  // Pure read, valid in any period state; the real backend always nulls
  // invoice_id on preview (adjustments target draft-issued ids only).
  http.get(`${API_URL}/billing-periods/:id/preview`, ({ params }) => {
    if (params.id !== fixturePeriodOpen.id) {
      return HttpResponse.json(fail("NOT_FOUND", "billing period not found"), { status: 404 });
    }
    return HttpResponse.json(ok(buildReviewResponse(true)));
  }),

  http.get(`${API_URL}/sessions/pending`, () => {
    const items = store.periodStatus === "open" ? store.blockingSessions : [];
    return HttpResponse.json(ok({ total: items.length, items }));
  }),

  http.post(`${API_URL}/billing-periods/:id/close`, ({ params }) => {
    if (params.id !== fixturePeriodOpen.id) {
      return HttpResponse.json(fail("NOT_FOUND", "billing period not found"), { status: 404 });
    }
    if (store.blockingSessions.length > 0) {
      return HttpResponse.json(fail("CONFLICT", "period has unconfirmed sessions"), {
        status: 409,
      });
    }
    store.periodStatus = "closed";
    const rows = buildRows(store.adjustmentsByInvoice);
    const totalDue = rows.reduce((sum, row) => sum + row.total_due, 0);
    const response: CloseResponse = {
      period: { ...fixturePeriodOpen, status: "closed", closed_at: new Date().toISOString() },
      issued_count: rows.length,
      voided_count: 0,
      total_due: totalDue,
      warnings: { future_unconfirmed_sessions: [] },
    };
    return HttpResponse.json(ok(response));
  }),

  http.post(`${API_URL}/invoices/:invoiceId/adjustments`, async ({ params, request }) => {
    const invoiceId = params.invoiceId as string;
    const body = (await request.json()) as { amount: number; reason: string };
    if (body.amount === 0) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", { amount: "must not be zero" }),
        { status: 422 },
      );
    }
    if (body.reason.trim().length < 3) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", {
          reason: "must be between 3 and 500 characters",
        }),
        { status: 422 },
      );
    }
    const previous = store.adjustmentsByInvoice.get(invoiceId) ?? 0;
    store.adjustmentsByInvoice.set(invoiceId, previous + body.amount);
    const response: Adjustment = {
      id: nextId("adjustment-"),
      invoice_id: invoiceId,
      amount: body.amount,
      reason: body.reason,
      source_session_id: null,
      created_at: new Date().toISOString(),
    };
    return HttpResponse.json(ok(response), { status: 201 });
  }),
];
