import { http, HttpResponse } from "msw";

import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  Exercise,
  ExerciseInput,
  LessonExercise,
  LessonExerciseInput,
  LessonInput,
  LessonMaterial,
  LessonMaterialInput,
  LogField,
  LogFieldInput,
  Material,
  MaterialInput,
  ProgramTemplate,
  ScoreComponent,
  ScoreComponentInput,
  TemplateInput,
  TemplateLesson,
  TemplateLessonDetail,
  TemplateVersion,
  VersionDetail,
} from "../schemas/library-schemas";

// --- Fixtures ---
// One template with a published v1 (two lessons) and an open draft v2 that
// copied them, plus a second template whose only version is the draft v1.

const OWNER_ID = "73000000-0000-4000-8000-000000000001";

export const templateToan6: ProgramTemplate = {
  id: "80000000-0000-4000-8000-000000000001",
  code: "TOAN6",
  name: "Toán 6 cơ bản",
  subject: "Toán",
  level: "Lớp 6",
  description: "Chương trình Toán 6 theo SGK mới.",
  created_by: OWNER_ID,
  published_version_no: 1,
  draft_version_no: 2,
  version_count: 2,
  created_at: "2026-09-01T08:00:00Z",
  updated_at: "2026-09-10T08:00:00Z",
};

export const templateVan9: ProgramTemplate = {
  id: "80000000-0000-4000-8000-000000000002",
  code: "VAN9",
  name: "Văn 9 luyện thi",
  subject: "Ngữ văn",
  level: null,
  description: null,
  created_by: OWNER_ID,
  published_version_no: null,
  draft_version_no: 1,
  version_count: 1,
  created_at: "2026-09-05T08:00:00Z",
  updated_at: "2026-09-05T08:00:00Z",
};

export const versionToan6Published: TemplateVersion = {
  id: "81000000-0000-4000-8000-000000000001",
  template_id: templateToan6.id,
  version_no: 1,
  status: "published",
  changelog: null,
  published_at: "2026-09-08T08:00:00Z",
  created_by: OWNER_ID,
  lesson_count: 2,
  created_at: "2026-09-01T08:00:00Z",
  updated_at: "2026-09-08T08:00:00Z",
};

export const versionToan6Draft: TemplateVersion = {
  id: "81000000-0000-4000-8000-000000000002",
  template_id: templateToan6.id,
  version_no: 2,
  status: "draft",
  changelog: "Thêm buổi ôn tập",
  published_at: null,
  created_by: OWNER_ID,
  lesson_count: 2,
  created_at: "2026-09-10T08:00:00Z",
  updated_at: "2026-09-10T08:00:00Z",
};

export const versionVan9Draft: TemplateVersion = {
  id: "81000000-0000-4000-8000-000000000003",
  template_id: templateVan9.id,
  version_no: 1,
  status: "draft",
  changelog: null,
  published_at: null,
  created_by: OWNER_ID,
  lesson_count: 0,
  created_at: "2026-09-05T08:00:00Z",
  updated_at: "2026-09-05T08:00:00Z",
};

function lesson(
  id: string,
  versionId: string,
  position: number,
  title: string,
  extra: Partial<TemplateLesson> = {},
): TemplateLesson {
  return {
    id,
    version_id: versionId,
    position,
    title,
    objectives: null,
    duration_min: 90,
    homework_note: null,
    created_at: "2026-09-01T08:00:00Z",
    updated_at: "2026-09-01T08:00:00Z",
    ...extra,
  };
}

export const lessonPublishedSoTuNhien = lesson(
  "82000000-0000-4000-8000-000000000001",
  versionToan6Published.id,
  1,
  "Số tự nhiên",
);
export const lessonPublishedPhanSo = lesson(
  "82000000-0000-4000-8000-000000000002",
  versionToan6Published.id,
  2,
  "Phân số",
);
export const lessonDraftSoTuNhien = lesson(
  "82000000-0000-4000-8000-000000000003",
  versionToan6Draft.id,
  1,
  "Số tự nhiên",
  { objectives: "Nhận biết tập N", homework_note: "Bài 1-5 trang 10" },
);
export const lessonDraftPhanSo = lesson(
  "82000000-0000-4000-8000-000000000004",
  versionToan6Draft.id,
  2,
  "Phân số",
  { duration_min: null },
);

