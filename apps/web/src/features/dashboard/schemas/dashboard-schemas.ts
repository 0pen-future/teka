import { z } from "zod";

/**
 * `sessions.PendingSessionResponse`
 * (`apps/api/internal/features/sessions/dto.go`) — held sessions whose
 * attendance is not yet confirmed. The id field is `session_id`, not `id`.
 */
export const pendingSessionSchema = z.object({
  session_id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  session_date: z.string(),
  start_time: z.string().nullable(),
  status: z.string(),
  expected_student_count: z.number().int(),
  days_overdue: z.number().int(),
});

/** `sessions.PendingResponse` — the endpoint returns `{total, items}`, not a bare array. */
export const pendingSessionsResponseSchema = z.object({
  total: z.number().int(),
  items: z.array(pendingSessionSchema),
});

export type PendingSession = z.infer<typeof pendingSessionSchema>;
export type PendingSessionsResponse = z.infer<typeof pendingSessionsResponseSchema>;
