import { http, HttpResponse } from "msw";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type {
  CreateInviteResponse,
  Invitation,
  InvitePreview,
} from "../schemas/invitation-schemas";

let inviteCounter = 0;

export function makeInvite(overrides: Partial<Invitation> = {}): Invitation {
  inviteCounter += 1;
  return {
    id: `40000000-0000-4000-8000-${String(inviteCounter).padStart(12, "0")}`,
    phone: `+8490300${String(inviteCounter).padStart(4, "0")}`,
    status: "pending",
    expires_at: "2026-08-19T10:00:00Z",
    created_at: "2026-08-12T10:00:00Z",
    ...overrides,
  };
}

export function makeCreateInviteResponse(
  overrides: Partial<CreateInviteResponse> = {},
): CreateInviteResponse {
  return {
    id: "40000000-0000-4000-8000-000000000001",
    phone: "+84901234567",
    expires_at: "2026-08-19T10:00:00Z",
    link: "https://app.teka.dev/invite/test-invite-token",
    dm_status: "sent",
    ...overrides,
  };
}

/**
 * Scripts `GET /centers/me/invitations`: each call answers the next payload,
 * repeating the last one — so a test can hand the pre-action list then the
 * post-refetch one. Returns a live call counter for refetch assertions.
 */
export function mockInvites(...payloads: Invitation[][]): { calls: number } {
  const record = { calls: 0 };
  server.use(
    http.get(`${API_URL}/centers/me/invitations`, () => {
      const current = Math.min(record.calls, payloads.length - 1);
      record.calls += 1;
      return HttpResponse.json(ok(payloads[current]));
    }),
  );
  return record;
}

/** Overrides `POST /centers/me/invitations`, capturing the request body. */
export function mockCreateInvite(response: CreateInviteResponse): {
  received: { phone?: string };
} {
  const captured: { received: { phone?: string } } = { received: {} };
  server.use(
    http.post(`${API_URL}/centers/me/invitations`, async ({ request }) => {
      captured.received = (await request.json()) as { phone?: string };
      return HttpResponse.json(ok(response), { status: 201 });
    }),
  );
  return captured;
}

/** Overrides `POST /centers/me/invitations` with a scripted error envelope. */
export function mockCreateInviteFailure(status: number, code: string, message: string) {
  server.use(
    http.post(`${API_URL}/centers/me/invitations`, () =>
      HttpResponse.json(fail(code, message), { status }),
    ),
  );
}

/** Overrides `DELETE /centers/me/invitations/:id`, capturing the revoked id. */
export function mockRevokeInvite(status = 204): { revokedId: string } {
  const captured = { revokedId: "" };
  server.use(
    http.delete(`${API_URL}/centers/me/invitations/:id`, ({ params }) => {
      captured.revokedId = String(params.id);
      return status === 204
        ? new HttpResponse(null, { status: 204 })
        : HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status });
    }),
  );
  return captured;
}

/**
 * Overrides `POST /invitations/preview` for exactly one token; every other
 * token (or a null `preview`) answers the same generic 404, matching the
 * API's anti-enumeration contract.
 */
export function mockInvitePreview(token: string, preview: InvitePreview | null) {
  server.use(
    http.post(`${API_URL}/invitations/preview`, async ({ request }) => {
      const body = (await request.json()) as { token?: string };
      if (body.token !== token || !preview) {
        return HttpResponse.json(fail("NOT_FOUND", "invitation not found"), { status: 404 });
      }
      return HttpResponse.json(ok(preview));
    }),
  );
}

/**
 * Overrides `POST /invitations/accept`, capturing the request body; a
 * `succeed: false` script answers the same generic 400 the real API sends
 * for every rejection reason.
 */
export function mockAcceptInvite(succeed: boolean): { received: Record<string, unknown> } {
  const captured: { received: Record<string, unknown> } = { received: {} };
  server.use(
    http.post(`${API_URL}/invitations/accept`, async ({ request }) => {
      captured.received = (await request.json()) as Record<string, unknown>;
      if (!succeed) {
        return HttpResponse.json(fail("BAD_REQUEST", "invalid or expired invitation"), {
          status: 400,
        });
      }
      return new HttpResponse(null, { status: 204 });
    }),
  );
  return captured;
}
