import { http, HttpResponse } from "msw";

import type { Class } from "@/features/roster";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  AllocationResponse,
  BulkSendRow,
  ClassCollectionRow,
  CollectionsSummary,
  ContactBalanceRow,
  NotificationRow,
  PaymentResponse,
  Period,
} from "../schemas/collections-schemas";

export const fixturePeriod: Period = {
  id: "c0000000-0000-4000-8000-000000000001",
  year: 2026,
  month: 8,
  period_start: "2026-08-01",
  period_end: "2026-08-31",
  status: "open",
  closed_at: null,
};

export const classMath: Class = {
  id: "c1000000-0000-4000-8000-000000000001",
  name: "Toán 6A",
  teacher_id: "c2000000-0000-4000-8000-000000000001",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 150000,
  status: "active",
  schedules: [],
  created_at: "2026-01-01T08:00:00Z",
};

export const classEnglish: Class = {
  id: "c1000000-0000-4000-8000-000000000002",
  name: "Anh Văn 7B",
  teacher_id: "c2000000-0000-4000-8000-000000000002",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 150000,
  status: "active",
  schedules: [],
  created_at: "2026-01-01T08:00:00Z",
};

/** No sessions fell in the fixture period — never produces a collections/invoice row. */
export const classEmpty: Class = {
  id: "c1000000-0000-4000-8000-000000000003",
  name: "Vẽ 5A",
  teacher_id: "c2000000-0000-4000-8000-000000000003",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 150000,
  status: "active",
  schedules: [],
  created_at: "2026-01-01T08:00:00Z",
};

// --- Contacts ---
// Two children in two different classes, both fully paid.
export const contactTwoChildren = {
  id: "c2000000-0000-4000-8000-000000000001",
  full_name: "Phạm Văn Hùng",
  phone: "+84987654321",
};

/** Single child, carrying nợ cũ (opening balance) into this period, fully unpaid. */
export const contactSingleChildOwing = {
  id: "c2000000-0000-4000-8000-000000000002",
  full_name: "Nguyễn Thị Lan",
  phone: "+84912345678",
};

/** Single child, partially paid this period — the underpayment/shortfall case. */
export const contactUnderpaid = {
  id: "c2000000-0000-4000-8000-000000000003",
  full_name: "Trần Văn Bình",
  phone: "+84900000000",
};

/** Two children across two classes, both still unpaid — the record-payment target. */
export const contactTwoChildrenOwing = {
  id: "c2000000-0000-4000-8000-000000000004",
  full_name: "Lê Thị Mai",
  phone: "+84911111111",
};

interface Invoice {
  id: string;
  contact_id: string;
  contact_name: string;
  phone: string;
  student_id: string;
  student_name: string;
  class_id: string;
  class_name: string;
  billable_count: number;
  absent_count: number;
  line_amount: number;
  opening_balance: number;
  total_due: number;
  paid_amount: number;
  outstanding: number;
}

export const invoiceStudentA1 = "c3000000-0000-4000-8000-000000000001";
export const invoiceStudentA2 = "c3000000-0000-4000-8000-000000000002";
export const invoiceStudentB = "c3000000-0000-4000-8000-000000000003";
export const invoiceStudentC = "c3000000-0000-4000-8000-000000000004";
export const invoiceStudentD1 = "c3000000-0000-4000-8000-000000000005";
export const invoiceStudentD2 = "c3000000-0000-4000-8000-000000000006";