// Catalog items: one material and one exercise attached to the draft's
// first lesson, one of each left free so a picker has something to add.

export const materialSlide: Material = {
  id: "83000000-0000-4000-8000-000000000001",
  title: "Slide số tự nhiên",
  kind: "doc",
  url: "https://example.com/slide-so-tu-nhien.pdf",
  description: "Slide bài giảng chương 1.",
  tags: ["chương 1", "slide"],
  created_at: "2026-09-02T08:00:00Z",
  updated_at: "2026-09-02T08:00:00Z",
};

export const materialVideo: Material = {
  id: "83000000-0000-4000-8000-000000000002",
  title: "Video phân số",
  kind: "video",
  url: "https://example.com/video-phan-so",
  description: null,
  tags: [],
  created_at: "2026-09-03T08:00:00Z",
  updated_at: "2026-09-03T08:00:00Z",
};

export const exerciseBai1: Exercise = {
  id: "84000000-0000-4000-8000-000000000001",
  title: "Bài 1: Tập hợp",
  description: "Liệt kê phần tử của tập hợp.",
  difficulty: 2,
  tags: ["chương 1"],
  created_at: "2026-09-02T08:00:00Z",
  updated_at: "2026-09-02T08:00:00Z",
};

export const exerciseBai2: Exercise = {
  id: "84000000-0000-4000-8000-000000000002",
  title: "Bài 2: So sánh phân số",
  description: null,
  difficulty: null,
  tags: [],
  created_at: "2026-09-03T08:00:00Z",
  updated_at: "2026-09-03T08:00:00Z",
};

interface MaterialLink {
  lesson_id: string;
  material_id: string;
  shared_with_students: boolean;
  position: number;
}

interface ExerciseLink {
  lesson_id: string;
  exercise_id: string;
  position: number;
}

type StoredLogField = LogField & { version_id: string };

interface LibraryStore {
  templates: ProgramTemplate[];
  versions: TemplateVersion[];
  lessons: TemplateLesson[];
  materials: Material[];
  exercises: Exercise[];
  materialLinks: MaterialLink[];
  exerciseLinks: ExerciseLink[];
  logFields: StoredLogField[];
  scoreSets: Record<string, ScoreComponent[]>;
  nextId: number;
}

let store: LibraryStore = freshStore();

function freshStore(): LibraryStore {
  return {
    templates: [structuredClone(templateToan6), structuredClone(templateVan9)],
    versions: [
      structuredClone(versionToan6Published),
      structuredClone(versionToan6Draft),
      structuredClone(versionVan9Draft),
    ],
    lessons: [
      structuredClone(lessonPublishedSoTuNhien),
      structuredClone(lessonPublishedPhanSo),
      structuredClone(lessonDraftSoTuNhien),
      structuredClone(lessonDraftPhanSo),
    ],
    materials: [structuredClone(materialSlide), structuredClone(materialVideo)],
    exercises: [structuredClone(exerciseBai1), structuredClone(exerciseBai2)],
    materialLinks: [
      {
        lesson_id: lessonPublishedSoTuNhien.id,
        material_id: materialSlide.id,
        shared_with_students: false,
        position: 1,
      },
      {
        lesson_id: lessonDraftSoTuNhien.id,
        material_id: materialSlide.id,
        shared_with_students: true,
        position: 1,
      },
    ],
    exerciseLinks: [
      { lesson_id: lessonDraftSoTuNhien.id, exercise_id: exerciseBai1.id, position: 1 },
    ],
    logFields: [
      {
        id: "85000000-0000-4000-8000-000000000001",
        version_id: versionToan6Published.id,
        position: 1,
        label: "Mức độ tập trung",
        kind: "select",
        options: ["Tốt", "Khá"],
        required: true,
      },
      {
        id: "85000000-0000-4000-8000-000000000002",
        version_id: versionToan6Draft.id,
        position: 1,
        label: "Ghi chú buổi học",
        kind: "text",
        options: [],
        required: false,
      },
    ],
    scoreSets: {
      [versionToan6Published.id]: [{ key: "kt", label: "Kiểm tra", max: 10, weight: 1 }],
      [versionToan6Draft.id]: [
        { key: "hw", label: "Bài tập về nhà", max: 10, weight: 0.4 },
        { key: "kt", label: "Kiểm tra", max: 10, weight: 0.6 },
      ],
    },
    nextId: 1,
  };
}

