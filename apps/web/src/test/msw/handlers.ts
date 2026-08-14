import { http, HttpResponse } from "msw";

import type { Teacher } from "@/features/auth";
import type { Meta } from "@/lib/api/envelope";

/** Must match vitest.config.ts test.env.VITE_API_URL. */
export const API_URL = "http://localhost:8080/api/v1";

/**
 * Origin for the public statement routes, which the real API mounts at the
 * server root (`/public/statements`) — outside `/api/v1`. Mirrors the
 * `publicApiClient` base-URL derivation in `@/lib/api/public-client.ts`.
 */
export const PUBLIC_API_URL = API_URL.replace(/\/api\/v1\/?$/, "");

// --- Envelope builders (mirror the Go API's response shape exactly) ---

export function ok<T>(data: T, meta?: Meta) {
  return meta === undefined ? { success: true, data } : { success: true, data, meta };
}

export function fail(code: string, message: string, fields?: Record<string, string>) {
  return {
    success: false,
    error: fields === undefined ? { code, message } : { code, message, fields },
  };
}

export function listMeta(total: number, page = 1, perPage = 20): Meta {
  return {
    page,
    per_page: perPage,
    total,
    total_pages: Math.max(1, Math.ceil(total / perPage)),
  };
}

// --- Fixtures ---

let teacherCounter = 0;

