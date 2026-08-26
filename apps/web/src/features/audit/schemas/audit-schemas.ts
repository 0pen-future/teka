import { z } from "zod";

/**
 * Wire shape of one audit row, mirroring `LogResponse` in
 * `apps/api/internal/features/audit/dto.go`. `actor_user_id` is null for
 * actor-less rows; an empty `actor_name` means the teacher record no longer
 * exists (LEFT JOIN miss) and renders as "(đã xóa)".
 */
export const auditLogSchema = z.object({
  id: z.string(),
  occurred_at: z.string(),
  actor_user_id: z.string().nullable(),
  actor_name: z.string(),
  actor_role: z.string(),
  action: z.string(),
  method: z.string(),
  path: z.string(),
  entity_type: z.string(),
  entity_id: z.string(),
  status_code: z.number().int(),
  ip: z.string(),
  user_agent: z.string(),
  metadata: z.record(z.string(), z.string()).nullable(),
});

/** `next_cursor` is an opaque token; empty string means last page. */
export const auditLogPageSchema = z.object({
  items: z.array(auditLogSchema),
  next_cursor: z.string(),
});

export type AuditLog = z.infer<typeof auditLogSchema>;
export type AuditLogPage = z.infer<typeof auditLogPageSchema>;

/**
 * Server-side filters for GET /audit-logs. A cursor is only valid together
 * with the filters that produced it, so changing any field must restart
 * pagination from the first page.
 */
export interface AuditLogFilters {
  actor_id?: string;
  /** Prefix match, e.g. "auth." or a full action name. */
  action?: string;
  /** RFC3339 instant, inclusive. */
  from?: string;
  /** RFC3339 instant, inclusive. */
  to?: string;
}
