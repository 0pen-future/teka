import { http, HttpResponse } from "msw";

import type { Session } from "@/features/attendance";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  Class,
  ClassStaff,
  Contact,
  Enrollment,
  Schedule,
  Student,
} from "../schemas/roster-schemas";

// --- Fixtures ---
// Two contacts; the second has two children who share a full name, exactly
// the case `display_note` exists to disambiguate on the attendance screen.

export const contactSingleChild: Contact = {
  id: "40000000-0000-4000-8000-000000000001",
  full_name: "Nguyễn Thị Lan",
  phone: "+84912345678",
  student_count: 1,
  created_at: "2026-01-01T08:00:00Z",
};

export const contactTwoChildren: Contact = {
  id: "40000000-0000-4000-8000-000000000002",
  full_name: "Phạm Văn Hùng",
  phone: "+84987654321",
  student_count: 2,
  created_at: "2026-01-01T08:00:00Z",
};

export const studentOnlyChild: Student = {
  id: "50000000-0000-4000-8000-000000000001",
  full_name: "Trần Minh Khôi",
  display_note: "",
  contact_id: contactSingleChild.id,
  contact_name: contactSingleChild.full_name,
  contact_phone: contactSingleChild.phone,
  created_at: "2026-01-02T08:00:00Z",
};

export const studentSiblingOne: Student = {
  id: "50000000-0000-4000-8000-000000000002",
  full_name: "Nguyễn Văn An",
  display_note: "Anh, lớp 9A",
  contact_id: contactTwoChildren.id,
  contact_name: contactTwoChildren.full_name,
  contact_phone: contactTwoChildren.phone,
  created_at: "2026-01-02T08:00:00Z",
};

export const studentSiblingTwo: Student = {
  id: "50000000-0000-4000-8000-000000000003",
  full_name: "Nguyễn Văn An",
  display_note: "Em, lớp 7B",
  contact_id: contactTwoChildren.id,
  contact_name: contactTwoChildren.full_name,
  contact_phone: contactTwoChildren.phone,
  created_at: "2026-01-02T08:00:00Z",
};

export const classSchedule: Schedule = {
  id: "60000000-0000-4000-8000-000000000001",
  weekday: 2,
  start_time: "18:00",
  duration_min: 90,
  effective_from: "2026-01-05",
  effective_to: null,
};

export const classWithSchedule: Class = {
  id: "70000000-0000-4000-8000-000000000001",
  name: "Toán 6A",
  teacher_id: "73000000-0000-4000-8000-000000000001",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 150000,
  status: "active",
  schedules: [classSchedule],
  created_at: "2026-01-01T08:00:00Z",
  my_staff_roles: [],
};

/**
 * Member fixtures for the class-staff mock only, independent of the center
 * feature's own fixtures: two candidates for hoc_vu/tro_giang plus the
 * class's current teacher, who already holds the giao_vien stint below.
 */
export const staffCandidateHocVu = {
  id: "73000000-0000-4000-8000-000000000002",
  full_name: "Thầy Nam",
};
export const staffCandidateTroGiang = {
  id: "73000000-0000-4000-8000-000000000003",
  full_name: "Cô Hương",
};

const staffMemberNames: Record<string, string> = {
  [classWithSchedule.teacher_id]: "Cô Lan",
  [staffCandidateHocVu.id]: staffCandidateHocVu.full_name,
  [staffCandidateTroGiang.id]: staffCandidateTroGiang.full_name,
};

/** Internal store shape for a `class_staff` row — `class_id` never reaches the wire. */
interface ClassStaffFixture extends ClassStaff {
  class_id: string;
}

/** The class's dual-write giao_vien stint, seeded like Phase 1's backfill. */
export const classStaffGiaoVien: ClassStaffFixture = {
  id: "90000000-0000-4000-8000-000000000001",
  class_id: classWithSchedule.id,
  teacher_id: classWithSchedule.teacher_id,
  teacher_name: "Cô Lan",
  role_key: "giao_vien",
  role_label: "Giáo viên",
  started_at: "2026-01-01T08:00:00Z",
  ended_at: null,
};