function buildInvoices(): Invoice[] {
  return [
    {
      id: invoiceStudentA1,
      contact_id: contactTwoChildren.id,
      contact_name: contactTwoChildren.full_name,
      phone: contactTwoChildren.phone,
      student_id: "c4000000-0000-4000-8000-000000000001",
      student_name: "Phạm Minh Anh",
      class_id: classMath.id,
      class_name: classMath.name,
      billable_count: 8,
      absent_count: 0,
      line_amount: 500000,
      opening_balance: 0,
      total_due: 500000,
      paid_amount: 500000,
      outstanding: 0,
    },
    {
      id: invoiceStudentA2,
      contact_id: contactTwoChildren.id,
      contact_name: contactTwoChildren.full_name,
      phone: contactTwoChildren.phone,
      student_id: "c4000000-0000-4000-8000-000000000002",
      student_name: "Phạm Minh Châu",
      class_id: classEnglish.id,
      class_name: classEnglish.name,
      billable_count: 6,
      absent_count: 1,
      line_amount: 450000,
      opening_balance: 0,
      total_due: 450000,
      paid_amount: 450000,
      outstanding: 0,
    },
    {
      id: invoiceStudentB,
      contact_id: contactSingleChildOwing.id,
      contact_name: contactSingleChildOwing.full_name,
      phone: contactSingleChildOwing.phone,
      student_id: "c4000000-0000-4000-8000-000000000003",
      student_name: "Nguyễn Minh Khôi",
      class_id: classMath.id,
      class_name: classMath.name,
      billable_count: 8,
      absent_count: 0,
      line_amount: 500000,
      opening_balance: 200000,
      total_due: 700000,
      paid_amount: 0,
      outstanding: 700000,
    },
    {
      id: invoiceStudentC,
      contact_id: contactUnderpaid.id,
      contact_name: contactUnderpaid.full_name,
      phone: contactUnderpaid.phone,
      student_id: "c4000000-0000-4000-8000-000000000004",
      student_name: "Trần Gia Bảo",
      class_id: classMath.id,
      class_name: classMath.name,
      billable_count: 8,
      absent_count: 0,
      line_amount: 500000,
      opening_balance: 0,
      total_due: 500000,
      paid_amount: 300000,
      outstanding: 200000,
    },
    {
      id: invoiceStudentD1,
      contact_id: contactTwoChildrenOwing.id,
      contact_name: contactTwoChildrenOwing.full_name,
      phone: contactTwoChildrenOwing.phone,
      student_id: "c4000000-0000-4000-8000-000000000005",
      student_name: "Lê Gia Hân",
      class_id: classMath.id,
      class_name: classMath.name,
      billable_count: 8,
      absent_count: 0,
      line_amount: 500000,
      opening_balance: 0,
      total_due: 500000,
      paid_amount: 0,
      outstanding: 500000,
    },
    {
      id: invoiceStudentD2,
      contact_id: contactTwoChildrenOwing.id,
      contact_name: contactTwoChildrenOwing.full_name,
      phone: contactTwoChildrenOwing.phone,
      student_id: "c4000000-0000-4000-8000-000000000006",
      student_name: "Lê Gia Bảo",
      class_id: classEnglish.id,
      class_name: classEnglish.name,
      billable_count: 6,
      absent_count: 1,
      line_amount: 450000,
      opening_balance: 0,
      total_due: 450000,
      paid_amount: 0,
      outstanding: 450000,
    },
  ];
}

type StoredPayment = PaymentResponse;

interface StoredNotification {
  id: string;
  contact_id: string;
  contact_name: string;
  phone: string;
  channel: "zalo_manual" | "zalo_zns" | "sms" | "zalo_personal";
  purpose: "statements" | "reminder";
  status: "queued" | "sent" | "delivered" | "failed";
  error_message: string | null;
  run_id: string | null;
  sent_at: string | null;
  created_at: string;
  message_text: string;
  url: string;
}

interface StoredRun {
  id: string;
  status: "running" | "completed" | "interrupted" | "expired";
  purpose: "statements" | "reminder";
  notification_ids: string[];
}

/**
 * Contacts the teacher has bound to a Zalo friend. Lan and Mai are mapped;
 * Hùng and Bình are not — so a statements send splits 2 auto / 2 manual and a
 * reminder send (Hùng fully paid) splits 2 auto / 1 manual.
 */
export const zaloMappedContacts: Record<string, { zalo_user_id: string; zalo_name: string }> = {
  [contactSingleChildOwing.id]: { zalo_user_id: "zalo-uid-lan", zalo_name: "Lan Nguyễn" },
  [contactTwoChildrenOwing.id]: { zalo_user_id: "zalo-uid-mai", zalo_name: "Mai Lê" },
};

interface Store {
  classes: Class[];
  invoices: Invoice[];
  payments: Map<string, StoredPayment>;
  notifications: StoredNotification[];
  run: StoredRun | null;
  /** Contact ids whose run row must fail (with this reason) instead of delivering. */
  runFailures: Map<string, string>;
  /** When true, snapshot reads stop advancing the run — it stays running. */
  runHeld: boolean;
  /** Mapped contacts the preview reports as not (yet) Zalo friends. */
  notFriendContacts: Set<string>;
  /** The preview's `max_run_size`; 0 means no cap, like the real config default. */
  previewMaxRunSize: number;
}

