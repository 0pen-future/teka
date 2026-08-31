import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";
import { ApiError } from "@/lib/api/errors";

import {
  centerMeOwnerSchema,
  centerMeSchema,
  type CenterMe,
  type CenterMeOwner,
  type RenameCenterInput,
} from "../schemas/center-schemas";

export async function getCenterMe(): Promise<CenterMe> {
  const res = await apiClient.get<unknown>("/centers/me");
  return parseData(centerMeSchema, res.data);
}

/**
 * Rename is owner-only, so it always answers the full owner-shaped
 * `MeResponse` — parsing that shape directly (not the role-shaped union)
 * lets the cache be replaced outright instead of refetched.
 */
export async function renameCenter(input: RenameCenterInput): Promise<CenterMeOwner> {
  const res = await apiClient.patch<unknown>("/centers/me", input);
  return parseData(centerMeOwnerSchema, res.data);
}

/**
 * Removing a membership is idempotent from the caller's point of view: a 404
 * means the member is already gone — the state the user asked for — so it
 * converges instead of surfacing an error.
 */
export async function removeMember(teacherId: string): Promise<void> {
  try {
    await apiClient.delete(`/centers/me/members/${teacherId}`);
  } catch (error) {
    if (error instanceof ApiError && error.code === "NOT_FOUND") {
      return;
    }
    throw error;
  }
}
