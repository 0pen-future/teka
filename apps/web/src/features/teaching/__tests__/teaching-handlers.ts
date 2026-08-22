import { http, HttpResponse } from "msw";

import { API_URL, fail, ok, primaryTeacher } from "@/test/msw/handlers";

import { transitionLessonPlanStatus, type LessonPlanStatus } from "../lib/teaching-store";
import type {
  MarkEntryInput,
  MarkResponse,
  PlanResponse,
  PutCurriculumInput,
  SavePlanInput,
} from "../schemas/teaching-schemas";

/**
 * Stateful msw handlers for the teaching endpoints (cf. the attendance
 * feature's `attendance-handlers.ts`): an in-memory store so save → re-read
 * flows round-trip in tests. Register with `server.use(...teachingHandlers)`
 * and call `resetTeachingApiStore()` in `beforeEach`; seed via the exported
 * helpers. Status transitions reuse `transitionLessonPlanStatus`, the same
 * table the Go service enforces — an illegal move answers 409 like the API.
 */

interface SessionRef {
  id: string;
  class_id: string;
  /** YYYY-MM-DD — the month read filters on its YYYY-MM prefix. */
  session_date: string;
}

interface Store {
  /** classId → curriculum wire shape. */
  curricula: Map<string, { lessons: string[]; current_index: number }>;
  /** `classId#lessonIndex` → plan wire row. */
  plans: Map<string, PlanResponse>;
  /** sessionId → note body. */
  notes: Map<string, string>;
  /** `sessionId#studentId` → mark fields. */
  marks: Map<string, { score: number | null; personal_note: string | null }>;
  /** Sessions the month read can attribute notes/marks to. */
  sessions: Map<string, SessionRef>;
}

let store: Store = {
  curricula: new Map(),
  plans: new Map(),
  notes: new Map(),
  marks: new Map(),
  sessions: new Map(),
};

/** Read-only peek for asserting what a flow actually persisted. */
export function getTeachingApiStore(): Store {
  return store;
}

export function resetTeachingApiStore(): void {
  store = {
    curricula: new Map(),
    plans: new Map(),
    notes: new Map(),
    marks: new Map(),
    sessions: new Map(),
  };
}

// --- Seed helpers ---

/** The month read only sees notes/marks of sessions it knows; register them here. */
export function seedTeachingSession(session: SessionRef): void {
  store.sessions.set(session.id, session);
}

export function seedCurriculum(classId: string, lessons: string[], currentIndex = 0): void {
  store.curricula.set(classId, { lessons, current_index: currentIndex });
}

export function seedPlan(
  classId: string,
  lessonIndex: number,
  overrides: Partial<PlanResponse> = {},
): void {
  store.plans.set(`${classId}#${lessonIndex}`, {
    class_id: classId,
    lesson_index: lessonIndex,
    goal: "",
    activities: [],
    homework: "",
    file_name: null,
    status: "draft",
    redo_note: null,
    owner_comment: null,
    submitted_by: null,
    submitted_by_name: null,
    submitted_at: null,
    ...overrides,
  });
}

export function seedNote(sessionId: string, body: string): void {
  store.notes.set(sessionId, body);
}

export function seedMark(
  sessionId: string,
  studentId: string,
  fields: { score?: number | null; personal_note?: string | null },
): void {
  store.marks.set(`${sessionId}#${studentId}`, {
    score: fields.score ?? null,
    personal_note: fields.personal_note ?? null,
  });
}

// --- Handlers ---

const actionNames: Record<string, "submit" | "approve" | "requestRedo" | "reopen"> = {
  submit: "submit",
  approve: "approve",
  "request-redo": "requestRedo",
  reopen: "reopen",
};

function sessionMarks(sessionId: string): MarkResponse[] {
  const rows: MarkResponse[] = [];
  for (const [key, fields] of store.marks) {
    const [session_id = "", student_id = ""] = key.split("#");
    if (session_id === sessionId) {
      rows.push({ session_id, student_id, ...fields });
    }
  }
  return rows;
}

