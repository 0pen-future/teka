import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getCenterMe, joinCenter, removeMember, renameCenter } from "../api/center-api";

export const centerKeys = {
  me: ["center", "me"] as const,
};

export function useCenter() {
  return useQuery({ queryKey: centerKeys.me, queryFn: getCenterMe });
}

export function useRenameCenter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: renameCenter,
    onSuccess: (me) => queryClient.setQueryData(centerKeys.me, me),
  });
}

/**
 * Joining swaps the account's entire tenancy scope: every cached list —
 * classes, students, periods, dashboard — may belong to the old center.
 * Eviction, not invalidation: invalidateQueries keeps inactive queries'
 * data renderable until their refetch lands (or forever, if it fails),
 * which would paint the old center's rows to the new member.
 */
export function useJoinCenter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: joinCenter,
    onSuccess: () => queryClient.removeQueries(),
  });
}

export function useRemoveMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: removeMember,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: centerKeys.me }),
  });
}

/** Same DELETE as remove, but leaving changes the caller's own scope — evict everything. */
export function useLeaveCenter() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: removeMember,
    onSuccess: () => queryClient.removeQueries(),
  });
}