export function resetLibraryStore(): void {
  store = freshStore();
}

export function getLibraryStore(): LibraryStore {
  return store;
}

function mintId(): string {
  const n = String(store.nextId++).padStart(12, "0");
  return `89000000-0000-4000-8000-${n}`;
}

const NOW = "2026-09-20T08:00:00Z";

function notFound(resource: string) {
  return HttpResponse.json(fail("NOT_FOUND", `${resource} not found`), { status: 404 });
}

/** Recomputes the template's summary columns the way the API's list query does. */
function summarize(template: ProgramTemplate): ProgramTemplate {
  const versions = store.versions.filter((v) => v.template_id === template.id);
  const published = versions.find((v) => v.status === "published");
  const draft = versions.find((v) => v.status === "draft");
  return {
    ...template,
    published_version_no: published?.version_no ?? null,
    draft_version_no: draft?.version_no ?? null,
    version_count: versions.length,
  };
}

function withCount(version: TemplateVersion): TemplateVersion {
  return {
    ...version,
    lesson_count: store.lessons.filter((l) => l.version_id === version.id).length,
  };
}

function lessonMaterials(lessonId: string): LessonMaterial[] {
  return store.materialLinks
    .filter((link) => link.lesson_id === lessonId)
    .sort((a, b) => a.position - b.position)
    .flatMap((link) => {
      const material = store.materials.find((m) => m.id === link.material_id);
      return material
        ? [
            {
              ...material,
              shared_with_students: link.shared_with_students,
              position: link.position,
            },
          ]
        : [];
    });
}

function lessonExercises(lessonId: string): LessonExercise[] {
  return store.exerciseLinks
    .filter((link) => link.lesson_id === lessonId)
    .sort((a, b) => a.position - b.position)
    .flatMap((link) => {
      const exercise = store.exercises.find((e) => e.id === link.exercise_id);
      return exercise ? [{ ...exercise, position: link.position }] : [];
    });
}

function lessonDetail(row: TemplateLesson): TemplateLessonDetail {
  return { ...row, materials: lessonMaterials(row.id), exercises: lessonExercises(row.id) };
}

function versionLogFields(versionId: string): LogField[] {
  return store.logFields
    .filter((f) => f.version_id === versionId)
    .sort((a, b) => a.position - b.position)
    .map((f) => ({
      id: f.id,
      position: f.position,
      label: f.label,
      kind: f.kind,
      options: f.options,
      required: f.required,
    }));
}

function versionDetail(version: TemplateVersion): VersionDetail {
  return {
    ...withCount(version),
    score_set: store.scoreSets[version.id] ?? [],
    log_fields: versionLogFields(version.id),
    lessons: renumber(version.id).map(lessonDetail),
  };
}

function locked(version: TemplateVersion | undefined) {
  return version?.status === "draft"
    ? null
    : HttpResponse.json(fail("VERSION_LOCKED", "phiên bản đã khoá"), { status: 409 });
}

function validationError(fields: Record<string, string>) {
  return HttpResponse.json(fail("VALIDATION_ERROR", "validation failed", fields), { status: 422 });
}

function itemFilter(request: Request) {
  const q = (new URL(request.url).searchParams.get("q") ?? "").toLowerCase();
  return (item: { title: string }) => q === "" || item.title.toLowerCase().includes(q);
}

function renumber(versionId: string): TemplateLesson[] {
  const rows = store.lessons
    .filter((l) => l.version_id === versionId)
    .sort((a, b) => a.position - b.position);
  rows.forEach((row, index) => {
    row.position = index + 1;
  });
  return rows;
}

