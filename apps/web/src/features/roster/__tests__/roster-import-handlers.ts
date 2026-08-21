import { http, HttpResponse, type HttpResponseResolver } from "msw";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type { ImportReport, ImportRowError } from "../schemas/import-schemas";

/** A report with nothing found and nothing to do; overrides fill in the run. */
export function makeImportReport(overrides: Partial<ImportReport> = {}): ImportReport {
  const empty = { created: 0, reused: 0 };
  return {
    committed: false,
    classes: empty,
    schedules: empty,
    contacts: empty,
    students: empty,
    enrollments: empty,
    ...overrides,
  };
}

export function makeRowError(overrides: Partial<ImportRowError> = {}): ImportRowError {
  return {
    sheet: "Lop",
    line: 3,
    column: "Tên lớp",
    code: "TOO_LONG",
    message: "tên lớp tối đa 100 ký tự, ô này có 101",
    ...overrides,
  };
}

/** The 422 body: row defects ride in `details`, not in `fields`. */
export function rowErrorsResponse(errors: ImportRowError[], truncated?: number) {
  return HttpResponse.json(
    fail("VALIDATION_ERROR", "file có dòng không hợp lệ", undefined, {
      errors,
      ...(truncated === undefined ? {} : { truncated }),
    }),
    { status: 422 },
  );
}

/** What the page actually put on the wire for one import request. */
export interface ImportCall {
  dryRun: boolean;
  /** Whether a `file` part was present. jsdom drops the name, so only presence is observable. */
  hasFile: boolean;
}

/**
 * Scripts `POST /imports/roster`, recording each call so a test can assert
 * the flag the page sent — the difference between a check and a commit is a
 * single form field, and getting it backwards would write data during a
 * check.
 */
export function mockRosterImport(
  respond: (call: ImportCall, index: number) => Response | Promise<Response>,
): ImportCall[] {
  const calls: ImportCall[] = [];
  server.use(
    http.post(`${API_URL}/imports/roster`, async ({ request }) => {
      // Read the multipart body as text rather than through
      // `request.formData()`: jsdom's File is not undici's, and the parser
      // rejects it. The raw body is also the stricter assertion — it proves
      // the browser set a boundary and the fields went out as parts.
      const body = await request.text();
      const call: ImportCall = {
        dryRun: /name="dry_run"\r?\n\r?\n(true|false)/.exec(body)?.[1] === "true",
        hasFile: body.includes('name="file"; filename='),
      };
      calls.push(call);
      return respond(call, calls.length - 1);
    }),
  );
  return calls;
}

/** Answers a check with `report` and the commit with the same counts, committed. */
export function mockImportHappyPath(report: ImportReport): ImportCall[] {
  return mockRosterImport((call) => HttpResponse.json(ok({ ...report, committed: !call.dryRun })));
}

/**
 * Scripts `GET /imports/roster/template`. The route answers a binary stream
 * outside the envelope, so a failure has to be given explicitly.
 */
export function mockImportTemplate(resolver?: HttpResponseResolver): void {
  server.use(
    http.get(
      `${API_URL}/imports/roster/template`,
      resolver ??
        (() =>
          HttpResponse.arrayBuffer(new TextEncoder().encode("PKfake-xlsx").buffer, {
            headers: {
              "Content-Type": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
              "Content-Disposition": 'attachment; filename="teka-import-mau.xlsx"',
            },
          })),
    ),
  );
}
