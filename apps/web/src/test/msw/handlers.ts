import { http, HttpResponse } from "msw";

import type { Teacher } from "@/features/auth";
import type { Meta } from "@/lib/api/envelope";

/** Must match vitest.config.ts test.env.VITE_API_URL. */
export const API_URL = "http://localhost:8080/api/v1";

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

// --- Default happy-path handlers; tests override per case with server.use() ---

export const handlers = [
  http.post(`${API_URL}/auth/login`, () => HttpResponse.json(ok(makeSession(primaryTeacher)))),
  http.post(`${API_URL}/auth/register`, async ({ request }) => {
    const body = (await request.json()) as { full_name: string; phone: string };
    const teacher = makeTeacher({ full_name: body.full_name, phone: body.phone });
    return HttpResponse.json(ok(makeSession(teacher)), { status: 201 });
  }),
  // No refresh cookie in tests by default: a fresh visitor has no session.
  http.post(`${API_URL}/auth/refresh`, () =>
    HttpResponse.json(fail("UNAUTHORIZED", "invalid refresh token"), { status: 401 }),
  ),
  http.post(`${API_URL}/auth/logout`, () => HttpResponse.json(ok({ message: "logged out" }))),
  http.get(`${API_URL}/me`, () => HttpResponse.json(ok(primaryTeacher))),
  http.get(`${API_URL}/sessions/pending`, () =>
    HttpResponse.json(ok({ total: defaultPendingSessions.length, items: defaultPendingSessions })),
  ),
  http.post(`${API_URL}/billing-periods`, () =>
    HttpResponse.json(ok(makePeriod()), { status: 201 }),
  ),
];
