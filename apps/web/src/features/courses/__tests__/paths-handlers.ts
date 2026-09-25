import { http, HttpResponse } from "msw";

import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  CoursePath,
  LearningPath,
  PathInput,
  Stage,
  StageInput,
} from "../schemas/paths-schemas";
import { courseToan6, getCoursesStore } from "./courses-handlers";

// --- Fixtures ---
// One active path with two stages (the first recommends Toán 6) and one
// draft path with no stage yet.

export const pathToan: LearningPath = {
  id: "92000000-0000-4000-8000-000000000001",
  code: "LT-TOAN",
  name: "Lộ trình Toán THCS",
  description: "Từ nền tảng lớp 6 tới ôn thi vào 10.",
  status: "active",
  stage_count: 2,
  course_count: 1,
  stages: [
    {
      id: "93000000-0000-4000-8000-000000000001",
      position: 1,
      name: "Nền tảng",
      goal: "Nắm chắc số học và hình học cơ bản.",
      courses: [
        {
          id: courseToan6.id,
          code: courseToan6.code,
          name: courseToan6.name,
          status: courseToan6.status,
          position: 1,
        },
      ],
    },
    {
      id: "93000000-0000-4000-8000-000000000002",
      position: 2,
      name: "Nâng cao",
      goal: null,
      courses: [],
    },
  ],
  created_at: "2026-09-02T08:00:00Z",
  updated_at: "2026-09-12T08:00:00Z",
};

export const pathVan: LearningPath = {
  id: "92000000-0000-4000-8000-000000000002",
  code: "LT-VAN",
  name: "Lộ trình Văn luyện thi",
  description: null,
  status: "draft",
  stage_count: 0,
  course_count: 0,
  stages: [],
  created_at: "2026-09-06T08:00:00Z",
  updated_at: "2026-09-06T08:00:00Z",
};

interface PathsStore {
  paths: LearningPath[];
}

function clone(path: LearningPath): LearningPath {
  return {
    ...path,
    stages: path.stages.map((stage) => ({
      ...stage,
      courses: stage.courses.map((course) => ({ ...course })),
    })),
  };
}

function seed(): PathsStore {
  return { paths: [clone(pathToan), clone(pathVan)] };
}

let store = seed();
let sequence = 0;

export function resetPathsStore(): void {
  store = seed();
  sequence = 0;
}

export function getPathsStore(): PathsStore {
  return store;
}

function nextId(prefix: string): string {
  sequence += 1;
  return `${prefix}${String(sequence).padStart(4, "0")}`.padEnd(36, "0");
}

/** Mirrors the API's derived counters after any stage or course change. */
function recount(path: LearningPath): LearningPath {
  path.stages.forEach((stage, index) => {
    stage.position = index + 1;
  });
  path.stage_count = path.stages.length;
  path.course_count = new Set(path.stages.flatMap((stage) => stage.courses.map((c) => c.id))).size;
  path.updated_at = new Date().toISOString();
  return path;
}

function applyInput(path: LearningPath, body: PathInput): LearningPath {
  path.code = body.code;
  path.name = body.name;
  path.description = body.description;
  path.status = body.status;
  path.updated_at = new Date().toISOString();
  return path;
}

function codeTaken(code: string, exceptId?: string): boolean {
  return store.paths.some((path) => path.code === code && path.id !== exceptId);
}

function findPath(id: string | readonly string[] | undefined): LearningPath | undefined {
  return store.paths.find((path) => path.id === id);
}

function findStage(
  path: LearningPath,
  id: string | readonly string[] | undefined,
): Stage | undefined {
  return path.stages.find((stage) => stage.id === id);
}

const pathNotFound = () =>
  HttpResponse.json(fail("NOT_FOUND", "learning path not found"), { status: 404 });
const stageNotFound = () =>
  HttpResponse.json(fail("NOT_FOUND", "path stage not found"), { status: 404 });