export const teachingHandlers = [
  http.get(`${API_URL}/classes/:id/curriculum`, ({ params }) => {
    const curriculum = store.curricula.get(params.id as string);
    return HttpResponse.json(ok(curriculum ?? { lessons: [], current_index: 0 }));
  }),

  http.put(`${API_URL}/classes/:id/curriculum`, async ({ params, request }) => {
    const input = (await request.json()) as PutCurriculumInput;
    const clamped = Math.max(0, Math.min(input.current_index ?? 0, input.lessons.length - 1));
    const curriculum = {
      lessons: input.lessons,
      current_index: input.lessons.length === 0 ? 0 : clamped,
    };
    store.curricula.set(params.id as string, curriculum);
    return HttpResponse.json(ok(curriculum));
  }),

  http.get(`${API_URL}/classes/:id/lesson-plans`, ({ params }) => {
    const plans = [...store.plans.values()]
      .filter((plan) => plan.class_id === params.id)
      .sort((a, b) => a.lesson_index - b.lesson_index);
    return HttpResponse.json(ok(plans));
  }),

  http.put(`${API_URL}/classes/:id/lesson-plans/:index`, async ({ params, request }) => {
    const classId = params.id as string;
    const lessonIndex = Number(params.index);
    const key = `${classId}#${lessonIndex}`;
    const current = store.plans.get(key);
    const next = transitionLessonPlanStatus(current?.status ?? "none", "save");
    if (next === null) {
      return HttpResponse.json(fail("CONFLICT", "plan status does not allow saving"), {
        status: 409,
      });
    }
    const input = (await request.json()) as SavePlanInput;
    const plan: PlanResponse = {
      class_id: classId,
      lesson_index: lessonIndex,
      goal: input.goal,
      activities: input.activities,
      homework: input.homework,
      file_name: input.file_name,
      status: next as Exclude<LessonPlanStatus, "none">,
      redo_note: current?.redo_note ?? null,
      owner_comment: current?.owner_comment ?? null,
      submitted_by: current?.submitted_by ?? null,
      submitted_by_name: current?.submitted_by_name ?? null,
      submitted_at: current?.submitted_at ?? null,
    };
    store.plans.set(key, plan);
    return HttpResponse.json(ok(plan));
  }),

  http.post(`${API_URL}/classes/:id/lesson-plans/:index/:action`, async ({ params, request }) => {
    const action = actionNames[params.action as string];
    if (!action) {
      return HttpResponse.json(fail("NOT_FOUND", "route not found"), { status: 404 });
    }
    const key = `${params.id as string}#${Number(params.index)}`;
    const current = store.plans.get(key);
    const next = transitionLessonPlanStatus(current?.status ?? "none", action);
    if (!current || next === null) {
      return HttpResponse.json(fail("CONFLICT", "plan status does not allow this action"), {
        status: 409,
      });
    }
    const body = (await request.json().catch(() => ({}))) as { comment?: string };
    const comment = (body.comment ?? "").trim();
    if (action === "requestRedo" && comment === "") {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", { comment: "comment is required" }),
        { status: 422 },
      );
    }
    const plan: PlanResponse = {
      ...current,
      status: next as Exclude<LessonPlanStatus, "none">,
      ...(action === "submit"
        ? {
            redo_note: null,
            submitted_by: primaryTeacher.id,
            submitted_by_name: primaryTeacher.full_name,
            submitted_at: new Date().toISOString(),
          }
        : {}),
      ...(action === "approve" ? { owner_comment: comment === "" ? null : comment } : {}),
      ...(action === "requestRedo" ? { redo_note: comment } : {}),
      ...(action === "reopen" ? { redo_note: null, owner_comment: null } : {}),
    };
    store.plans.set(key, plan);
    return HttpResponse.json(ok(plan));
  }),

  http.get(`${API_URL}/classes/:id/marks`, ({ params, request }) => {
    const month = new URL(request.url).searchParams.get("month") ?? "";
    if (!/^\d{4}-\d{2}$/.test(month)) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", { month: "month must be YYYY-MM" }),
        { status: 422 },
      );
    }
    const monthSessions = new Set(
      [...store.sessions.values()]
        .filter((s) => s.class_id === params.id && s.session_date.startsWith(month))
        .map((s) => s.id),
    );
    const session_notes = [...store.notes]
      .filter(([sessionId]) => monthSessions.has(sessionId))
      .map(([session_id, body]) => ({ session_id, body }));
    const marks = [...monthSessions].flatMap((sessionId) => sessionMarks(sessionId));
    return HttpResponse.json(ok({ session_notes, marks }));
  }),

  http.put(`${API_URL}/sessions/:id/note`, async ({ params, request }) => {
    const sessionId = params.id as string;
    const { body } = (await request.json()) as { body: string };
    if (body.trim() === "") {
      store.notes.delete(sessionId);
      return HttpResponse.json(ok({ session_id: sessionId, body: "" }));
    }
    store.notes.set(sessionId, body);
    return HttpResponse.json(ok({ session_id: sessionId, body }));
  }),

  http.get(`${API_URL}/teaching/review-queue`, () => {
    const items = [...store.plans.values()]
      .filter((plan) => plan.status === "pending")
      .sort((a, b) => (a.submitted_at ?? "").localeCompare(b.submitted_at ?? ""))
      .map((plan) => ({
        plan_id: `${plan.class_id}#${plan.lesson_index}`,
        class_id: plan.class_id,
        class_name: "",
        lesson_index: plan.lesson_index,
        lesson_title: store.curricula.get(plan.class_id)?.lessons[plan.lesson_index] ?? null,
        teacher_name: plan.submitted_by_name,
        submitted_at: plan.submitted_at,
      }));
    return HttpResponse.json(ok(items));
  }),

  http.put(`${API_URL}/sessions/:id/marks`, async ({ params, request }) => {
    const sessionId = params.id as string;
    const entries = (await request.json()) as MarkEntryInput[];
    for (const entry of entries) {
      const key = `${sessionId}#${entry.student_id}`;
      const row = store.marks.get(key) ?? { score: null, personal_note: null };
      if ("score" in entry) {
        row.score = entry.score ?? null;
      }
      if ("personal_note" in entry) {
        const trimmed = (entry.personal_note ?? "").trim();
        row.personal_note = trimmed === "" ? null : trimmed;
      }
      if (row.score === null && row.personal_note === null) {
        store.marks.delete(key);
      } else {
        store.marks.set(key, row);
      }
    }
    return HttpResponse.json(ok(sessionMarks(sessionId)));
  }),
];
