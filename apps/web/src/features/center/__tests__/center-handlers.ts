import { http, HttpResponse } from "msw";

import { ALL_PERMISSION_KEYS, API_URL, DEFAULT_CENTER_PERMISSIONS, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type {
  CenterMe,
  CenterMeMember,
  CenterMeOwner,
  CenterMember,
} from "../schemas/center-schemas";
import type { CenterPermissions, MemberPermissions } from "../schemas/permission-schemas";

let memberCounter = 0;

export function makeMember(overrides: Partial<CenterMember> = {}): CenterMember {
  memberCounter += 1;
  return {
    id: `20000000-0000-4000-8000-${String(memberCounter).padStart(12, "0")}`,
    full_name: `Giáo Viên ${memberCounter}`,
    phone: `+8490200${String(memberCounter).padStart(4, "0")}`,
    is_owner: false,
    can_send_reports: false,
    ...overrides,
  };
}

/** `centers.MeResponse` — the owner's read model. */
export function makeCenterMeOwner(overrides: Partial<CenterMeOwner> = {}): CenterMeOwner {
  return {
    center: {
      id: "30000000-0000-4000-8000-000000000001",
      name: "Trung Tâm Bình Minh",
      is_owner: true,
      ...overrides.center,
    },
    members: overrides.members ?? [makeMember({ is_owner: true })],
    // The server folds the owner bypass into the effective array.
    permissions: overrides.permissions ?? ALL_PERMISSION_KEYS,
  };
}

/** `centers.MemberMeResponse` — a non-owner member's read model. */
export function makeCenterMeMember(overrides: Partial<CenterMeMember> = {}): CenterMeMember {
  return {
    center_name: "Trung Tâm Bình Minh",
    can_send_reports: false,
    permissions: [],
    ...overrides,
  };
}

/**
 * Scripts `GET /centers/me`: each call answers the next payload, repeating
 * the last one — so a test can hand the pre-action roster then the
 * post-refetch one. Returns a live call counter for refetch assertions.
 */
export function mockCenterMe(...payloads: CenterMe[]): { calls: number } {
  const record = { calls: 0 };
  server.use(
    http.get(`${API_URL}/centers/me`, () => {
      const current = Math.min(record.calls, payloads.length - 1);
      record.calls += 1;
      return HttpResponse.json(ok(payloads[current]));
    }),
  );
  return record;
}

/** One member's RBAC row for the permissions read model (no role, no overrides). */
export function makeMemberPermissions(
  member: CenterMember,
  overrides: Partial<MemberPermissions> = {},
): MemberPermissions {
  return {
    teacher_id: member.id,
    full_name: member.full_name,
    role_id: null,
    role_key: "",
    grants: [],
    denies: [],
    assignment_version: 1,
    ...overrides,
  };
}

/** The default read model (3 empty roles) with the given member rows. */
export function makeCenterPermissions(
  overrides: Partial<CenterPermissions> = {},
): CenterPermissions {
  return { ...DEFAULT_CENTER_PERMISSIONS, ...overrides };
}

/**
 * Scripts `GET /centers/me/permissions` like `mockCenterMe`: each call
 * answers the next payload, repeating the last one, so a test can hand the
 * pre-mutation model and then the post-refetch one.
 */
export function mockCenterPermissions(...payloads: CenterPermissions[]): { calls: number } {
  const record = { calls: 0 };
  server.use(
    http.get(`${API_URL}/centers/me/permissions`, () => {
      const current = Math.min(record.calls, payloads.length - 1);
      record.calls += 1;
      return HttpResponse.json(ok(payloads[current]));
    }),
  );
  return record;
}
