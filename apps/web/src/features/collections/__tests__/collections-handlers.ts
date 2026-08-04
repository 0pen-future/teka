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
  channel: "zalo_manual" | "zalo_zns" | "sms";
  purpose: "statements" | "reminder";
  status: "queued" | "sent" | "delivered" | "failed";
  sent_at: string | null;
  created_at: string;
  message_text: string;
  url: string;
}

interface Store {
  classes: Class[];
  invoices: Invoice[];
  payments: Map<string, StoredPayment>;
  notifications: StoredNotification[];
}

function seedCollectionsStore(): Store {
  return {
    classes: [{ ...classMath }, { ...classEnglish }, { ...classEmpty }],
    invoices: buildInvoices(),
    payments: new Map(),
    notifications: [],
  };
}

let store = seedCollectionsStore();

export function resetCollectionsStore() {
  store = seedCollectionsStore();
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

  http.post(`${API_URL}/billing-periods/:id/notifications/bulk`, async ({ request }) => {
    const body = (await request.json()) as {
      purpose: "statements" | "reminder";
      channel?: "zalo_manual" | "zalo_zns" | "sms";
    };
    const rows = buildContactRows().filter((row) =>
      body.purpose === "reminder" ? row.outstanding > 0 : true,
    );
    const generated: BulkSendRow[] = rows.map((row) => {
      const record: StoredNotification = {
        id: nextId("notification-"),
        contact_id: row.contact_id,
        contact_name: row.full_name,
        phone: row.phone,
        channel: body.channel ?? "zalo_manual",
        purpose: body.purpose,
        status: "queued",
        sent_at: null,
        created_at: new Date().toISOString(),
        message_text: messageTextFor(row.contact_id, body.purpose),
        url: `/statements/${row.contact_id}?period=${fixturePeriod.id}`,
      };
      store.notifications.push(record);
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
    return HttpResponse.json(
      ok({
        queued_count: generated.length,
        skipped_paid_count: buildContactRows().length - rows.length,
        collapsed_count: 0,
        bulk_text: generated.map((row) => row.message_text).join("\n\n"),
        rows: generated,
      }),
    );
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
      sent_at: notification.sent_at,
      created_at: notification.created_at,
    }));
    if (purpose) {
      items = items.filter((item) => item.purpose === purpose);
    }
    if (status) {
      items = items.filter((item) => item.status === status);
    }
    return HttpResponse.json(ok(items, listMeta(items.length)));
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
