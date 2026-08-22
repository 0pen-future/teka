import { z } from "zod";

/**
 * `imports.ReportEntity` (`apps/api/internal/features/imports/dto.go`). The
 * created/reused split is the only signal the operator has that a re-import
 * was a no-op, so it is reported per entity rather than as one total.
 */
export const importReportEntitySchema = z.object({
  created: z.number().int(),
  reused: z.number().int(),
});

export type ImportReportEntity = z.infer<typeof importReportEntitySchema>;

/**
 * `imports.Report`. `committed` is false for a check (`dry_run=true`); the
 * counts are identical either way, because the check walks the same
 * resolution and the same existence lookups as the real pass.
 */
export const importReportSchema = z.object({
  committed: z.boolean(),
  classes: importReportEntitySchema,
  schedules: importReportEntitySchema,
  contacts: importReportEntitySchema,
  students: importReportEntitySchema,
  enrollments: importReportEntitySchema,
});

export type ImportReport = z.infer<typeof importReportSchema>;

/**
 * `imports.RowError`. `line` is the worksheet's own 1-based row number so the
 * operator reads it straight off the Excel gutter. `column` is `omitempty`
 * server-side — a whole-row defect points at no single cell.
 */
export const importRowErrorSchema = z.object({
  sheet: z.string(),
  line: z.number().int(),
  column: z.string().optional(),
  code: z.string(),
  message: z.string(),
});

export type ImportRowError = z.infer<typeof importRowErrorSchema>;

/**
 * `imports.ErrorsPayload` — the `details` half of a 422. The whole list comes
 * back at once so a workbook is fixed in one pass; `truncated` counts the
 * defects omitted when the list was capped.
 */
export const importErrorsPayloadSchema = z.object({
  errors: z.array(importRowErrorSchema),
  truncated: z.number().int().optional(),
});

export type ImportErrorsPayload = z.infer<typeof importErrorsPayloadSchema>;
