import { z } from "zod";

/**
 * `centers.PermissionInfo` — one catalog entry. Labels are the API's
 * Vietnamese display names (single source in `authctx/permissions.go`); the
 * UI never keeps its own key→label map.
 */
export const permissionInfoSchema = z.object({
  key: z.string(),
  label: z.string(),
});

export type PermissionInfo = z.infer<typeof permissionInfoSchema>;

/** `centers.RoleResponse` — a center role with its current permission set. */
export const roleSchema = z.object({
  id: z.string(),
  key: z.string(),
  name: z.string(),
  permissions: z.array(z.string()),
});

export type Role = z.infer<typeof roleSchema>;

/**
 * `centers.MemberPermissionsResponse` — one non-owner member's RBAC state.
 * `role_id`/`role_key` are null/"" for a stint holding no role (the API
 * reports it as-is; the UI renders the default "Giáo viên" badge).
 */
export const memberPermissionsSchema = z.object({
  teacher_id: z.string(),
  full_name: z.string(),
  role_id: z.string().nullable(),
  role_key: z.string(),
  grants: z.array(z.string()),
  denies: z.array(z.string()),
});

export type MemberPermissions = z.infer<typeof memberPermissionsSchema>;

/** `centers.PermissionsResponse` — body of `GET /centers/me/permissions`. */
export const centerPermissionsSchema = z.object({
  catalog: z.array(permissionInfoSchema),
  roles: z.array(roleSchema),
  members: z.array(memberPermissionsSchema),
});

export type CenterPermissions = z.infer<typeof centerPermissionsSchema>;

/**
 * The dual-life restriction (API phase 2): `reports.send` is assignable only
 * per member while the legacy `can_send_reports` column is authoritative, so
 * the role matrix disables that cell. Lifted when the column drops.
 */
export const REPORTS_SEND_KEY = "reports.send";
