import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

import {
  classMessagePageSchema,
  classMessageSchema,
  type ClassMessage,
  type ClassMessagePage,
} from "../schemas/class-program-schemas";

/** `GET /classes/:id/messages?before=&limit=` — newest first, cursor on the oldest id shown. */
export async function listClassMessages(
  classId: string,
  before?: string,
): Promise<ClassMessagePage> {
  const res = await apiClient.get<unknown>(`/classes/${classId}/messages`, {
    params: before ? { before } : {},
  });
  return parseData(classMessagePageSchema, res.data);
}

export async function postClassMessage(classId: string, body: string): Promise<ClassMessage> {
  const res = await apiClient.post<unknown>(`/classes/${classId}/messages`, { body });
  return parseData(classMessageSchema, res.data);
}

/** Author or owner only; the API answers 403 otherwise. */
export async function deleteClassMessage(classId: string, messageId: string): Promise<void> {
  await apiClient.delete(`/classes/${classId}/messages/${messageId}`);
}
