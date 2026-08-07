import { z } from "zod";

import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  zaloFriendSchema,
  zaloLinkStartSchema,
  zaloLinkStatusSchema,
  zaloStatusSchema,
  type ZaloFriend,
  type ZaloLinkStart,
  type ZaloLinkStatus,
  type ZaloStatus,
} from "../schemas/zalo-schemas";

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
