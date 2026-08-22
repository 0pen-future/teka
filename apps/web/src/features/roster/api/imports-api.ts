import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";
import { ApiError } from "@/lib/api/errors";

import {
  importErrorsPayloadSchema,
  importReportSchema,
  type ImportErrorsPayload,
  type ImportReport,
} from "../schemas/import-schemas";

/**
 * Overrides `apiClient`'s 10s default, which a real roster exceeds. It sits
 * deliberately above the server's own 30s WriteTimeout
 * (`apps/api/internal/server/server.go`): the transaction commits or rolls
 * back server-side either way, so giving up first would leave the owner
 * unable to tell whether the data landed — the one failure mode here with no
 * recovery from the UI.
 */
const IMPORT_TIMEOUT_MS = 60_000;

/** Generating and streaming the template is cheap; this only covers a slow link. */
const TEMPLATE_TIMEOUT_MS = 30_000;

/**
 * `GET /imports/roster/template` returns the workbook itself, not an
 * envelope — a binary stream, like the health probes. Failures still arrive
 * as the JSON envelope; the response interceptor reads the blob back to JSON
 * so this call's 403 carries the same code as any other.
 */
export async function downloadTemplate(): Promise<Blob> {
  const res = await apiClient.get<Blob>("/imports/roster/template", {
    responseType: "blob",
    timeout: TEMPLATE_TIMEOUT_MS,
  });
  return res.data;
}

export interface ImportRosterInput {
  file: File;
  /** true = check only. The server also defaults to true when the flag is unreadable. */
  dryRun: boolean;
}

/** `POST /imports/roster`. Owner-only; the server is the authority on that. */
export async function importRoster({ file, dryRun }: ImportRosterInput): Promise<ImportReport> {
  const form = new FormData();
  form.append("file", file);
  form.append("dry_run", String(dryRun));
  // Content-Type is deliberately unset: the browser has to add the multipart
  // boundary, and a hand-set header drops it and yields an opaque 400.
  const res = await apiClient.post<unknown>("/imports/roster", form, {
    timeout: IMPORT_TIMEOUT_MS,
  });
  return parseData(importReportSchema, res.data);
}

/**
 * Pulls the per-row defects out of a rejected import. Returns null for every
 * other failure — a 403, a 409 from a concurrent import, a network drop —
 * so the caller falls back to the envelope's plain message instead of
 * rendering an empty table.
 */
export function importRowErrors(err: unknown): ImportErrorsPayload | null {
  if (!(err instanceof ApiError) || err.status !== 422) {
    return null;
  }
  const parsed = importErrorsPayloadSchema.safeParse(err.details);
  return parsed.success ? parsed.data : null;
}
