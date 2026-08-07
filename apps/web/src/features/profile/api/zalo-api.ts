import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  zaloLinkStartSchema,
  zaloLinkStatusSchema,
  zaloStatusSchema,
  type ZaloLinkStart,
  type ZaloLinkStatus,
  type ZaloStatus,
} from "../schemas/zalo-schemas";

/** Whether this teacher has a linked Zalo account, and whether it still works. */
export async function getZaloStatus(): Promise<ZaloStatus> {
  const res = await apiClient.get<unknown>("/me/zalo");
  return parseData(zaloStatusSchema, res.data);
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