export const enrollmentActive: Enrollment = {
  id: "80000000-0000-4000-8000-000000000001",
  student_id: studentSiblingOne.id,
  student_name: studentSiblingOne.full_name,
  class_id: classWithSchedule.id,
  class_name: classWithSchedule.name,
  started_on: "2026-01-05",
  ended_on: null,
  unit_price: classWithSchedule.default_unit_price,
  created_at: "2026-01-05T08:00:00Z",
};

/**
 * ISO date for a day of the month the test run executes in — the students
 * page only queries the current calendar month, so a fixed date would fall
 * outside its window as real time moves on.
 */
export function dayOfCurrentMonth(day: number): string {
  const now = new Date();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  return `${now.getFullYear()}-${month}-${String(day).padStart(2, "0")}`;
}

function makeSession(day: number, status: Session["status"]): Session {
  return {
    id: `session-${String(day).padStart(2, "0")}`,
    class_id: classWithSchedule.id,
    class_name: classWithSchedule.name,
    session_date: dayOfCurrentMonth(day),
    start_time: "18:00",
    status,
    cancel_reason: status === "cancelled" ? "Nghỉ lễ" : null,
    attendance_confirmed_at: null,
    student_count: 1,
    created_at: "2026-01-01T08:00:00Z",
  };
}

// --- In-memory store, reset before each test in the suite's beforeEach ---

export function seedRosterStore() {
  return {
    contacts: [contactSingleChild, contactTwoChildren].map((contact) => ({ ...contact })),
    students: [studentOnlyChild, studentSiblingOne, studentSiblingTwo].map((student) => ({
      ...student,
    })),
    classes: [{ ...classWithSchedule, schedules: [{ ...classSchedule }] }],
    classStaff: [{ ...classStaffGiaoVien }],
    enrollments: [{ ...enrollmentActive }],
    // Four countable sessions this month plus one cancelled — the BUỔI T{m}
    // column must skip the cancelled one.
    sessions: [
      makeSession(5, "held"),
      makeSession(8, "cancelled"),
      makeSession(12, "held"),
      makeSession(19, "planned"),
      makeSession(26, "planned"),
    ],
    // sessionId → absent student ids for the attendance-sheet handler; empty
    // by default (everyone present), tests mutate via `getRosterStore()`.
    absences: {} as Record<string, string[]>,
  };
}

let store = seedRosterStore();

export function resetRosterStore() {
  store = seedRosterStore();
}

/** Read-only peek for asserting what a flow actually persisted. */
export function getRosterStore() {
  return store;
}

let idCounter = 0;
function nextId(prefix: string) {
  idCounter += 1;
  return `${prefix}${String(idCounter).padStart(8, "0")}`;
}

/** Treats an empty-string form value the same as an absent one (`??` alone would not). */
function orNull(value: string | null | undefined): string | null {
  if (!value) {
    return null;
  }
  return value;
}

/** Same empty-string-aware fallback as `orNull`, but to today's date instead of `null`. */
function orToday(value: string | undefined): string {
  if (!value) {
    return new Date().toISOString().slice(0, 10);
  }
  return value;
}

