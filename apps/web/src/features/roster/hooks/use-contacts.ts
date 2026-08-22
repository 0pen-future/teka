import { keepPreviousData, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  clearContactZaloMapping,
  createContact,
  deleteContact,
  getContact,
  listContacts,
  setContactZaloMapping,
  updateContact,
  type ListContactsParams,
} from "../api/contacts-api";
import type { ContactInput, ZaloMappingInput } from "../schemas/roster-schemas";
import { contactsKeys, studentsKeys } from "./roster-keys";

export { contactsKeys };

export function useContactsList(params: ListContactsParams = {}) {
  return useQuery({
    queryKey: contactsKeys.list(params),
    queryFn: () => listContacts(params),
    placeholderData: keepPreviousData,
  });
}

export function useContact(id: string | undefined) {
  return useQuery({
    queryKey: contactsKeys.detail(id ?? ""),
    queryFn: () => getContact(id!),
    enabled: Boolean(id),
  });
}

/**
 * `student_count` is denormalized onto each contact row, so any student
 * mutation (create, reassign contact, anonymize) can change what a contact
 * list/detail shows — every student mutation hook also invalidates
 * `contactsKeys`.
 */
export function useCreateContact() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ContactInput) => createContact(input),
    onSuccess: (contact) => {
      void queryClient.invalidateQueries({ queryKey: contactsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.detail(contact.id) });
    },
  });
}

export function useUpdateContact(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ContactInput) => updateContact(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: contactsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.detail(id) });
      // The contact's name/phone are denormalized onto every student row.
      void queryClient.invalidateQueries({ queryKey: studentsKeys.lists() });
    },
  });
}

export function useSetContactZaloMapping(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ZaloMappingInput) => setContactZaloMapping(id, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: contactsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.detail(id) });
    },
  });
}

export function useClearContactZaloMapping(id: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: () => clearContactZaloMapping(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: contactsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.detail(id) });
    },
  });
}

export function useDeleteContact() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => deleteContact(id),
    onSuccess: (_data, id) => {
      void queryClient.invalidateQueries({ queryKey: contactsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: contactsKeys.detail(id) });
    },
  });
}
