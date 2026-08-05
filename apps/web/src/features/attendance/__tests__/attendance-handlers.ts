import { http, HttpResponse } from "msw";

import type { Class } from "@/features/roster";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type { AttendanceRow, ConfirmAttendanceInput, Session } from "../schemas/attendance-schemas";

/**
 * Fixture dates are relative to the run's current date, not a fixed calendar
 * date — `SessionsPage`'s default list window is the trailing 13 days, so a
 * hardcoded date would silently fall outside it (and this suite would start
 * failing) as real time moves on.
 */
function daysAgo(count: number): string {
  const date = new Date();
  date.setDate(date.getDate() - count);
  return date.toISOString().slice(0, 10);
}

// --- Fixtures ---
// One active class with a 30-student roster (PRD R2 AC1's class size),
// including two same-name siblings ("Nguyễn Văn An") distinguished only by
// `display_note` — exactly the case the attendance row's badge disambiguates.

export const fixtureClass: Class = {
  id: "90000000-0000-4000-8000-000000000001",
  name: "Toán 6A",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 150000,
  status: "active",
  schedules: [],
  created_at: "2026-01-01T08:00:00Z",
};

function buildRosterTemplate(): AttendanceRow[] {
  const rows: AttendanceRow[] = [];
  for (let i = 1; i <= 28; i += 1) {
    rows.push({
      student_id: `student-${String(i).padStart(3, "0")}`,
      student_name: `Học sinh ${i}`,
      display_note: "",
      enrollment_id: `enrollment-${String(i).padStart(3, "0")}`,
      status: null,
      billable: true,
      note: null,
    });
  }
  rows.push({
    student_id: "student-sibling-1",
    student_name: "Nguyễn Văn An",
    display_note: "Anh, lớp 9A",
    enrollment_id: "enrollment-sibling-1",
    status: null,
    billable: true,
    note: null,
  });
  rows.push({
    student_id: "student-sibling-2",
    student_name: "Nguyễn Văn An",
    display_note: "Em, lớp 7B",
    enrollment_id: "enrollment-sibling-2",
    status: null,
    billable: true,
    note: null,
  });
  return rows;
}

/** 30 rows total: 28 unique names plus the two "Nguyễn Văn An" siblings. */
export const fixtureRosterTemplate = buildRosterTemplate();

export const sessionUnconfirmedPast: Session = {
  id: "91000000-0000-4000-8000-000000000001",
  class_id: fixtureClass.id,
  class_name: fixtureClass.name,
  session_date: daysAgo(5),
  start_time: "18:00",
  status: "planned",
  cancel_reason: null,
  attendance_confirmed_at: null,
  student_count: fixtureRosterTemplate.length,
  created_at: "2026-01-01T08:00:00Z",
};

/** Already confirmed with two pre-existing absentees, for the reopen-and-edit case. */
export const sessionConfirmed: Session = {
  id: "91000000-0000-4000-8000-000000000002",
  class_id: fixtureClass.id,
  class_name: fixtureClass.name,
  session_date: daysAgo(2),
  start_time: "18:00",
  status: "held",
  cancel_reason: null,
  attendance_confirmed_at: "2026-01-01T19:00:00Z",
  student_count: fixtureRosterTemplate.length,
  created_at: "2026-01-01T08:00:00Z",
};

const confirmedAbsentIds = new Set(["student-001", "student-002"]);

/** Dated inside `closedPeriodMonths`, for the closed-period-warning case. */
export const sessionInClosedPeriod: Session = {
  id: "91000000-0000-4000-8000-000000000003",
  class_id: fixtureClass.id,
  class_name: fixtureClass.name,
  session_date: daysAgo(60),
  start_time: "18:00",
  status: "planned",
  cancel_reason: null,
  attendance_confirmed_at: null,
  student_count: fixtureRosterTemplate.length,
  created_at: "2026-01-01T08:00:00Z",
};

/** Already cancelled, bills nobody — for the cancelled-session empty state. */
export const sessionCancelled: Session = {
  id: "91000000-0000-4000-8000-000000000004",
  class_id: fixtureClass.id,
  class_name: fixtureClass.name,
  session_date: daysAgo(3),
  start_time: "18:00",
  status: "cancelled",
  cancel_reason: "Nghỉ lễ",
  attendance_confirmed_at: null,
  student_count: fixtureRosterTemplate.length,
  created_at: "2026-01-01T08:00:00Z",
};

/** Months (`"YYYY-MM"`) the `/billing-periods` handler treats as closed. */
const closedPeriodMonths = new Set([sessionInClosedPeriod.session_date.slice(0, 7)]);

// --- In-memory store, reset before each test in the suite's beforeEach ---

interface Store {
  classes: Class[];
  sessions: Session[];
  rosters: Map<string, AttendanceRow[]>;
}

