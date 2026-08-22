import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getCenterMe, removeMember, renameCenter } from "../api/center-api";

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

export function useRemoveMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: removeMember,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: centerKeys.me }),
  });
}
