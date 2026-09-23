import { http, HttpResponse } from "msw";

import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";

import type {
  LessonInput,
  ProgramTemplate,
  TemplateInput,
  TemplateLesson,
  TemplateVersion,
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

interface LibraryStore {
  templates: ProgramTemplate[];
  versions: TemplateVersion[];
  lessons: TemplateLesson[];
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
        store.lessons.push({ ...row, id: mintId(), version_id: version.id });
      }
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
    return row ? HttpResponse.json(ok(row)) : notFound("lesson");
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
    return HttpResponse.json(ok(row));
  }),
  http.delete(`${API_URL}/library/lessons/:lid`, ({ params }) => {
    const index = store.lessons.findIndex((l) => l.id === params.lid);
    if (index === -1) return notFound("lesson");
    const [removed] = store.lessons.splice(index, 1);
    if (removed) renumber(removed.version_id);
    return HttpResponse.json(ok({ deleted: true }));
  }),
];