function seedCollectionsStore(): Store {
  return {
    classes: [{ ...classMath }, { ...classEnglish }, { ...classEmpty }],
    invoices: buildInvoices(),
    payments: new Map(),
    notifications: [],
    run: null,
    runFailures: new Map(),
    runHeld: false,
    notFriendContacts: new Set(),
    previewMaxRunSize: 0,
  };
}

let store = seedCollectionsStore();

export function resetCollectionsStore() {
  store = seedCollectionsStore();
}

/** Makes the run row for this contact fail with `reason` instead of delivering. */
export function failRunRowFor(contactId: string, reason: string) {
  store.runFailures.set(contactId, reason);
}

/** Moves a mapped contact into the preview's `mapped_not_friend` bucket. */
export function markZaloNotFriend(contactId: string) {
  store.notFriendContacts.add(contactId);
}

/** Sets the preview's `max_run_size` cap (0 = no cap). */
export function setPreviewMaxRunSize(size: number) {
  store.previewMaxRunSize = size;
}

/** Freezes run progression so a test can act while the run is still running. */
export function holdRunProgress() {
  store.runHeld = true;
}

/** Simulates a server restart mid-run: the run stops advancing until resumed. */
export function interruptRun() {
  if (store.run) {
    store.run.status = "interrupted";
  }
}

/**
 * Seeds a run already half-way through delivering — what a teacher who closed
 * the tab mid-run finds when they reopen the page: one row delivered, one
 * still queued, nothing generated in this session.
 */
export function seedRunMidFlight() {
  const runId = nextId("run-");
  const delivered: StoredNotification = {
    id: nextId("notification-"),
    contact_id: contactSingleChildOwing.id,
    contact_name: contactSingleChildOwing.full_name,
    phone: contactSingleChildOwing.phone,
    channel: "zalo_personal",
    purpose: "statements",
    status: "delivered",
    error_message: null,
    run_id: runId,
    sent_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    message_text: messageTextFor(contactSingleChildOwing.id, "statements"),
    url: `/statements/${contactSingleChildOwing.id}?period=${fixturePeriod.id}`,
  };
  const queued: StoredNotification = {
    id: nextId("notification-"),
    contact_id: contactTwoChildrenOwing.id,
    contact_name: contactTwoChildrenOwing.full_name,
    phone: contactTwoChildrenOwing.phone,
    channel: "zalo_personal",
    purpose: "statements",
    status: "queued",
    error_message: null,
    run_id: runId,
    sent_at: null,
    created_at: new Date().toISOString(),
    message_text: messageTextFor(contactTwoChildrenOwing.id, "statements"),
    url: `/statements/${contactTwoChildrenOwing.id}?period=${fixturePeriod.id}`,
  };
  store.notifications.push(delivered, queued);
  store.run = {
    id: runId,
    status: "running",
    purpose: "statements",
    notification_ids: [delivered.id, queued.id],
  };
}

/**
 * A failed personal row left behind by an earlier, finished run. Its reason
 * belongs to that old run only — a newer run's banner must not adopt it.
 */
export function seedOldRunFailure(reason: string) {
  store.notifications.push({
    id: nextId("notification-"),
    contact_id: contactUnderpaid.id,
    contact_name: contactUnderpaid.full_name,
    phone: contactUnderpaid.phone,
    channel: "zalo_personal",
    purpose: "statements",
    status: "failed",
    error_message: reason,
    run_id: nextId("run-"),
    sent_at: null,
    created_at: new Date().toISOString(),
    message_text: messageTextFor(contactUnderpaid.id, "statements"),
    url: `/statements/${contactUnderpaid.id}?period=${fixturePeriod.id}`,
  });
}

let idCounter = 0;
function nextId(prefix: string) {
  idCounter += 1;
  return `${prefix}${String(idCounter).padStart(8, "0")}`;
}

function paymentStatusOf(paidAmount: number, totalDue: number): "unpaid" | "partial" | "paid" {
  if (paidAmount <= 0) {
    return "unpaid";
  }
  if (paidAmount >= totalDue) {
    return "paid";
  }
  return "partial";
}

