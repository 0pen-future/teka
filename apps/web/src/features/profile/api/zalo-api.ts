import { z } from "zod";

import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  zaloFriendMatchSchema,
  zaloFriendSchema,
  zaloLinkStartSchema,
  zaloLinkStatusSchema,
  zaloStatusSchema,
  type ZaloFriend,
  type ZaloFriendMatch,
  type ZaloLinkStart,
  type ZaloLinkStatus,
  type ZaloStatus,
} from "../schemas/zalo-schemas";

/** Server-side cap on one match request (`zalo.MaxMatchPhones`). */
export const ZALO_MATCH_MAX_PHONES = 200;

/**
 * Phones per HTTP request. The server resolves phones against Zalo in paced
 * chunks of 30 with a 1–3s wait between chunks, so a single request carrying
 * the full {@link ZALO_MATCH_MAX_PHONES} cannot answer inside either the
 * client's 10s default timeout or the server's 30s write timeout. 100 phones
 * is ~4 chunks (~3–9s of pacing), which fits the raised budget below.
 */
export const ZALO_MATCH_REQUEST_SIZE = 100;

/** Per-request budget for one paced match call; the client default (10s) is too short. */
const ZALO_MATCH_TIMEOUT_MS = 30_000;

/** Whether this teacher has a linked Zalo account, and whether it still works. */
export async function getZaloStatus(): Promise<ZaloStatus> {
  const res = await apiClient.get<unknown>("/me/zalo");
  return parseData(zaloStatusSchema, res.data);
}

/**
 * The linked account's friend list — a live Zalo call on every request, not a
 * cached table, so callers must throttle with `staleTime` (see `useZaloFriends`).
 */
export async function getZaloFriends(): Promise<ZaloFriend[]> {
  const res = await apiClient.get<unknown>("/me/zalo/friends");
  return parseData(z.array(zaloFriendSchema), res.data);
}

/**
 * Resolves phones against Zalo and labels each row against the friend list —
 * a live, paced Zalo lookup (up to {@link ZALO_MATCH_MAX_PHONES} phones), so
 * callers fire it once per explicit user action, never on render.
 */
export async function matchZaloFriends(
  phones: string[],
  signal?: AbortSignal,
): Promise<ZaloFriendMatch[]> {
  const rows: ZaloFriendMatch[] = [];
  for (let start = 0; start < phones.length; start += ZALO_MATCH_REQUEST_SIZE) {
    const res = await apiClient.post<unknown>(
      "/me/zalo/friends/match",
      { phones: phones.slice(start, start + ZALO_MATCH_REQUEST_SIZE) },
      { signal, timeout: ZALO_MATCH_TIMEOUT_MS },
    );
    rows.push(...parseData(z.array(zaloFriendMatchSchema), res.data));
  }
  return rows;
}

/**
 * Sends one friend request. The endpoint takes exactly one user per call —
 * the contract itself is the rate limit; the server fills in the greeting.
 */
export async function sendZaloFriendRequest(userId: string): Promise<void> {
  await apiClient.post("/me/zalo/friends/request", { user_id: userId });
}

/** Begins a QR attempt, recording the consent version the teacher acknowledged. */
export async function startZaloLink(consentVersion: string): Promise<ZaloLinkStart> {
  const res = await apiClient.post<unknown>("/me/zalo/link/start", {
    consent_version: consentVersion,
  });
  return parseData(zaloLinkStartSchema, res.data);
}

/** One poll of an in-flight attempt. */
export async function getZaloLinkStatus(linkId: string): Promise<ZaloLinkStatus> {
  const res = await apiClient.get<unknown>("/me/zalo/link/status", { params: { id: linkId } });
  return parseData(zaloLinkStatusSchema, res.data);
}

/** Drops the stored session. Answers 204, so there is no body to parse. */
export async function unlinkZalo(): Promise<void> {
  await apiClient.delete("/me/zalo");
}