export function makeTeacher(overrides: Partial<Teacher> = {}): Teacher {
  teacherCounter += 1;
  return {
    id: `00000000-0000-4000-8000-${String(teacherCounter).padStart(12, "0")}`,
    phone: `+8490100${String(teacherCounter).padStart(4, "0")}`,
    full_name: `Teacher ${teacherCounter}`,
    timezone: "Asia/Ho_Chi_Minh",
    status: "active",
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

export const primaryTeacher = makeTeacher({ full_name: "Cô Lan" });
export const secondaryTeacher = makeTeacher({ full_name: "Thầy Minh" });

export function makeSession(teacher: Teacher) {
  return {
    access_token: "test-access-token",
    token_type: "Bearer",
    expires_in: 900,
    teacher,
  };
}

interface PendingSession {
  session_id: string;
  class_id: string;
  class_name: string;
  session_date: string;
  start_time: string | null;
  status: string;
  expected_student_count: number;
  days_overdue: number;
}

export function makePendingSession(overrides: Partial<PendingSession> = {}): PendingSession {
  return {
    session_id: "10000000-0000-4000-8000-000000000001",
    class_id: "20000000-0000-4000-8000-000000000001",
    class_name: "Toán 6A",
    session_date: "2026-07-15",
    start_time: "18:00:00",
    status: "held",
    expected_student_count: 10,
    days_overdue: 1,
    ...overrides,
  };
}

export const defaultPendingSessions: PendingSession[] = [];

let classCounter = 0;

/** `classes.ClassResponse` fixture — one Monday-19:00 schedule by default. */
export function makeClass(overrides: Record<string, unknown> = {}) {
  classCounter += 1;
  const id = `20000000-0000-4000-8000-${String(classCounter).padStart(12, "0")}`;
  return {
    id,
    name: `Toán 9A${classCounter}`,
    start_date: "2026-08-01",
    end_date: null,
    default_unit_price: 60000,
    status: "active",
    schedules: [
      {
        id: `21000000-0000-4000-8000-${String(classCounter).padStart(12, "0")}`,
        weekday: 1,
        start_time: "19:00",
        duration_min: 90,
        effective_from: "2026-08-01",
        effective_to: null,
      },
    ],
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

/** `sessions.SessionResponse` fixture for `GET /classes/:id/sessions`. */
export function makeClassSession(overrides: Record<string, unknown> = {}) {
  return {
    id: "10000000-0000-4000-8000-000000000099",
    class_id: "20000000-0000-4000-8000-000000000001",
    class_name: "Toán 9A1",
    session_date: "2026-08-03",
    start_time: "19:00:00",
    status: "held",
    cancel_reason: null,
    attendance_confirmed_at: "2026-08-03T21:00:00Z",
    student_count: 10,
    created_at: "2026-08-01T10:00:00Z",
    ...overrides,
  };
}

/** `billing.PreviewResponse` fixture — zeroed totals unless overridden. */
export function makePreview(overrides: Record<string, unknown> = {}) {
  return {
    invoices: [],
    totals: {
      student_count: 0,
      total_opening: 0,
      total_charge: 0,
      total_adjustment: 0,
      total_due: 0,
    },
    ...overrides,
  };
}

/** `collections` summary fixture — zeroed unless overridden. */
export function makeCollectionsSummary(overrides: Record<string, unknown> = {}) {
  return {
    student_count: 0,
    contact_count: 0,
    total_due: 0,
    total_paid: 0,
    total_outstanding: 0,
    paid_contact_count: 0,
    unpaid_contact_count: 0,
    partial_contact_count: 0,
    unallocated_credit: 0,
    ...overrides,
  };
}

export function makePeriod(overrides: Record<string, unknown> = {}) {
  return {
    id: "30000000-0000-4000-8000-000000000001",
    year: 2026,
    month: 8,
    period_start: "2026-08-01",
    period_end: "2026-08-31",
    status: "open",
    closed_at: null,
    ...overrides,
  };
}

// --- Public statement fixtures (GET /public/statements/:token) ---

const publicStatementFixtureOneChild = {
  contact_name: "Chị Hoa",
  period: "08/2026",
  children: [
    {
      student_name: "Nguyễn Văn An",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Toán 6A",
          unit_price: 150000,
          billable_count: 12,
          absent_count: 1,
          amount: 1800000,
          sessions: [
            { date: "2026-08-03", status: "present", counted: true },
            { date: "2026-08-05", status: "present", counted: true },
            { date: "2026-08-07", status: "absent", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 1800000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 1800000,
    adjustment_total: 0,
    total_due: 1800000,
    paid: 0,
    outstanding: 1800000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Nguyễn Văn An", total_due: 1800000, paid: 0, outstanding: 1800000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-an.png",
    amount: 1800000,
    note: "HP An T8",
  },
};

const publicStatementFixtureTwoChildren = {
  contact_name: "Anh Minh",
  period: "08/2026",
  children: [
    {
      student_name: "Trần Thị Bích",
      display_note: "Lớp 6",
      opening_balance: 500000,
      classes: [
        {
          class_name: "Toán 6A",
          unit_price: 150000,
          billable_count: 10,
          absent_count: 0,
          amount: 1500000,
          sessions: [
            { date: "2026-08-03", status: "present", counted: true },
            { date: "2026-08-05", status: "present", counted: true },
          ],
        },
        {
          class_name: "Lý 6A",
          unit_price: 130000,
          billable_count: 8,
          absent_count: 2,
          amount: 1040000,
          sessions: [
            { date: "2026-08-04", status: "present", counted: true },
            { date: "2026-08-06", status: "absent", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 3040000,
    },
    {
      student_name: "Trần Văn Cường",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Văn 9A",
          unit_price: 160000,
          billable_count: 9,
          absent_count: 0,
          amount: 1440000,
          sessions: [{ date: "2026-08-02", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 1440000,
    },
  ],
  totals: {
    opening_balance: 500000,
    current_charge: 3980000,
    adjustment_total: 0,
    total_due: 4480000,
    paid: 0,
    outstanding: 4480000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Trần Thị Bích", total_due: 3040000, paid: 0, outstanding: 3040000 },
      { student_name: "Trần Văn Cường", total_due: 1440000, paid: 0, outstanding: 1440000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-minh.png",
    amount: 4480000,
    note: "HP Bich Cuong T8",
  },
};

const publicStatementFixtureCancelledSession = {
  contact_name: "Chị Lan",
  period: "08/2026",
  children: [
    {
      student_name: "Phạm Thị Dung",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Tiếng Anh 7A",
          unit_price: 140000,
          billable_count: 7,
          absent_count: 1,
          amount: 980000,
          sessions: [
            { date: "2026-08-01", status: "present", counted: true },
            { date: "2026-08-08", status: "absent", counted: false },
            { date: "2026-08-15", status: "cancelled", counted: false },
          ],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 980000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 980000,
    adjustment_total: 0,
    total_due: 980000,
    paid: 0,
    outstanding: 980000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Phạm Thị Dung", total_due: 980000, paid: 0, outstanding: 980000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-dung.png",
    amount: 980000,
    note: "HP Dung T8",
  },
};

const publicStatementFixtureNoQr = {
  contact_name: "Anh Tuấn",
  period: "08/2026",
  children: [
    {
      student_name: "Lê Văn Em",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Toán 5A",
          unit_price: 120000,
          billable_count: 8,
          absent_count: 0,
          amount: 960000,
          sessions: [{ date: "2026-08-02", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 960000,
    },
  ],
  totals: {
    opening_balance: 0,
    current_charge: 960000,
    adjustment_total: 0,
    total_due: 960000,
    paid: 0,
    outstanding: 960000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [{ student_name: "Lê Văn Em", total_due: 960000, paid: 0, outstanding: 960000 }],
  },
  qr: null,
};

/** Reachable by token value from `publicStatementHandler` below; any other token 404s. */
const publicStatementFixturesByToken: Record<string, unknown> = {
  "valid-token": publicStatementFixtureOneChild,
  "two-child-token": publicStatementFixtureTwoChildren,
  "cancelled-session-token": publicStatementFixtureCancelledSession,
  "no-qr-token": publicStatementFixtureNoQr,
};

// --- Default happy-path handlers; tests override per case with server.use() ---

export const handlers = [
  http.post(`${API_URL}/auth/login`, () => HttpResponse.json(ok(makeSession(primaryTeacher)))),
  // No refresh cookie in tests by default: a fresh visitor has no session.
  http.post(`${API_URL}/auth/refresh`, () =>
    HttpResponse.json(fail("UNAUTHORIZED", "invalid refresh token"), { status: 401 }),
  ),
  http.post(`${API_URL}/auth/logout`, () => HttpResponse.json(ok({ message: "logged out" }))),
  http.get(`${API_URL}/me`, () => HttpResponse.json(ok(primaryTeacher))),
  // A test teacher has no linked Zalo account unless the test says otherwise.
  http.get(`${API_URL}/me/zalo`, () => HttpResponse.json(ok({ linked: false }))),
  http.get(`${API_URL}/sessions/pending`, () =>
    HttpResponse.json(ok({ total: defaultPendingSessions.length, items: defaultPendingSessions })),
  ),
  http.post(`${API_URL}/billing-periods`, () =>
    HttpResponse.json(ok(makePeriod()), { status: 201 }),
  ),
  // Dashboard aggregation defaults: an empty roster with nothing billed.
  http.get(`${API_URL}/classes`, () => HttpResponse.json(ok([], listMeta(0)))),
  http.get(`${API_URL}/classes/:id/sessions`, () => HttpResponse.json(ok([]))),
  // Teaching defaults: nothing saved yet. Stateful round-trip handlers live in
  // `@/features/teaching/__tests__/teaching-handlers.ts`; tests that write
  // register those with server.use().
  http.get(`${API_URL}/classes/:id/curriculum`, () =>
    HttpResponse.json(ok({ lessons: [], current_index: 0 })),
  ),
  http.get(`${API_URL}/classes/:id/lesson-plans`, () => HttpResponse.json(ok([]))),
  http.get(`${API_URL}/classes/:id/marks`, () =>
    HttpResponse.json(ok({ session_notes: [], marks: [] })),
  ),
  http.get(`${API_URL}/teaching/review-queue`, () => HttpResponse.json(ok([]))),
  http.get(`${API_URL}/students`, () => HttpResponse.json(ok([], listMeta(0)))),
  http.get(`${API_URL}/billing-periods/:id/preview`, () => HttpResponse.json(ok(makePreview()))),
  http.get(`${API_URL}/billing-periods/:id/collections/summary`, () =>
    HttpResponse.json(ok(makeCollectionsSummary())),
  ),
  // Always the same generic body — the real endpoint never reveals whether
  // the phone matched an eligible account (anti-enumeration).
  http.post(`${API_URL}/auth/forgot-password`, () =>
    HttpResponse.json(ok({ message: "if this phone is registered, a reset link has been sent" })),
  ),
  http.post(`${API_URL}/auth/reset-password`, () => new HttpResponse(null, { status: 204 })),
  // Owner-shaped by default — the signed-in teacher owns their center, so the
  // layout's center card resolves everywhere; member-shaped tests override.
  http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(
      ok({
        center: {
          id: "30000000-0000-4000-8000-000000000001",
          name: "Trung Tâm Bình Minh",
          is_owner: true,
        },
        members: [],
      }),
    ),
  ),
  http.post(`${API_URL}/centers/me/invitations`, () =>
    HttpResponse.json(
      ok({
        id: "40000000-0000-4000-8000-000000000001",
        phone: "+84901234567",
        expires_at: "2026-08-19T10:00:00Z",
        link: "https://app.teka.dev/invite/test-invite-token",
        dm_status: "sent",
      }),
      { status: 201 },
    ),
  ),
  http.get(`${API_URL}/centers/me/invitations`, () => HttpResponse.json(ok([]))),
  http.delete(
    `${API_URL}/centers/me/invitations/:id`,
    () => new HttpResponse(null, { status: 204 }),
  ),
  // Anti-enumeration: every rejection reason (unknown/expired/revoked/used
  // token) collapses to the same generic 404 on the real API; tests override
  // this default with a fixture keyed to a specific token.
  http.post(`${API_URL}/invitations/preview`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", phone_masked: "+84******567" })),
  ),
  http.post(`${API_URL}/invitations/accept`, () => new HttpResponse(null, { status: 204 })),
  // Every failure mode (unknown, malformed, revoked, expired, already-paid,
  // soft-deleted) collapses to a plain 404 — the real API has no other error
  // code for this endpoint.
  http.get(`${PUBLIC_API_URL}/public/statements/:token`, ({ params }) => {
    const fixture = publicStatementFixturesByToken[params.token as string];
    if (!fixture) {
      return HttpResponse.json(fail("NOT_FOUND", "statement not found"), { status: 404 });
    }
    return HttpResponse.json(ok(fixture));
  }),
];