function buildContactRows(): ContactBalanceRow[] {
  const byContact = new Map<string, Invoice[]>();
  for (const invoice of store.invoices) {
    const list = byContact.get(invoice.contact_id) ?? [];
    list.push(invoice);
    byContact.set(invoice.contact_id, list);
  }
  const rows: ContactBalanceRow[] = [];
  for (const [contactId, invoices] of byContact) {
    const totalDue = invoices.reduce((sum, inv) => sum + inv.total_due, 0);
    const totalPaid = invoices.reduce((sum, inv) => sum + inv.paid_amount, 0);
    rows.push({
      contact_id: contactId,
      full_name: invoices[0]!.contact_name,
      phone: invoices[0]!.phone,
      contact_archived: false,
      student_count: invoices.length,
      total_due: totalDue,
      total_paid: totalPaid,
      outstanding: totalDue - totalPaid,
      payment_status: paymentStatusOf(totalPaid, totalDue),
      invoices: invoices.map((inv) => ({
        invoice_id: inv.id,
        student_name: inv.student_name,
        total_due: inv.total_due,
        paid_amount: inv.paid_amount,
        outstanding: inv.outstanding,
      })),
    });
  }
  return rows;
}

function toClassRow(invoice: Invoice): ClassCollectionRow {
  return {
    invoice_id: invoice.id,
    student_id: invoice.student_id,
    student_name: invoice.student_name,
    contact_id: invoice.contact_id,
    contact_name: invoice.contact_name,
    class_name: invoice.class_name,
    billable_count: invoice.billable_count,
    absent_count: invoice.absent_count,
    line_amount: invoice.line_amount,
    invoice_opening_balance: invoice.opening_balance,
    invoice_total_due: invoice.total_due,
    invoice_paid_amount: invoice.paid_amount,
    invoice_outstanding: invoice.outstanding,
    payment_status: paymentStatusOf(invoice.paid_amount, invoice.total_due),
  };
}

function buildSummary(): CollectionsSummary {
  const rows = buildContactRows();
  const invoices = store.invoices;
  return {
    student_count: invoices.length,
    contact_count: rows.length,
    total_due: invoices.reduce((sum, inv) => sum + inv.total_due, 0),
    total_paid: invoices.reduce((sum, inv) => sum + inv.paid_amount, 0),
    total_outstanding: invoices.reduce((sum, inv) => sum + inv.outstanding, 0),
    paid_contact_count: rows.filter((row) => row.payment_status === "paid").length,
    unpaid_contact_count: rows.filter((row) => row.payment_status === "unpaid").length,
    partial_contact_count: rows.filter((row) => row.payment_status === "partial").length,
    unallocated_credit: 0,
  };
}

function messageTextFor(contactId: string, purpose: "statements" | "reminder"): string {
  const invoices = store.invoices.filter((inv) => inv.contact_id === contactId);
  const contactName = invoices[0]?.contact_name ?? "";
  const lines = invoices.map(
    (inv) =>
      `${inv.student_name}: ${inv.billable_count} buổi, ${inv.line_amount.toLocaleString("vi-VN")}đ`,
  );
  const openingTotal = invoices.reduce((sum, inv) => sum + inv.opening_balance, 0);
  const totalDue = invoices.reduce((sum, inv) => sum + inv.total_due, 0);
  const header =
    purpose === "reminder"
      ? "[Nhắc học phí]"
      : `[Học phí T${fixturePeriod.month}/${fixturePeriod.year}]`;
  return [
    header,
    `Kính gửi ${contactName},`,
    ...lines,
    openingTotal > 0 ? `Nợ cũ: ${openingTotal.toLocaleString("vi-VN")}đ` : null,
    `Tổng: ${totalDue.toLocaleString("vi-VN")}đ`,
    `Xem chi tiết: /statements/${contactId}?period=${fixturePeriod.id}`,
  ]
    .filter((line): line is string => Boolean(line))
    .join("\n");
}

