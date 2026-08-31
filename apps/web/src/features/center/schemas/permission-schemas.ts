import { z } from "zod";

/**
 * `centers.PermissionInfo` — one catalog entry. Labels are the API's
 * Vietnamese display names (single source in `authctx/catalog.go`); the UI
 * never keeps its own key→label map. The structured fields default so an
 * older API (rollback window) still parses; an unknown risk from a newer API
 * falls back to "high" — over-warning is the safe direction for a value that
 * only drives confirmation pressure.
 */
export const permissionInfoSchema = z.object({
  key: z.string(),
  label: z.string(),
  resource: z.string().default(""),
  action: z.string().default(""),
  kind: z.enum(["crud", "scope", "special"]).catch("crud").default("crud"),
  risk: z.enum(["low", "medium", "high"]).catch("high").default("low"),
  description: z.string().default(""),
});

export type PermissionInfo = z.infer<typeof permissionInfoSchema>;

/**
 * `centers.RoleResponse` — a center role with its current permission set.
 * `assignment_version` defaults to 0, which the API treats as "skip the CAS
 * check" — exactly right when a rolled-back API stops sending it.
 */
export const roleSchema = z.object({
  id: z.string(),
  key: z.string(),
  name: z.string(),
  permissions: z.array(z.string()),
  assignment_version: z.number().default(0),
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
  assignment_version: z.number().default(0),
});

export type MemberPermissions = z.infer<typeof memberPermissionsSchema>;

/** `centers.PermissionsResponse` — body of `GET /centers/me/permissions`. */
export const centerPermissionsSchema = z.object({
  catalog: z.array(permissionInfoSchema),
  roles: z.array(roleSchema),
  members: z.array(memberPermissionsSchema),
  catalog_version: z.number().default(0),
});

export type CenterPermissions = z.infer<typeof centerPermissionsSchema>;

/**
 * Vietnamese group headings per catalog resource — display-only grouping aid
 * (per-key labels still come from the API). An unmapped resource falls back
 * to its raw key so a newer API's new resource still renders.
 */
const RESOURCE_LABELS: Record<string, string> = {
  classes: "Lớp học",
  schedules: "Lịch học",
  contacts: "Liên hệ",
  students: "Học viên",
  enrollments: "Ghi danh",
  sessions: "Buổi học",
  attendance: "Điểm danh",
  scores: "Điểm số",
  teaching: "Giảng dạy",
  billing: "Học phí",
  payments: "Thanh toán",
  statements: "Sao kê",
  notifications: "Thông báo",
  reports: "Báo cáo",
  members: "Thành viên",
  center: "Trung tâm",
  invitations: "Lời mời",
  audit: "Nhật ký",
  imports: "Import",
  dashboard: "Dashboard",
  khac: "Khác",
};

export interface CatalogGroup {
  resource: string;
  label: string;
  entries: PermissionInfo[];
}

/**
 * Groups the catalog by resource, preserving the API's registry order both
 * across groups and inside each one. Entries without a resource (older API)
 * collapse into one unlabeled group so nothing disappears.
 */
export function groupCatalog(catalog: PermissionInfo[]): CatalogGroup[] {
  const groups: CatalogGroup[] = [];
  const byResource = new Map<string, CatalogGroup>();
  for (const entry of catalog) {
    const resource = entry.resource || "khac";
    let group = byResource.get(resource);
    if (!group) {
      group = { resource, label: RESOURCE_LABELS[resource] ?? resource, entries: [] };
      byResource.set(resource, group);
      groups.push(group);
    }
    group.entries.push(entry);
  }
  return groups;
}
