import { http, HttpResponse } from "msw";

import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type {
  CenterMe,
  CenterMeMember,
  CenterMeOwner,
  CenterMember,
} from "../schemas/center-schemas";

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
  };
}

/** `centers.MemberMeResponse` — a non-owner member's read model. */
export function makeCenterMeMember(overrides: Partial<CenterMeMember> = {}): CenterMeMember {
  return {
    center_name: "Trung Tâm Bình Minh",
    can_send_reports: false,
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