export const collectionsHandlers = [
  http.get(`${API_URL}/classes`, ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get("status");
    const items = store.classes.filter(
      (klass) => !status || status === "all" || klass.status === status,
    );
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),

  http.get(`${API_URL}/billing-periods/:id`, ({ params }) => {
    if (params.id !== fixturePeriod.id) {
      return HttpResponse.json(fail("NOT_FOUND", "period not found"), { status: 404 });
    }
    return HttpResponse.json(ok(fixturePeriod));
  }),

  http.get(`${API_URL}/billing-periods/:id/collections`, ({ request }) => {
    const url = new URL(request.url);
    const view = url.searchParams.get("view");
    const status = url.searchParams.get("status");

    if (view === "class") {
      const classId = url.searchParams.get("class_id");
      if (!classId) {
        return HttpResponse.json(fail("VALIDATION_ERROR", "class_id is required"), { status: 422 });
      }
      let rows = store.invoices.filter((invoice) => invoice.class_id === classId).map(toClassRow);
      if (status) {
        rows = rows.filter((row) => row.payment_status === status);
      }
      return HttpResponse.json(ok(rows, listMeta(rows.length)));
    }

    let rows = buildContactRows();
    if (status) {
      rows = rows.filter((row) => row.payment_status === status);
    }
    return HttpResponse.json(ok(rows, listMeta(rows.length)));
  }),

  http.get(`${API_URL}/billing-periods/:id/collections/summary`, () => {
    return HttpResponse.json(ok(buildSummary()));
  }),

  http.post(`${API_URL}/payments`, async ({ request }) => {
    const body = (await request.json()) as {
      contact_id: string;
      amount: number;
      method: "cash" | "transfer" | "other";
      received_on: string;
      reference_code?: string;
      note?: string;
    };
    const contactInvoices = store.invoices.filter(
      (invoice) => invoice.contact_id === body.contact_id && invoice.outstanding > 0,
    );
    let remaining = body.amount;
    const allocations: AllocationResponse[] = [];
    for (const invoice of contactInvoices) {
      if (remaining <= 0) {
        break;
      }
      const amount = Math.min(invoice.outstanding, remaining);
      if (amount <= 0) {
        continue;
      }
      invoice.paid_amount += amount;
      invoice.outstanding -= amount;
      remaining -= amount;
      allocations.push({
        invoice_id: invoice.id,
        student_id: invoice.student_id,
        student_name: invoice.student_name,
        period_id: fixturePeriod.id,
        amount,
        allocated_by: "auto",
        total_due: invoice.total_due,
        paid_amount: invoice.paid_amount,
        outstanding: invoice.outstanding,
      });
    }
    const payment: StoredPayment = {
      id: nextId("payment-"),
      contact_id: body.contact_id,
      amount: body.amount,
      method: body.method,
      received_on: body.received_on,
      reference_code: body.reference_code ?? null,
      note: body.note ?? null,
      reverses_payment_id: null,
      reversed_at: null,
      allocations,
      unallocated_amount: remaining,
      created_at: new Date().toISOString(),
    };
    store.payments.set(payment.id, payment);
    return HttpResponse.json(ok(payment), { status: 201 });
  }),

  http.put(`${API_URL}/payments/:id/allocations`, async ({ params, request }) => {
    const payment = store.payments.get(params.id as string);
    if (!payment) {
      return HttpResponse.json(fail("NOT_FOUND", "payment not found"), { status: 404 });
    }
    const body = (await request.json()) as {
      allocations: { invoice_id: string; amount: number }[];
    };

    for (const previous of payment.allocations) {
      const invoice = store.invoices.find((item) => item.id === previous.invoice_id);
      if (invoice) {
        invoice.paid_amount -= previous.amount;
        invoice.outstanding += previous.amount;
      }
    }

    const nextAllocations: AllocationResponse[] = [];
    for (const line of body.allocations) {
      const invoice = store.invoices.find((item) => item.id === line.invoice_id);
      if (!invoice) {
        continue;
      }
      invoice.paid_amount += line.amount;
      invoice.outstanding -= line.amount;
      nextAllocations.push({
        invoice_id: invoice.id,
        student_id: invoice.student_id,
        student_name: invoice.student_name,
        period_id: fixturePeriod.id,
        amount: line.amount,
        allocated_by: "manual",
        total_due: invoice.total_due,
        paid_amount: invoice.paid_amount,
        outstanding: invoice.outstanding,
      });
    }
    payment.allocations = nextAllocations;
    payment.unallocated_amount =
      payment.amount - nextAllocations.reduce((sum, line) => sum + line.amount, 0);
    return HttpResponse.json(ok(payment));
  }),

  // Mirrors the real preview: the purpose's full target set split by the
  // caller's live Zalo state — mapped+friend, mapped-not-friend, unmapped —
  // each bucket sorted by contact name, plus the run-size cap.
  http.get(`${API_URL}/billing-periods/:id/notifications/preview`, ({ request }) => {
    const url = new URL(request.url);
    const purpose = url.searchParams.get("purpose");
    const rows = buildContactRows().filter((row) =>
      purpose === "reminder" ? row.outstanding > 0 : true,
    );
    const contact = (row: ContactBalanceRow) => ({
      contact_id: row.contact_id,
      contact_name: row.full_name,
    });
    const byName = (a: { contact_name: string }, b: { contact_name: string }) =>
      a.contact_name.localeCompare(b.contact_name);
    return HttpResponse.json(
      ok({
        auto_send: rows
          .filter(
            (row) =>
              row.contact_id in zaloMappedContacts && !store.notFriendContacts.has(row.contact_id),
          )
          .map(contact)
          .sort(byName),
        mapped_not_friend: rows
          .filter(
            (row) =>
              row.contact_id in zaloMappedContacts && store.notFriendContacts.has(row.contact_id),
          )
          .map(contact)
          .sort(byName),
        unmapped: rows
          .filter((row) => !(row.contact_id in zaloMappedContacts))
          .map(contact)
          .sort(byName),
        max_run_size: store.previewMaxRunSize,
      }),
    );
  }),

  http.post(`${API_URL}/billing-periods/:id/notifications/bulk`, async ({ request }) => {
    const body = (await request.json()) as {
      purpose: "statements" | "reminder";
      channel?: "zalo_manual" | "zalo_zns" | "sms" | "zalo_personal";
    };
    const personal = body.channel === "zalo_personal";
    if (personal && store.run?.status === "running") {
      return HttpResponse.json(
        fail("CONFLICT", "a zalo_personal run is already sending; wait for it to finish"),
        { status: 409 },
      );
    }
    const rows = buildContactRows().filter((row) =>
      body.purpose === "reminder" ? row.outstanding > 0 : true,
    );
    const runNotificationIds: string[] = [];
    const generated: BulkSendRow[] = rows.map((row) => {
      const mapped = personal && row.contact_id in zaloMappedContacts;
      const channel = mapped
        ? "zalo_personal"
        : personal
          ? "zalo_manual"
          : (body.channel ?? "zalo_manual");
      const record: StoredNotification = {
        id: nextId("notification-"),
        contact_id: row.contact_id,
        contact_name: row.full_name,
        phone: row.phone,
        channel,
        purpose: body.purpose,
        status: "queued",
        error_message: null,
        run_id: null,
        sent_at: null,
        created_at: new Date().toISOString(),
        message_text: messageTextFor(row.contact_id, body.purpose),
        url: `/statements/${row.contact_id}?period=${fixturePeriod.id}`,
      };
      store.notifications.push(record);
      if (mapped) {
        runNotificationIds.push(record.id);
      }
      return {
        notification_id: record.id,
        contact_id: record.contact_id,
        contact_name: record.contact_name,
        phone: record.phone,
        channel: record.channel,
        purpose: record.purpose,
        status: record.status,
        message_text: record.message_text,
        url: record.url,
        collapsed: false,
      };
    });
    let runId: string | null = null;
    if (runNotificationIds.length > 0) {
      runId = nextId("run-");
      store.run = {
        id: runId,
        status: "running",
        purpose: body.purpose,
        notification_ids: runNotificationIds,
      };
      for (const notification of store.notifications) {
        if (runNotificationIds.includes(notification.id)) {
          notification.run_id = runId;
        }
      }
    }
    // BulkText is the copy-paste bundle — auto-sent personal rows stay out.
    const manualRows = generated.filter((row) => row.channel !== "zalo_personal");
    return HttpResponse.json(
      ok({
        queued_count: generated.length,
        skipped_paid_count: buildContactRows().length - rows.length,
        collapsed_count: 0,
        run_id: runId,
        personal_queued_count: runNotificationIds.length,
        fallback_manual_count: personal ? manualRows.length : 0,
        bulk_text: manualRows.map((row) => row.message_text).join("\n\n"),
        rows: generated,
      }),
    );
  }),

  http.get(`${API_URL}/billing-periods/:id/notifications/run`, () => {
    const run = store.run;
    if (!run) {
      return HttpResponse.json(ok({ active: false, run_id: null, total: 0, sent: 0, failed: 0 }));
    }
    // Each poll delivers (or fails) one more queued row — the background
    // sender's pacing compressed to "one row per snapshot read" so tests can
    // watch x/y advance without real waiting.
    if (run.status === "running" && !store.runHeld) {
      const next = store.notifications.find(
        (item) => run.notification_ids.includes(item.id) && item.status === "queued",
      );
      if (next) {
        const reason = store.runFailures.get(next.contact_id);
        if (reason) {
          next.status = "failed";
          next.error_message = reason;
        } else {
          next.status = "delivered";
          next.sent_at = new Date().toISOString();
        }
      }
      const queuedLeft = store.notifications.some(
        (item) => run.notification_ids.includes(item.id) && item.status === "queued",
      );
      if (!queuedLeft) {
        run.status = "completed";
      }
    }
    const rows = store.notifications.filter((item) => run.notification_ids.includes(item.id));
    return HttpResponse.json(
      ok({
        active: run.status === "running",
        run_id: run.id,
        status: run.status,
        purpose: run.purpose,
        total: rows.length,
        sent: rows.filter((item) => item.status === "delivered").length,
        failed: rows.filter((item) => item.status === "failed").length,
      }),
    );
  }),

  http.post(`${API_URL}/billing-periods/:id/notifications/run/resume`, () => {
    const run = store.run;
    if (!run) {
      return HttpResponse.json(fail("NOT_FOUND", "notification run not found"), { status: 404 });
    }
    if (run.status !== "interrupted") {
      return HttpResponse.json(fail("CONFLICT", "only an interrupted run can be resumed"), {
        status: 409,
      });
    }
    run.status = "running";
    const rows = store.notifications.filter((item) => run.notification_ids.includes(item.id));
    return HttpResponse.json(
      ok({
        active: true,
        run_id: run.id,
        status: run.status,
        purpose: run.purpose,
        total: rows.length,
        sent: rows.filter((item) => item.status === "delivered").length,
        failed: rows.filter((item) => item.status === "failed").length,
      }),
    );
  }),

  // The roster feature owns the real `/contacts` handler; the notifications
  // page only needs id + zalo mapping to split auto vs manual counts, so this
  // suite serves its own contacts consistent with the collections fixtures.
  http.get(`${API_URL}/contacts`, () => {
    const contacts = buildContactRows().map((row) => ({
      id: row.contact_id,
      full_name: row.full_name,
      phone: row.phone,
      student_count: row.student_count,
      created_at: "2026-01-01T08:00:00Z",
      ...(zaloMappedContacts[row.contact_id] ?? {}),
    }));
    return HttpResponse.json(ok(contacts, listMeta(contacts.length)));
  }),

  http.get(`${API_URL}/billing-periods/:id/notifications`, ({ request }) => {
    const url = new URL(request.url);
    const purpose = url.searchParams.get("purpose");
    const status = url.searchParams.get("status");
    let items: NotificationRow[] = store.notifications.map((notification) => ({
      id: notification.id,
      contact_id: notification.contact_id,
      contact_name: notification.contact_name,
      phone: notification.phone,
      channel: notification.channel,
      purpose: notification.purpose,
      status: notification.status,
      error_message: notification.error_message ?? undefined,
      run_id: notification.run_id ?? undefined,
      sent_at: notification.sent_at,
      created_at: notification.created_at,
    }));
    if (purpose) {
      items = items.filter((item) => item.purpose === purpose);
    }
    if (status) {
      items = items.filter((item) => item.status === status);
    }
    // The real handler answers a bare array with no meta block — the mock
    // must match the wire, or a paginated-envelope regression hides here.
    return HttpResponse.json(ok(items));
  }),

  http.post(`${API_URL}/notifications/mark-sent`, async ({ request }) => {
    const body = (await request.json()) as { ids: string[] };
    const idSet = new Set(body.ids);
    const now = new Date().toISOString();
    for (const notification of store.notifications) {
      if (idSet.has(notification.id)) {
        notification.status = "sent";
        notification.sent_at = now;
      }
    }
    return new HttpResponse(null, { status: 204 });
  }),
];
