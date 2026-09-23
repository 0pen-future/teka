import { http, HttpResponse } from "msw";

import type { Class } from "@/features/roster";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  Course,
  CourseInput,
  TuitionPack,
  TuitionPackInput,
} from "../schemas/courses-schemas";

// --- Fixtures ---
// One active course with a default template (the library fixtures' published
// Toán 6 v1), two tuition packs and one running class; one draft course with
// nothing attached.

export const courseToan6: Course = {
  id: "90000000-0000-4000-8000-000000000001",
  code: "TOAN-6",
  name: "Toán 6 nền tảng",
  subject: "Toán",
  level: "Lớp 6",
  description: "Khóa nền tảng cho học sinh mới vào lớp 6.",
  status: "active",
  default_template_version_id: "81000000-0000-4000-8000-000000000001",
  default_template: {
    version_id: "81000000-0000-4000-8000-000000000001",
    version_no: 1,
    status: "published",
    template_id: "80000000-0000-4000-8000-000000000001",
    code: "TOAN6",
    name: "Toán 6 cơ bản",
  },
  default_unit_price: 180000,
  total_sessions: 36,
  duration_min: 90,
  classes_running: 1,
  classes_upcoming: 0,
  tuition_packs: [
    {
      id: "91000000-0000-4000-8000-000000000001",
      name: "Gói 12 buổi",
      sessions: 12,
      price: 2000000,
      position: 1,
    },
    {
      id: "91000000-0000-4000-8000-000000000002",
      name: "Gói 24 buổi",
      sessions: 24,
      price: 3800000,
      position: 2,
    },
  ],
  created_at: "2026-09-01T08:00:00Z",
  updated_at: "2026-09-10T08:00:00Z",
};

export const courseVan9: Course = {
  id: "90000000-0000-4000-8000-000000000002",
  code: "VAN-9",
  name: "Văn 9 luyện thi",
  subject: "Ngữ văn",
  level: null,
  description: null,
  status: "draft",
  default_template_version_id: null,
  default_template: null,
  default_unit_price: 200000,
  total_sessions: null,
  duration_min: null,
  classes_running: 0,
  classes_upcoming: 0,
  tuition_packs: [],
  created_at: "2026-09-05T08:00:00Z",
  updated_at: "2026-09-05T08:00:00Z",
};

/** The one class attached to `courseToan6`, as `GET /classes?course_id=` returns it. */
export const classToan6A: Class = {
  id: "70000000-0000-4000-8000-000000000001",
  name: "Toán 6A",
  teacher_id: "73000000-0000-4000-8000-000000000001",
  start_date: "2026-01-05",
  end_date: null,
  default_unit_price: 180000,
  status: "active",
  schedules: [],
  created_at: "2026-01-01T08:00:00Z",
  my_staff_roles: [],
  student_count: 0,
  code: "TOAN6A",
  tags: [],
  recruiting: false,
  note: null,
  phase: "running",
  course: { id: courseToan6.id, code: courseToan6.code, name: courseToan6.name },
  parent_class_id: null,
  lineage_note: null,
};

interface CoursesStore {
  courses: Course[];
  classes: Class[];
}

function seed(): CoursesStore {
  return {
    courses: [
      { ...courseToan6, tuition_packs: courseToan6.tuition_packs.map((pack) => ({ ...pack })) },
      { ...courseVan9, tuition_packs: [] },
    ],
    classes: [{ ...classToan6A }],
  };
}

let store = seed();
let sequence = 0;

export function resetCoursesStore(): void {
  store = seed();
  sequence = 0;
}

export function getCoursesStore(): CoursesStore {
  return store;
}

function nextId(prefix: string): string {
  sequence += 1;
  return `${prefix}${String(sequence).padStart(4, "0")}`.padEnd(36, "0");
}

/** Mirrors the API's default-template join against the library fixtures. */
function templateFor(versionId: string | null): Course["default_template"] {
  if (versionId === "81000000-0000-4000-8000-000000000001") {
    return courseToan6.default_template;
  }
  return null;
}

function applyInput(course: Course, body: CourseInput): Course {
  course.code = body.code;
  course.name = body.name;
  course.subject = body.subject;
  course.level = body.level;
  course.description = body.description;
  course.status = body.status;
  course.default_template_version_id = body.default_template_version_id;
  course.default_template = templateFor(body.default_template_version_id);
  course.default_unit_price = body.default_unit_price;
  course.total_sessions = body.total_sessions;
  course.duration_min = body.duration_min;
  course.updated_at = new Date().toISOString();
  return course;
}

