import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  acceptInvite,
  createInvite,
  listInvites,
  previewInvite,
  revokeInvite,
} from "../api/invitation-api";

export const invitationKeys = {
  list: ["invitations", "list"] as const,
  preview: (token: string) => ["invitations", "preview", token] as const,
};

export function useInvites() {
  return useQuery({ queryKey: invitationKeys.list, queryFn: listInvites });
}

export function useCreateInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: createInvite,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitationKeys.list }),
  });
}

export function useRevokeInvite() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: revokeInvite,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: invitationKeys.list }),
  });
}

/**
 * `retry: false` and `gcTime: 0` mirror the public statement preview
 * (`features/statement/hooks/use-statement.ts`): a wrong or since-revoked
 * token should fail fast, not spend three retries on a 404, and a stale
 * cached preview must not survive to be shown for a token that has since
 * been revoked or accepted on a shared device.
 */
export function useInvitePreview(token: string | undefined) {
  return useQuery({
    queryKey: invitationKeys.preview(token ?? ""),
    queryFn: () => previewInvite(token!),
    enabled: Boolean(token),
    staleTime: 0,
    gcTime: 0,
    retry: false,
  });
}

export function useAcceptInvite() {
  return useMutation({ mutationFn: acceptInvite });
}
