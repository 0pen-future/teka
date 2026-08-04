import { apiClient } from "@/lib/api/client";
import { parseData, parseList, type Paginated } from "@/lib/api/envelope";

import { contactSchema, type Contact, type ContactInput } from "../schemas/roster-schemas";

export interface ListContactsParams {
  query?: string;
  page?: number;
  per_page?: number;
  sort?: string;
}

/** `GET /contacts` (`apps/api/internal/features/contacts/handler.go`). */
export async function listContacts(params: ListContactsParams = {}): Promise<Paginated<Contact>> {
  const res = await apiClient.get<unknown>("/contacts", { params });
  return parseList(contactSchema, res.data);
}

export async function getContact(id: string): Promise<Contact> {
  const res = await apiClient.get<unknown>(`/contacts/${id}`);
  return parseData(contactSchema, res.data);
}

export async function createContact(input: ContactInput): Promise<Contact> {
  const res = await apiClient.post<unknown>("/contacts", input);
  return parseData(contactSchema, res.data);
}

/** `PUT /contacts/:id` — full replace, there is no PATCH on this resource. */
export async function updateContact(id: string, input: ContactInput): Promise<Contact> {
  const res = await apiClient.put<unknown>(`/contacts/${id}`, input);
  return parseData(contactSchema, res.data);
}

/** Soft delete; the API returns 409 while live students still reference the contact. */
export async function deleteContact(id: string): Promise<void> {
  await apiClient.delete(`/contacts/${id}`);
}