function codeTaken(code: string, exceptId?: string): boolean {
  return store.courses.some((course) => course.code === code && course.id !== exceptId);
}

export const coursesHandlers = [
  http.get(`${API_URL}/courses`, ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get("status");
    const q = url.searchParams.get("q")?.toLowerCase() ?? "";
    const items = store.courses.filter((course) => {
      if (status && course.status !== status) return false;
      if (q && !course.name.toLowerCase().includes(q) && !course.code.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.post(`${API_URL}/courses`, async ({ request }) => {
    const body = (await request.json()) as CourseInput;
    if (codeTaken(body.code)) {
      return HttpResponse.json(fail("CODE_TAKEN", "Mã khóa học đã được dùng trong trung tâm"), {
        status: 409,
      });
    }
    const course = applyInput(
      {
        ...courseVan9,
        id: nextId("course-"),
        classes_running: 0,
        classes_upcoming: 0,
        tuition_packs: [],
        created_at: new Date().toISOString(),
      },
      body,
    );
    store.courses.push(course);
    return HttpResponse.json(ok(course), { status: 201 });
  }),
  http.get(`${API_URL}/courses/:id`, ({ params }) => {
    const course = store.courses.find((item) => item.id === params.id);
    if (!course) {
      return HttpResponse.json(fail("NOT_FOUND", "course not found"), { status: 404 });
    }
    return HttpResponse.json(ok(course));
  }),
  http.put(`${API_URL}/courses/:id`, async ({ params, request }) => {
    const course = store.courses.find((item) => item.id === params.id);
    if (!course) {
      return HttpResponse.json(fail("NOT_FOUND", "course not found"), { status: 404 });
    }
    const body = (await request.json()) as CourseInput;
    if (codeTaken(body.code, course.id)) {
      return HttpResponse.json(fail("CODE_TAKEN", "Mã khóa học đã được dùng trong trung tâm"), {
        status: 409,
      });
    }
    return HttpResponse.json(ok(applyInput(course, body)));
  }),
  http.delete(`${API_URL}/courses/:id`, ({ params }) => {
    const course = store.courses.find((item) => item.id === params.id);
    if (!course) {
      return HttpResponse.json(fail("NOT_FOUND", "course not found"), { status: 404 });
    }
    if (store.classes.some((klass) => klass.course?.id === course.id)) {
      return HttpResponse.json(
        fail("COURSE_IN_USE", "Khóa học đang có lớp gắn vào, hãy lưu trữ thay vì xoá"),
        { status: 409 },
      );
    }
    store.courses = store.courses.filter((item) => item.id !== course.id);
    return new HttpResponse(null, { status: 204 });
  }),
  http.post(`${API_URL}/courses/:id/archive`, ({ params }) => {
    const course = store.courses.find((item) => item.id === params.id);
    if (!course) {
      return HttpResponse.json(fail("NOT_FOUND", "course not found"), { status: 404 });
    }
    if (course.status === "archived") {
      return HttpResponse.json(fail("COURSE_ARCHIVED", "Khóa học đã được lưu trữ"), {
        status: 409,
      });
    }
    course.status = "archived";
    course.updated_at = new Date().toISOString();
    return HttpResponse.json(ok(course));
  }),
  http.put(`${API_URL}/courses/:id/tuition-packs`, async ({ params, request }) => {
    const course = store.courses.find((item) => item.id === params.id);
    if (!course) {
      return HttpResponse.json(fail("NOT_FOUND", "course not found"), { status: 404 });
    }
    const body = (await request.json()) as TuitionPackInput[];
    const packs: TuitionPack[] = body.map((pack, index) => ({
      id: nextId("pack-"),
      name: pack.name,
      sessions: pack.sessions,
      price: pack.price,
      position: index + 1,
    }));
    course.tuition_packs = packs;
    return HttpResponse.json(ok(packs));
  }),
  // The Vận hành tab reads the roster list filtered by course.
  http.get(`${API_URL}/classes`, ({ request }) => {
    const courseId = new URL(request.url).searchParams.get("course_id");
    const items = store.classes.filter((klass) => !courseId || klass.course?.id === courseId);
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
];
