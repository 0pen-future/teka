import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";
import { ApiError } from "@/lib/api/errors";

import {
  createInviteResponseSchema,
  invitationSchema,
  invitePreviewSchema,
  type AcceptInviteInput,
  type CreateInviteInput,
  type CreateInviteResponse,
  type Invitation,
  type InvitePreview,
} from "../schemas/invitation-schemas";

export async function createInvite(input: CreateInviteInput): Promise<CreateInviteResponse> {
  const res = await apiClient.post<unknown>("/centers/me/invitations", input);
  return parseData(createInviteResponseSchema, res.data);
}

/** `GET /centers/me/invitations` answers a plain array, unpaginated — pending first. */
export async function listInvites(): Promise<Invitation[]> {
  const res = await apiClient.get<unknown>("/centers/me/invitations");
  return parseArray(invitationSchema, res.data);
}

/**
 * Revoking is idempotent from the caller's point of view: a 404 means the
 * invite is already gone (revoked, expired and swept, or accepted) — the
 * state the user asked for — so it converges instead of surfacing an error.
 */
export async function revokeInvite(id: string): Promise<void> {
  try {
    await apiClient.delete(`/centers/me/invitations/${id}`);
  } catch (error) {
    if (error instanceof ApiError && error.code === "NOT_FOUND") {
      return;
    }
    throw error;
  }
}

/**
 * The token travels in the body, not the URL — mirroring the API's
 * anti-enumeration contract (`invitations.PreviewRequest`). Any rejection
 * reason (unknown, revoked, expired, already-accepted token) answers the
 * same generic 404; the caller renders one neutral error regardless.
 */
export async function previewInvite(token: string): Promise<InvitePreview> {
  const res = await apiClient.post<unknown>("/invitations/preview", { token });
  return parseData(invitePreviewSchema, res.data);
}

/**
 * Accepting is a 204 with no body — every rejection reason collapses to the
 * same generic error server-side (`invitations.AcceptRequest`), so there is
 * nothing more specific for the caller to parse out of a failure.
 */
export async function acceptInvite(input: AcceptInviteInput): Promise<void> {
  await apiClient.post("/invitations/accept", input);
}
