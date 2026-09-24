import type { AuditLog } from "../schemas/audit-schemas";

/**
 * Empty actor_name with a non-null actor id means the teacher row is gone
 * (LEFT JOIN miss server-side); a null actor id means the event carried no
 * actor at all.
 */
export function actorLabel(log: AuditLog): string {
  if (log.actor_user_id === null) {
    return "Ẩn danh";
  }
  return log.actor_name === "" ? "(đã xóa)" : log.actor_name;
}