function seedAttendanceStore(): Store {
  const confirmedRows = fixtureRosterTemplate.map((row) => ({
    ...row,
    status: confirmedAbsentIds.has(row.student_id) ? ("absent" as const) : ("present" as const),
    billable: !confirmedAbsentIds.has(row.student_id),
  }));
  return {
    classes: [{ ...fixtureClass, schedules: [] }],
    sessions: [
      { ...sessionUnconfirmedPast },
      { ...sessionConfirmed },
      { ...sessionInClosedPeriod },
      { ...sessionCancelled },
    ],
    rosters: new Map([
      [sessionUnconfirmedPast.id, fixtureRosterTemplate.map((row) => ({ ...row }))],
      [sessionConfirmed.id, confirmedRows],
      [sessionInClosedPeriod.id, fixtureRosterTemplate.map((row) => ({ ...row }))],
      [sessionCancelled.id, fixtureRosterTemplate.map((row) => ({ ...row }))],
    ]),
  };
}

let store = seedAttendanceStore();

export function resetAttendanceStore() {
  store = seedAttendanceStore();
}

let idCounter = 0;
function nextId(prefix: string) {
  idCounter += 1;
  return `${prefix}${String(idCounter).padStart(8, "0")}`;
}

export const attendanceHandlers = [
  http.get(`${API_URL}/classes`, ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get("status");
    const items = store.classes.filter(
      (klass) => !status || status === "all" || klass.status === status,
    );
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),

  http.get(`${API_URL}/classes/:classId/sessions`, ({ params, request }) => {
    const classId = params.classId as string;
    const url = new URL(request.url);
    const from = url.searchParams.get("from") ?? "";
    const to = url.searchParams.get("to") ?? "";
    const items = store.sessions.filter(
      (session) =>
        session.class_id === classId &&
        (!from || session.session_date >= from) &&
        (!to || session.session_date <= to),
    );
    return HttpResponse.json(ok(items));
  }),

  http.get(`${API_URL}/sessions/:id`, ({ params }) => {
    const session = store.sessions.find((item) => item.id === params.id);
    if (!session) {
      return HttpResponse.json(fail("NOT_FOUND", "session not found"), { status: 404 });
    }
    return HttpResponse.json(ok(session));
  }),

  http.get(`${API_URL}/sessions/:id/attendance`, ({ params }) => {
    const sessionId = params.id as string;
    const session = store.sessions.find((item) => item.id === sessionId);
    const rows = store.rosters.get(sessionId);
    if (!session || !rows) {
      return HttpResponse.json(fail("NOT_FOUND", "session not found"), { status: 404 });
    }
    return HttpResponse.json(
      ok({
        session_id: session.id,
        session_date: session.session_date,
        status: session.status,
        attendance_confirmed_at: session.attendance_confirmed_at,
        rows,
      }),
    );
  }),

  http.post(`${API_URL}/sessions/:id/attendance`, async ({ params, request }) => {
    const sessionId = params.id as string;
    const session = store.sessions.find((item) => item.id === sessionId);
    const rows = store.rosters.get(sessionId);
    if (!session || !rows) {
      return HttpResponse.json(fail("NOT_FOUND", "session not found"), { status: 404 });
    }
    const body = (await request.json()) as ConfirmAttendanceInput;
    const absentIds = new Set(body.absent_student_ids);
    const nextRows: AttendanceRow[] = rows.map((row) => ({
      ...row,
      status: absentIds.has(row.student_id) ? "absent" : "present",
      billable: !absentIds.has(row.student_id),
    }));
    store.rosters.set(sessionId, nextRows);
    session.status = "held";
    session.attendance_confirmed_at = new Date().toISOString();

    const closed = closedPeriodMonths.has(session.session_date.slice(0, 7));
    return HttpResponse.json(
      ok({
        session_id: session.id,
        session_date: session.session_date,
        status: session.status,
        attendance_confirmed_at: session.attendance_confirmed_at,
        rows: nextRows,
        warning: closed
          ? "Buổi học này thuộc kỳ đã chốt sổ; thay đổi được ghi nhận là điều chỉnh ở kỳ kế tiếp."
          : null,
      }),
    );
  }),

  http.post(`${API_URL}/sessions/:id/cancel`, async ({ params, request }) => {
    const sessionId = params.id as string;
    const session = store.sessions.find((item) => item.id === sessionId);
    if (!session) {
      return HttpResponse.json(fail("NOT_FOUND", "session not found"), { status: 404 });
    }
    if (session.attendance_confirmed_at) {
      return HttpResponse.json(fail("CONFLICT", "session already confirmed"), { status: 409 });
    }
    const body = (await request.json()) as { reason: string };
    session.status = "cancelled";
    session.cancel_reason = body.reason;
    return HttpResponse.json(ok(session));
  }),

  http.post(`${API_URL}/billing-periods`, async ({ request }) => {
    const body = (await request.json()) as { year: number; month: number };
    const key = `${body.year}-${String(body.month).padStart(2, "0")}`;
    const status = closedPeriodMonths.has(key) ? "closed" : "open";
    return HttpResponse.json(
      ok({
        id: nextId("period-"),
        year: body.year,
        month: body.month,
        period_start: `${key}-01`,
        period_end: `${key}-28`,
        status,
        closed_at: status === "closed" ? new Date().toISOString() : null,
      }),
      { status: 201 },
    );
  }),
];
