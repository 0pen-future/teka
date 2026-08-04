import { z } from "zod";

/**
 * `billing.PeriodResponse` (`apps/api/internal/features/billing/dto.go`).
 * `status` is `"open" | "closed"` (`docs/schema_design.sql:263`).
 */
export const periodSchema = z.object({
  id: z.string(),
  year: z.number().int(),
  month: z.number().int(),
  period_start: z.string(),
  period_end: z.string(),
  status: z.enum(["open", "closed"]),
  closed_at: z.string().nullable(),
});

export type Period = z.infer<typeof periodSchema>;
