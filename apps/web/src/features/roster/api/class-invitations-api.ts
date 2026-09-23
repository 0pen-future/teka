import { apiClient } from "@/lib/api/client";
import { parseArray, parseData } from "@/lib/api/envelope";

import {
  classInvitationConfirmSchema,
  classInvitationSchema,
  type ClassInvitation,
  type ClassInvitationConfirm,
  type ClassInvitationSendInput,
  type ClassInvitationStatus,
} from "../schemas/roster-schemas";

export interface ListClassInvitationsParams {
  status?: ClassInvitationStatus;
  class_id?: string;
}

/**
 * `GET /class-invitations` (`apps/api/internal/features/classinvites/handler.go`).
 * The owner sees every invitation in the center; a member only those
 * addressed to them — the API narrows, the client never has to. No `meta`.
 */
export async function listClassInvitations(
  params: ListClassInvitationsParams = {},
): Promise<ClassInvitation[]> {
  const res = await apiClient.get<unknown>("/class-invitations", { params });
  return parseArray(classInvitationSchema, res.data);
}

/** `POST /classes/:id/invitations` — owner-only; self-invite is 422 `SELF_INVITE`, a second pending 409. */
export async function sendClassInvitation(
  classId: string,
  input: ClassInvitationSendInput,
): Promise<ClassInvitation> {
  const res = await apiClient.post<unknown>(`/classes/${classId}/invitations`, input);
  return parseData(classInvitationSchema, res.data);
}

/** `POST /class-invitations/:id/accept` — invitee only, pending only. Writes no stint. */
export async function acceptClassInvitation(id: string): Promise<ClassInvitation> {
  const res = await apiClient.post<unknown>(`/class-invitations/${id}/accept`);
  return parseData(classInvitationSchema, res.data);
}

/** `POST /class-invitations/:id/decline` — invitee only, pending only. */
export async function declineClassInvitation(id: string): Promise<ClassInvitation> {
  const res = await apiClient.post<unknown>(`/class-invitations/${id}/decline`);
  return parseData(classInvitationSchema, res.data);
}

/** `POST /class-invitations/:id/cancel` — owner only; pending or accepted. */
export async function cancelClassInvitation(id: string): Promise<ClassInvitation> {
  const res = await apiClient.post<unknown>(`/class-invitations/${id}/cancel`);
  return parseData(classInvitationSchema, res.data);
}

/** `POST /class-invitations/:id/remind` — owner only, pending only; stamps `reminded_at`. */
export async function remindClassInvitation(id: string): Promise<ClassInvitation> {
  const res = await apiClient.post<unknown>(`/class-invitations/${id}/remind`);
  return parseData(classInvitationSchema, res.data);
}

/**
 * `POST /class-invitations/:id/confirm` — owner only; pending or accepted.
 * Writes the stint: a giao_vien invite runs the class handoff (and reports
 * the planned sessions it moved), any other role assigns the staff row.
 */
export async function confirmClassInvitation(id: string): Promise<ClassInvitationConfirm> {
  const res = await apiClient.post<unknown>(`/class-invitations/${id}/confirm`);
  return parseData(classInvitationConfirmSchema, res.data);
}
