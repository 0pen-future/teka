import { http, HttpResponse } from "msw";

import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";

import type { ZaloLinkStatus, ZaloStatus } from "../schemas/zalo-schemas";

export const testLinkId = "7f000000-0000-4000-8000-000000000001";

/** A one-pixel PNG — real base64, so the data URI in the test is a real one. */
export const testQrPng =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==";

/** Overrides `GET /me/zalo` (the shared default answers "not linked"). */
export function mockZaloStatus(status: ZaloStatus) {
  server.use(http.get(`${API_URL}/me/zalo`, () => HttpResponse.json(ok(status))));
}

export interface ZaloLinkCalls {
  /** How many times `link/start` was called. */
  start: number;
  /** How many times `link/status` was polled. */
  polls: number;
  /** The `consent_version` of every `link/start` body, in order. */
  consentVersions: string[];
  /** The `id` query param of every poll, in order. */
  polledIds: string[];
}

/**
 * Scripts a link attempt: `link/start` hands back {@link testLinkId} and each
 * poll of `link/status` returns the next scripted state, repeating the last one
 * once the script runs out. Starting again rewinds the script, so a retry gets
 * a genuinely fresh attempt rather than replaying the outcome that ended the
 * previous one. Returns a live call record so a test can assert both what was
 * sent and that polling stopped.
 */
export function mockZaloLink(states: ZaloLinkStatus[]): ZaloLinkCalls {
  const calls: ZaloLinkCalls = { start: 0, polls: 0, consentVersions: [], polledIds: [] };
  let step = 0;
  server.use(
    http.post(`${API_URL}/me/zalo/link/start`, async ({ request }) => {
      const body = (await request.json()) as { consent_version?: string };
      calls.start += 1;
      calls.consentVersions.push(body.consent_version ?? "");
      step = 0;
      return HttpResponse.json(ok({ link_id: testLinkId }), { status: 202 });
    }),
    http.get(`${API_URL}/me/zalo/link/status`, ({ request }) => {
      const current = Math.min(step, states.length - 1);
      step += 1;
      calls.polls += 1;
      calls.polledIds.push(new URL(request.url).searchParams.get("id") ?? "");
      return HttpResponse.json(ok(states[current]));
    }),
  );
  return calls;
}
