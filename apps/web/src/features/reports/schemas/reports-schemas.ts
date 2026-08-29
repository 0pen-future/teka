import { z } from "zod";

/**
 * `billing.PeriodResponse` (`apps/api/internal/features/billing/dto.go`) as
 * the center-wide list endpoint returns it: `teacher_id`/`teacher_name`
 * identify the period's owning teacher so this feature can group by teacher.
 * `teacher_name` is omitempty on the wire (only the read endpoints' joined
 * rows populate it), so it stays optional here. Duplicated from
 * `features/billing` rather than imported — features must not couple through
 * source imports (see `features/collections/schemas/collections-schemas.ts`).
 */
export const reportPeriodSchema = z.object({
  id: z.string(),
  teacher_id: z.string(),
  teacher_name: z.string().optional(),
  year: z.number().int(),
  month: z.number().int(),
  period_start: z.string(),
  period_end: z.string(),
  status: z.enum(["open", "closed"]),
  closed_at: z.string().nullable(),
});

export type ReportPeriod = z.infer<typeof reportPeriodSchema>;