export const rosterHandlers = [
  http.get(`${API_URL}/contacts`, ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get("query")?.toLowerCase() ?? "";
    const items = store.contacts.filter(
      (contact) => contact.full_name.toLowerCase().includes(query) || contact.phone.includes(query),
    );
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.get(`${API_URL}/contacts/:id`, ({ params }) => {
    const contact = store.contacts.find((item) => item.id === params.id);
    if (!contact) {
      return HttpResponse.json(fail("NOT_FOUND", "contact not found"), { status: 404 });
    }
    return HttpResponse.json(ok(contact));
  }),
  http.post(`${API_URL}/contacts`, async ({ request }) => {
    const body = (await request.json()) as { full_name: string; phone: string };
    const contact: Contact = {
      id: nextId("contact-"),
      full_name: body.full_name,
      phone: body.phone,
      student_count: 0,
      created_at: new Date().toISOString(),
    };
    store.contacts.push(contact);
    return HttpResponse.json(ok(contact), { status: 201 });
  }),
  http.put(`${API_URL}/contacts/:id`, async ({ params, request }) => {
    const contact = store.contacts.find((item) => item.id === params.id);
    if (!contact) {
      return HttpResponse.json(fail("NOT_FOUND", "contact not found"), { status: 404 });
    }
    const body = (await request.json()) as { full_name: string; phone: string };
    contact.full_name = body.full_name;
    contact.phone = body.phone;
    return HttpResponse.json(ok(contact));
  }),
  http.put(`${API_URL}/contacts/:id/zalo-mapping`, async ({ params, request }) => {
    const contact = store.contacts.find((item) => item.id === params.id);
    if (!contact) {
      return HttpResponse.json(fail("NOT_FOUND", "contact not found"), { status: 404 });
    }
    const body = (await request.json()) as { zalo_user_id: string; zalo_name: string };
    // Mirrors the API's unique index: one Zalo friend maps to one contact.
    const taken = store.contacts.some(
      (item) => item.id !== contact.id && item.zalo_user_id === body.zalo_user_id,
    );
    if (taken) {
      return HttpResponse.json(fail("CONFLICT", "friend already mapped to another contact"), {
        status: 409,
      });
    }
    contact.zalo_user_id = body.zalo_user_id;
    contact.zalo_name = body.zalo_name;
    return HttpResponse.json(ok(contact));
  }),
  http.delete(`${API_URL}/contacts/:id/zalo-mapping`, ({ params }) => {
    const contact = store.contacts.find((item) => item.id === params.id);
    if (!contact) {
      return HttpResponse.json(fail("NOT_FOUND", "contact not found"), { status: 404 });
    }
    delete contact.zalo_user_id;
    delete contact.zalo_name;
    return new HttpResponse(null, { status: 204 });
  }),

  http.get(`${API_URL}/students`, ({ request }) => {
    const url = new URL(request.url);
    const query = url.searchParams.get("query")?.toLowerCase() ?? "";
    const contactId = url.searchParams.get("contact_id");
    const classId = url.searchParams.get("class_id");
    const unenrolled = url.searchParams.get("unenrolled") === "true";
    const enrolledStudentIds = new Set(
      store.enrollments.filter((e) => e.class_id === classId).map((e) => e.student_id),
    );
    const anyOpenEnrollmentIds = new Set(
      store.enrollments.filter((e) => !e.ended_on).map((e) => e.student_id),
    );
    const items = store.students.filter((student) => {
      if (contactId && student.contact_id !== contactId) return false;
      if (classId && !enrolledStudentIds.has(student.id)) return false;
      if (unenrolled && anyOpenEnrollmentIds.has(student.id)) return false;
      return student.full_name.toLowerCase().includes(query);
    });
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.get(`${API_URL}/students/:id`, ({ params }) => {
    const student = store.students.find((item) => item.id === params.id);
    if (!student) {
      return HttpResponse.json(fail("NOT_FOUND", "student not found"), { status: 404 });
    }
    return HttpResponse.json(ok(student));
  }),
  http.post(`${API_URL}/students`, async ({ request }) => {
    const body = (await request.json()) as {
      full_name: string;
      contact_id: string;
      display_note?: string;
    };
    const contact = store.contacts.find((item) => item.id === body.contact_id);
    if (!contact) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "invalid contact", { contact_id: "Người liên hệ không hợp lệ" }),
        { status: 422 },
      );
    }
    const student: Student = {
      id: nextId("student-"),
      full_name: body.full_name,
      display_note: body.display_note ?? "",
      contact_id: contact.id,
      contact_name: contact.full_name,
      contact_phone: contact.phone,
      created_at: new Date().toISOString(),
    };
    store.students.push(student);
    contact.student_count += 1;
    return HttpResponse.json(ok(student), { status: 201 });
  }),
  http.put(`${API_URL}/students/:id`, async ({ params, request }) => {
    const student = store.students.find((item) => item.id === params.id);
    if (!student) {
      return HttpResponse.json(fail("NOT_FOUND", "student not found"), { status: 404 });
    }
    const body = (await request.json()) as {
      full_name: string;
      contact_id: string;
      display_note?: string;
    };
    student.full_name = body.full_name;
    student.display_note = body.display_note ?? "";
    student.contact_id = body.contact_id;
    return HttpResponse.json(ok(student));
  }),
  http.delete(`${API_URL}/students/:id`, ({ params }) => {
    const student = store.students.find((item) => item.id === params.id);
    if (!student) {
      return HttpResponse.json(fail("NOT_FOUND", "student not found"), { status: 404 });
    }
    student.full_name = "Học sinh đã ẩn danh";
    for (const enrollment of store.enrollments) {
      if (enrollment.student_id === student.id && !enrollment.ended_on) {
        enrollment.ended_on = new Date().toISOString().slice(0, 10);
      }
    }
    return new HttpResponse(null, { status: 204 });
  }),

  http.get(`${API_URL}/classes`, ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get("status");
    const items = store.classes.filter((klass) => {
      if (!status || status === "all") return true;
      return klass.status === status;
    });
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.get(`${API_URL}/classes/:id`, ({ params }) => {
    const klass = store.classes.find((item) => item.id === params.id);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    return HttpResponse.json(ok(klass));
  }),
  http.post(`${API_URL}/classes`, async ({ request }) => {
    const body = (await request.json()) as Omit<
      Class,
      "id" | "status" | "schedules" | "created_at"
    > & {
      schedules: Omit<Schedule, "id">[];
    };
    const klass: Class = {
      id: nextId("class-"),
      name: body.name,
      // The API assigns a new class to the creating teacher; the fixture
      // roster runs under one owner, so a single stable id stands in.
      teacher_id: "73000000-0000-4000-8000-000000000001",
      start_date: body.start_date,
      end_date: orNull(body.end_date),
      default_unit_price: body.default_unit_price,
      status: "active",
      schedules: body.schedules.map((schedule) => ({
        ...schedule,
        id: nextId("schedule-"),
        effective_to: orNull(schedule.effective_to),
      })),
      created_at: new Date().toISOString(),
      my_staff_roles: [],
    };
    store.classes.push(klass);
    return HttpResponse.json(ok(klass), { status: 201 });
  }),
  http.put(`${API_URL}/classes/:id`, async ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.id);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const body = (await request.json()) as {
      name: string;
      start_date: string;
      end_date?: string;
      default_unit_price: number;
    };
    klass.name = body.name;
    klass.start_date = body.start_date;
    klass.end_date = orNull(body.end_date);
    klass.default_unit_price = body.default_unit_price;
    return HttpResponse.json(ok(klass));
  }),
  http.put(`${API_URL}/classes/:id/teacher`, async ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.id);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const body = (await request.json()) as { teacher_id: string };
    // Mirror the server's future-planned-only move so the returned count is
    // realistic; held/cancelled/past sessions keep their teacher.
    const todayIso = new Date().toISOString().slice(0, 10);
    let moved = 0;
    for (const session of store.sessions) {
      if (
        session.class_id === klass.id &&
        session.status === "planned" &&
        session.session_date >= todayIso
      ) {
        moved += 1;
      }
    }
    klass.teacher_id = body.teacher_id;
    return HttpResponse.json(
      ok({ class_id: klass.id, teacher_id: body.teacher_id, moved_planned_sessions: moved }),
    );
  }),
  http.post(`${API_URL}/classes/:classId/schedules`, async ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.classId);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const body = (await request.json()) as Omit<Schedule, "id">;
    const schedule: Schedule = {
      ...body,
      id: nextId("schedule-"),
      effective_to: orNull(body.effective_to),
    };
    klass.schedules.push(schedule);
    return HttpResponse.json(ok(schedule), { status: 201 });
  }),
  http.put(`${API_URL}/classes/:classId/schedules/:scheduleId`, async ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.classId);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const schedule = klass.schedules.find((item) => item.id === params.scheduleId);
    if (!schedule) {
      return HttpResponse.json(fail("NOT_FOUND", "schedule not found"), { status: 404 });
    }
    const body = (await request.json()) as Omit<Schedule, "id">;
    Object.assign(schedule, body, { effective_to: orNull(body.effective_to) });
    return HttpResponse.json(ok(schedule));
  }),
  http.delete(`${API_URL}/classes/:classId/schedules/:scheduleId`, ({ params }) => {
    const klass = store.classes.find((item) => item.id === params.classId);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    klass.schedules = klass.schedules.filter((schedule) => schedule.id !== params.scheduleId);
    return new HttpResponse(null, { status: 204 });
  }),

  http.get(`${API_URL}/classes/:classId/staff`, ({ params }) => {
    const items = store.classStaff.filter((item) => item.class_id === params.classId);
    return HttpResponse.json(ok(items));
  }),
  http.post(`${API_URL}/classes/:classId/staff`, async ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.classId);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const body = (await request.json()) as { teacher_id: string; role_key: string };
    if (body.role_key === "giao_vien") {
      return HttpResponse.json(fail("CONFLICT", "giáo viên chính chỉ thay đổi qua bàn giao lớp"), {
        status: 409,
      });
    }
    if (body.role_key !== "hoc_vu" && body.role_key !== "tro_giang") {
      return HttpResponse.json(fail("VALIDATION_ERROR", "vai trò không hợp lệ"), { status: 422 });
    }
    const alreadyActive = store.classStaff.some(
      (item) => item.class_id === klass.id && item.teacher_id === body.teacher_id && !item.ended_at,
    );
    if (alreadyActive) {
      return HttpResponse.json(
        fail("CONFLICT", "người này đã có vai trò đang hoạt động trong lớp"),
        { status: 409 },
      );
    }
    const staff: ClassStaffFixture = {
      id: nextId("staff-"),
      class_id: klass.id,
      teacher_id: body.teacher_id,
      teacher_name: staffMemberNames[body.teacher_id] ?? "Thành viên",
      role_key: body.role_key,
      role_label: body.role_key === "hoc_vu" ? "Học vụ" : "Trợ giảng",
      started_at: new Date().toISOString(),
      ended_at: null,
    };
    store.classStaff.push(staff);
    return HttpResponse.json(ok(staff), { status: 201 });
  }),
  http.delete(`${API_URL}/classes/:classId/staff/:staffId`, ({ params, request }) => {
    const staff = store.classStaff.find(
      (item) => item.id === params.staffId && item.class_id === params.classId,
    );
    if (!staff) {
      return HttpResponse.json(fail("NOT_FOUND", "staff assignment not found"), { status: 404 });
    }
    if (staff.role_key === "giao_vien" && !staff.ended_at) {
      return HttpResponse.json(fail("CONFLICT", "giáo viên chính chỉ thay đổi qua bàn giao lớp"), {
        status: 409,
      });
    }
    const url = new URL(request.url);
    if (url.searchParams.get("mode") === "void") {
      store.classStaff = store.classStaff.filter((item) => item.id !== staff.id);
      return new HttpResponse(null, { status: 204 });
    }
    if (staff.ended_at) {
      return HttpResponse.json(fail("NOT_FOUND", "staff assignment not found"), { status: 404 });
    }
    staff.ended_at = new Date().toISOString();
    return new HttpResponse(null, { status: 204 });
  }),

  http.get(`${API_URL}/classes/:classId/sessions`, ({ params, request }) => {
    const url = new URL(request.url);
    const from = url.searchParams.get("from");
    const to = url.searchParams.get("to");
    const items = store.sessions.filter(
      (session) =>
        session.class_id === params.classId &&
        (!from || session.session_date >= from) &&
        (!to || session.session_date <= to),
    );
    return HttpResponse.json(ok(items));
  }),

  http.get(`${API_URL}/sessions/:id/attendance`, ({ params }) => {
    const session = store.sessions.find((item) => item.id === params.id);
    if (!session) {
      return HttpResponse.json(fail("NOT_FOUND", "session not found"), { status: 404 });
    }
    const absentIds = new Set(store.absences[session.id] ?? []);
    // One row per student enrolled as of the session date, like the API.
    const rows = store.enrollments
      .filter(
        (enrollment) =>
          enrollment.class_id === session.class_id &&
          enrollment.started_on <= session.session_date &&
          (!enrollment.ended_on || enrollment.ended_on >= session.session_date),
      )
      .map((enrollment) => {
        const student = store.students.find((item) => item.id === enrollment.student_id);
        const absent = absentIds.has(enrollment.student_id);
        return {
          student_id: enrollment.student_id,
          student_name: student?.full_name ?? enrollment.student_name,
          display_note: student?.display_note ?? null,
          enrollment_id: enrollment.id,
          status: session.status === "held" ? (absent ? "absent" : "present") : null,
          billable: session.status === "held" && !absent,
          note: null,
        };
      });
    return HttpResponse.json(
      ok({
        session_id: session.id,
        session_date: session.session_date,
        status: session.status,
        attendance_confirmed_at: session.attendance_confirmed_at,
        rows,
        warning: null,
      }),
    );
  }),

  http.get(`${API_URL}/classes/:classId/enrollable-students`, ({ params, request }) => {
    const klass = store.classes.find((item) => item.id === params.classId);
    if (!klass) {
      return HttpResponse.json(fail("NOT_FOUND", "class not found"), { status: 404 });
    }
    const url = new URL(request.url);
    const q = url.searchParams.get("q")?.trim().toLowerCase() ?? "";
    // Mirrors the API: under two characters the answer is an empty list, and
    // rows carry names only — never phone or contact.
    const items =
      q.length < 2
        ? []
        : store.students
            .filter((student) => student.full_name.toLowerCase().includes(q))
            .map((student) => ({ id: student.id, full_name: student.full_name }))
            .sort((a, b) => a.full_name.localeCompare(b.full_name))
            .slice(0, 20);
    return HttpResponse.json(ok(items));
  }),
  http.get(`${API_URL}/enrollments`, ({ request }) => {
    const url = new URL(request.url);
    const studentId = url.searchParams.get("student_id");
    const classId = url.searchParams.get("class_id");
    const active = url.searchParams.get("active");
    const items = store.enrollments.filter((enrollment) => {
      if (studentId && enrollment.student_id !== studentId) return false;
      if (classId && enrollment.class_id !== classId) return false;
      if (active === "true" && enrollment.ended_on) return false;
      return true;
    });
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.post(`${API_URL}/enrollments`, async ({ request }) => {
    const body = (await request.json()) as {
      student_id: string;
      class_id: string;
      started_on?: string;
    };
    const student = store.students.find((item) => item.id === body.student_id);
    const klass = store.classes.find((item) => item.id === body.class_id);
    if (!student || !klass) {
      return HttpResponse.json(fail("VALIDATION_ERROR", "invalid student or class"), {
        status: 422,
      });
    }
    const enrollment: Enrollment = {
      id: nextId("enrollment-"),
      student_id: student.id,
      student_name: student.full_name,
      class_id: klass.id,
      class_name: klass.name,
      started_on: orToday(body.started_on),
      ended_on: null,
      unit_price: klass.default_unit_price,
      created_at: new Date().toISOString(),
    };
    store.enrollments.push(enrollment);
    return HttpResponse.json(ok(enrollment), { status: 201 });
  }),
  http.post(`${API_URL}/enrollments/:id/end`, async ({ params, request }) => {
    const enrollment = store.enrollments.find((item) => item.id === params.id);
    if (!enrollment) {
      return HttpResponse.json(fail("NOT_FOUND", "enrollment not found"), { status: 404 });
    }
    if (enrollment.ended_on) {
      return HttpResponse.json(fail("CONFLICT", "enrollment already ended"), { status: 409 });
    }
    const body = (await request.json()) as { ended_on?: string };
    enrollment.ended_on = orToday(body.ended_on);
    return HttpResponse.json(ok(enrollment));
  }),
];