export const libraryHandlers = [
  http.get(`${API_URL}/library/templates`, ({ request }) => {
    const q = (new URL(request.url).searchParams.get("q") ?? "").toLowerCase();
    const rows = store.templates
      .filter(
        (t) => q === "" || t.name.toLowerCase().includes(q) || t.code.toLowerCase().includes(q),
      )
      .map(summarize);
    return HttpResponse.json(ok(rows, listMeta(rows.length)));
  }),
  http.post(`${API_URL}/library/templates`, async ({ request }) => {
    const body = (await request.json()) as TemplateInput;
    if (store.templates.some((t) => t.code === body.code.toUpperCase())) {
      return HttpResponse.json(fail("CODE_TAKEN", "mã chương trình đã tồn tại"), {
        status: 409,
      });
    }
    const template: ProgramTemplate = {
      id: mintId(),
      code: body.code.toUpperCase(),
      name: body.name,
      subject: body.subject,
      level: body.level,
      description: body.description,
      created_by: OWNER_ID,
      published_version_no: null,
      draft_version_no: 1,
      version_count: 1,
      created_at: NOW,
      updated_at: NOW,
    };
    store.templates.push(template);
    store.versions.push({
      id: mintId(),
      template_id: template.id,
      version_no: 1,
      status: "draft",
      changelog: null,
      published_at: null,
      created_by: OWNER_ID,
      lesson_count: 0,
      created_at: NOW,
      updated_at: NOW,
    });
    return HttpResponse.json(ok(template), { status: 201 });
  }),
  http.get(`${API_URL}/library/templates/:id`, ({ params }) => {
    const template = store.templates.find((t) => t.id === params.id);
    return template ? HttpResponse.json(ok(summarize(template))) : notFound("template");
  }),
  http.put(`${API_URL}/library/templates/:id`, async ({ params, request }) => {
    const template = store.templates.find((t) => t.id === params.id);
    if (!template) return notFound("template");
    const body = (await request.json()) as TemplateInput;
    Object.assign(template, body, { code: body.code.toUpperCase(), updated_at: NOW });
    return HttpResponse.json(ok(summarize(template)));
  }),
  http.delete(`${API_URL}/library/templates/:id`, ({ params }) => {
    const index = store.templates.findIndex((t) => t.id === params.id);
    if (index === -1) return notFound("template");
    store.templates.splice(index, 1);
    return HttpResponse.json(ok({ deleted: true }));
  }),
  http.get(`${API_URL}/library/templates/:id/versions`, ({ params }) => {
    if (!store.templates.some((t) => t.id === params.id)) return notFound("template");
    const rows = store.versions
      .filter((v) => v.template_id === params.id)
      .sort((a, b) => b.version_no - a.version_no)
      .map(withCount);
    return HttpResponse.json(ok(rows));
  }),
  http.post(`${API_URL}/library/templates/:id/versions`, async ({ params, request }) => {
    const template = store.templates.find((t) => t.id === params.id);
    if (!template) return notFound("template");
    const versions = store.versions.filter((v) => v.template_id === template.id);
    if (versions.some((v) => v.status === "draft")) {
      return HttpResponse.json(fail("DRAFT_EXISTS", "chương trình đã có bản nháp"), {
        status: 409,
      });
    }
    const body = (await request.json().catch(() => ({}))) as { changelog?: string | null };
    const version: TemplateVersion = {
      id: mintId(),
      template_id: template.id,
      version_no: Math.max(0, ...versions.map((v) => v.version_no)) + 1,
      status: "draft",
      changelog: body.changelog ?? null,
      published_at: null,
      created_by: OWNER_ID,
      lesson_count: 0,
      created_at: NOW,
      updated_at: NOW,
    };
    store.versions.push(version);
    const source = versions
      .filter((v) => v.status !== "draft")
      .sort((a, b) => b.version_no - a.version_no)[0];
    if (source) {
      for (const row of store.lessons.filter((l) => l.version_id === source.id)) {
        const copy = { ...row, id: mintId(), version_id: version.id };
        store.lessons.push(copy);
        for (const link of store.materialLinks.filter((l) => l.lesson_id === row.id)) {
          store.materialLinks.push({ ...link, lesson_id: copy.id });
        }
        for (const link of store.exerciseLinks.filter((l) => l.lesson_id === row.id)) {
          store.exerciseLinks.push({ ...link, lesson_id: copy.id });
        }
      }
      for (const field of store.logFields.filter((f) => f.version_id === source.id)) {
        store.logFields.push({ ...field, id: mintId(), version_id: version.id });
      }
      store.scoreSets[version.id] = structuredClone(store.scoreSets[source.id] ?? []);
    }
    return HttpResponse.json(ok(withCount(version)), { status: 201 });
  }),
  http.post(`${API_URL}/library/versions/:vid/publish`, ({ params }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    if (version.status !== "draft") {
      return HttpResponse.json(fail("VERSION_NOT_DRAFT", "phiên bản không phải bản nháp"), {
        status: 409,
      });
    }
    version.status = "published";
    version.published_at = NOW;
    return HttpResponse.json(ok(withCount(version)));
  }),
  http.post(`${API_URL}/library/versions/:vid/archive`, ({ params }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    if (version.status !== "published") {
      return HttpResponse.json(fail("VERSION_NOT_PUBLISHED", "phiên bản chưa phát hành"), {
        status: 409,
      });
    }
    version.status = "archived";
    return HttpResponse.json(ok(withCount(version)));
  }),
  http.get(`${API_URL}/library/versions/:vid/lessons`, ({ params }) => {
    if (!store.versions.some((v) => v.id === params.vid)) return notFound("version");
    return HttpResponse.json(ok(renumber(String(params.vid))));
  }),
  http.post(`${API_URL}/library/versions/:vid/lessons`, async ({ params, request }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    if (version.status !== "draft") {
      return HttpResponse.json(fail("VERSION_LOCKED", "phiên bản đã khoá"), { status: 409 });
    }
    const body = (await request.json()) as LessonInput;
    const row: TemplateLesson = {
      id: mintId(),
      version_id: version.id,
      position: store.lessons.filter((l) => l.version_id === version.id).length + 1,
      ...body,
      created_at: NOW,
      updated_at: NOW,
    };
    store.lessons.push(row);
    return HttpResponse.json(ok(row), { status: 201 });
  }),
  http.put(`${API_URL}/library/versions/:vid/lessons/order`, async ({ params, request }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    if (version.status !== "draft") {
      return HttpResponse.json(fail("VERSION_LOCKED", "phiên bản đã khoá"), { status: 409 });
    }
    const body = (await request.json()) as { lesson_ids: string[] };
    const rows = store.lessons.filter((l) => l.version_id === version.id);
    const ids = new Set(rows.map((l) => l.id));
    if (body.lesson_ids.length !== rows.length || body.lesson_ids.some((id) => !ids.has(id))) {
      return HttpResponse.json(
        fail("VALIDATION_ERROR", "danh sách buổi không khớp", {
          lesson_ids: "phải chứa đúng mọi buổi của phiên bản",
        }),
        { status: 422 },
      );
    }
    body.lesson_ids.forEach((id, index) => {
      const row = rows.find((l) => l.id === id);
      if (row) row.position = index + 1;
    });
    return HttpResponse.json(ok(renumber(version.id)));
  }),
  http.get(`${API_URL}/library/lessons/:lid`, ({ params }) => {
    const row = store.lessons.find((l) => l.id === params.lid);
    return row ? HttpResponse.json(ok(lessonDetail(row))) : notFound("lesson");
  }),
  http.put(`${API_URL}/library/lessons/:lid`, async ({ params, request }) => {
    const row = store.lessons.find((l) => l.id === params.lid);
    if (!row) return notFound("lesson");
    const version = store.versions.find((v) => v.id === row.version_id);
    if (version?.status !== "draft") {
      return HttpResponse.json(fail("VERSION_LOCKED", "phiên bản đã khoá"), { status: 409 });
    }
    const body = (await request.json()) as LessonInput;
    Object.assign(row, body, { updated_at: NOW });
    return HttpResponse.json(ok(lessonDetail(row)));
  }),
  http.delete(`${API_URL}/library/lessons/:lid`, ({ params }) => {
    const index = store.lessons.findIndex((l) => l.id === params.lid);
    if (index === -1) return notFound("lesson");
    const [removed] = store.lessons.splice(index, 1);
    if (removed) {
      renumber(removed.version_id);
      store.materialLinks = store.materialLinks.filter((l) => l.lesson_id !== removed.id);
      store.exerciseLinks = store.exerciseLinks.filter((l) => l.lesson_id !== removed.id);
    }
    return HttpResponse.json(ok({ deleted: true }));
  }),
  http.get(`${API_URL}/library/versions/:vid`, ({ params }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    return version ? HttpResponse.json(ok(versionDetail(version))) : notFound("version");
  }),
  http.put(`${API_URL}/library/versions/:vid/log-fields`, async ({ params, request }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    const lock = locked(version);
    if (lock) return lock;
    const body = (await request.json()) as LogFieldInput[];
    const fields: Record<string, string> = {};
    body.forEach((item, index) => {
      if (item.label.trim() === "") fields[`${index}.label`] = "label is required";
      if (item.kind === "select" && item.options.filter((o) => o.trim()).length === 0) {
        fields[`${index}.options`] = "select needs at least one option";
      }
    });
    if (Object.keys(fields).length > 0) return validationError(fields);
    store.logFields = store.logFields.filter((f) => f.version_id !== version.id);
    body.forEach((item, index) => {
      store.logFields.push({
        id: mintId(),
        version_id: version.id,
        position: index + 1,
        label: item.label.trim(),
        kind: item.kind,
        options: item.kind === "select" ? item.options.map((o) => o.trim()).filter(Boolean) : [],
        required: item.required,
      });
    });
    return HttpResponse.json(ok(versionLogFields(version.id)));
  }),
  http.put(`${API_URL}/library/versions/:vid/score-set`, async ({ params, request }) => {
    const version = store.versions.find((v) => v.id === params.vid);
    if (!version) return notFound("version");
    const lock = locked(version);
    if (lock) return lock;
    const body = (await request.json()) as ScoreComponentInput[];
    const fields: Record<string, string> = {};
    const seen = new Set<string>();
    body.forEach((item, index) => {
      if (!/^[a-z0-9_]+$/.test(item.key)) fields[`${index}.key`] = "key is malformed";
      else if (seen.has(item.key)) fields[`${index}.key`] = "key is duplicated";
      seen.add(item.key);
      if (item.label.trim() === "") fields[`${index}.label`] = "label is required";
      if (!(item.max > 0)) fields[`${index}.max`] = "max must be greater than 0";
    });
    if (Object.keys(fields).length > 0) return validationError(fields);
    store.scoreSets[version.id] = body.map((item) => ({
      key: item.key,
      label: item.label.trim(),
      max: item.max,
      weight: item.weight,
    }));
    return HttpResponse.json(ok(store.scoreSets[version.id]));
  }),
  http.put(`${API_URL}/library/lessons/:lid/materials`, async ({ params, request }) => {
    const row = store.lessons.find((l) => l.id === params.lid);
    if (!row) return notFound("lesson");
    const lock = locked(store.versions.find((v) => v.id === row.version_id));
    if (lock) return lock;
    const body = (await request.json()) as LessonMaterialInput[];
    const missing = body.findIndex(
      (item) => !store.materials.some((m) => m.id === item.material_id),
    );
    if (missing !== -1) {
      return validationError({ [`${missing}.material_id`]: "material not found" });
    }
    store.materialLinks = store.materialLinks.filter((l) => l.lesson_id !== row.id);
    body.forEach((item, index) => {
      store.materialLinks.push({
        lesson_id: row.id,
        material_id: item.material_id,
        shared_with_students: item.shared_with_students,
        position: index + 1,
      });
    });
    return HttpResponse.json(ok(lessonMaterials(row.id)));
  }),
  http.put(`${API_URL}/library/lessons/:lid/exercises`, async ({ params, request }) => {
    const row = store.lessons.find((l) => l.id === params.lid);
    if (!row) return notFound("lesson");
    const lock = locked(store.versions.find((v) => v.id === row.version_id));
    if (lock) return lock;
    const body = (await request.json()) as LessonExerciseInput[];
    const missing = body.findIndex(
      (item) => !store.exercises.some((e) => e.id === item.exercise_id),
    );
    if (missing !== -1) {
      return validationError({ [`${missing}.exercise_id`]: "exercise not found" });
    }
    store.exerciseLinks = store.exerciseLinks.filter((l) => l.lesson_id !== row.id);
    body.forEach((item, index) => {
      store.exerciseLinks.push({
        lesson_id: row.id,
        exercise_id: item.exercise_id,
        position: index + 1,
      });
    });
    return HttpResponse.json(ok(lessonExercises(row.id)));
  }),
  http.get(`${API_URL}/library/materials`, ({ request }) => {
    const rows = store.materials
      .filter(itemFilter(request))
      .sort((a, b) => a.title.localeCompare(b.title));
    return HttpResponse.json(ok(rows, listMeta(rows.length)));
  }),
  http.post(`${API_URL}/library/materials`, async ({ request }) => {
    const body = (await request.json()) as MaterialInput;
    const material: Material = { id: mintId(), ...body, created_at: NOW, updated_at: NOW };
    store.materials.push(material);
    return HttpResponse.json(ok(material), { status: 201 });
  }),
  http.get(`${API_URL}/library/materials/:id`, ({ params }) => {
    const material = store.materials.find((m) => m.id === params.id);
    return material ? HttpResponse.json(ok(material)) : notFound("material");
  }),
  http.put(`${API_URL}/library/materials/:id`, async ({ params, request }) => {
    const material = store.materials.find((m) => m.id === params.id);
    if (!material) return notFound("material");
    const body = (await request.json()) as MaterialInput;
    Object.assign(material, body, { updated_at: NOW });
    return HttpResponse.json(ok(material));
  }),
  http.delete(`${API_URL}/library/materials/:id`, ({ params }) => {
    const index = store.materials.findIndex((m) => m.id === params.id);
    if (index === -1) return notFound("material");
    if (store.materialLinks.some((l) => l.material_id === params.id)) {
      return HttpResponse.json(fail("MATERIAL_IN_USE", "học liệu đang được gắn vào buổi học"), {
        status: 409,
      });
    }
    store.materials.splice(index, 1);
    return HttpResponse.json(ok({ deleted: true }));
  }),
  http.get(`${API_URL}/library/exercises`, ({ request }) => {
    const rows = store.exercises
      .filter(itemFilter(request))
      .sort((a, b) => a.title.localeCompare(b.title));
    return HttpResponse.json(ok(rows, listMeta(rows.length)));
  }),
  http.post(`${API_URL}/library/exercises`, async ({ request }) => {
    const body = (await request.json()) as ExerciseInput;
    const exercise: Exercise = { id: mintId(), ...body, created_at: NOW, updated_at: NOW };
    store.exercises.push(exercise);
    return HttpResponse.json(ok(exercise), { status: 201 });
  }),
  http.get(`${API_URL}/library/exercises/:id`, ({ params }) => {
    const exercise = store.exercises.find((e) => e.id === params.id);
    return exercise ? HttpResponse.json(ok(exercise)) : notFound("exercise");
  }),
  http.put(`${API_URL}/library/exercises/:id`, async ({ params, request }) => {
    const exercise = store.exercises.find((e) => e.id === params.id);
    if (!exercise) return notFound("exercise");
    const body = (await request.json()) as ExerciseInput;
    Object.assign(exercise, body, { updated_at: NOW });
    return HttpResponse.json(ok(exercise));
  }),
  http.delete(`${API_URL}/library/exercises/:id`, ({ params }) => {
    const index = store.exercises.findIndex((e) => e.id === params.id);
    if (index === -1) return notFound("exercise");
    if (store.exerciseLinks.some((l) => l.exercise_id === params.id)) {
      return HttpResponse.json(fail("EXERCISE_IN_USE", "bài tập đang được gắn vào buổi học"), {
        status: 409,
      });
    }
    store.exercises.splice(index, 1);
    return HttpResponse.json(ok({ deleted: true }));
  }),
];
