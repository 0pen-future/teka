import { http, HttpResponse } from "msw";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type { CenterMe, CenterMember } from "../schemas/center-schemas";

let memberCounter = 0;

export function makeMember(overrides: Partial<CenterMember> = {}): CenterMember {
  memberCounter += 1;
  return {
    id: `20000000-0000-4000-8000-${String(memberCounter).padStart(12, "0")}`,
    full_name: `Giáo Viên ${memberCounter}`,
    phone: `+8490200${String(memberCounter).padStart(4, "0")}`,
    is_owner: false,
    ...overrides,
  };
}

export function makeCenterMe(overrides: Partial<CenterMe> = {}): CenterMe {
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

/** Overrides `POST /centers/join` with a scripted error envelope. */
export function mockJoinFailure(
  status: number,
  code: string,
  message: string,
  fields?: Record<string, string>,
) {
  server.use(
    http.post(`${API_URL}/centers/join`, () =>
      HttpResponse.json(fail(code, message, fields), { status }),
    ),
  );
}