export const pathsHandlers = [
  http.get(`${API_URL}/paths`, ({ request }) => {
    const url = new URL(request.url);
    const status = url.searchParams.get("status");
    const q = url.searchParams.get("q")?.toLowerCase() ?? "";
    const items = store.paths.filter((path) => {
      if (status && path.status !== status) return false;
      if (q && !path.name.toLowerCase().includes(q) && !path.code.toLowerCase().includes(q)) {
        return false;
      }
      return true;
    });
    return HttpResponse.json(ok(items, listMeta(items.length)));
  }),
  http.post(`${API_URL}/paths`, async ({ request }) => {
    const body = (await request.json()) as PathInput;
    if (codeTaken(body.code)) {
      return HttpResponse.json(fail("CODE_TAKEN", "Mã lộ trình đã được dùng trong trung tâm"), {
        status: 409,
      });
    }
    const path = applyInput(
      {
        ...clone(pathVan),
        id: nextId("path-"),
        stages: [],
        stage_count: 0,
        course_count: 0,
        created_at: new Date().toISOString(),
      },
      body,
    );
    store.paths.push(path);
    return HttpResponse.json(ok(path), { status: 201 });
  }),
  http.get(`${API_URL}/paths/:id`, ({ params }) => {
    const path = findPath(params.id);
    return path ? HttpResponse.json(ok(path)) : pathNotFound();
  }),
  http.put(`${API_URL}/paths/:id`, async ({ params, request }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    const body = (await request.json()) as PathInput;
    if (codeTaken(body.code, path.id)) {
      return HttpResponse.json(fail("CODE_TAKEN", "Mã lộ trình đã được dùng trong trung tâm"), {
        status: 409,
      });
    }
    return HttpResponse.json(ok(applyInput(path, body)));
  }),
  http.delete(`${API_URL}/paths/:id`, ({ params }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    store.paths = store.paths.filter((item) => item.id !== path.id);
    return HttpResponse.json(ok({ deleted: true }));
  }),
  http.post(`${API_URL}/paths/:id/stages`, async ({ params, request }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    const body = (await request.json()) as StageInput;
    path.stages.push({
      id: nextId("stage-"),
      position: path.stages.length + 1,
      name: body.name,
      goal: body.goal,
      courses: [],
    });
    return HttpResponse.json(ok(recount(path)), { status: 201 });
  }),
  http.put(`${API_URL}/paths/:id/stages/order`, async ({ params, request }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    const { stage_ids: ids } = (await request.json()) as { stage_ids: string[] };
    const known = new Set(path.stages.map((stage) => stage.id));
    if (ids.length !== known.size || ids.some((id) => !known.has(id))) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", {
          stage_ids: "phải chứa đúng các giai đoạn của lộ trình",
        }),
        { status: 422 },
      );
    }
    path.stages = ids.map((id) => path.stages.find((stage) => stage.id === id)!);
    return HttpResponse.json(ok(recount(path)));
  }),
  http.put(`${API_URL}/paths/:id/stages/:sid`, async ({ params, request }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    const stage = findStage(path, params.sid);
    if (!stage) return stageNotFound();
    const body = (await request.json()) as StageInput;
    stage.name = body.name;
    stage.goal = body.goal;
    return HttpResponse.json(ok(recount(path)));
  }),
  http.delete(`${API_URL}/paths/:id/stages/:sid`, ({ params }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    if (!findStage(path, params.sid)) return stageNotFound();
    path.stages = path.stages.filter((stage) => stage.id !== params.sid);
    return HttpResponse.json(ok(recount(path)));
  }),
  http.put(`${API_URL}/paths/:id/stages/:sid/courses`, async ({ params, request }) => {
    const path = findPath(params.id);
    if (!path) return pathNotFound();
    const stage = findStage(path, params.sid);
    if (!stage) return stageNotFound();
    const { course_ids: ids } = (await request.json()) as { course_ids: string[] };
    const catalog = getCoursesStore().courses;
    const picked = ids.map((id) => catalog.find((course) => course.id === id));
    if (picked.some((course) => course?.status !== "active")) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "validation failed", {
          course_ids: "phải là khóa học còn hoạt động của trung tâm",
        }),
        { status: 422 },
      );
    }
    stage.courses = picked.map((course, index) => ({
      id: course!.id,
      code: course!.code,
      name: course!.name,
      status: course!.status,
      position: index + 1,
    }));
    return HttpResponse.json(ok(recount(path)));
  }),
  // The course detail page lists every live path that recommends the course.
  http.get(`${API_URL}/courses/:id/paths`, ({ params }) => {
    const rows: CoursePath[] = [];
    for (const path of store.paths) {
      for (const stage of path.stages) {
        if (stage.courses.some((course) => course.id === params.id)) {
          rows.push({
            id: path.id,
            code: path.code,
            name: path.name,
            status: path.status,
            stage_id: stage.id,
            stage_name: stage.name,
            stage_position: stage.position,
          });
        }
      }
    }
    return HttpResponse.json(ok(rows));
  }),
];
