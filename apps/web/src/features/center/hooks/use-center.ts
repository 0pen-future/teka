import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { getCenterMe, removeMember, renameCenter, setSendReports } from "../api/center-api";

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

export function useSetSendReports() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: ({ teacherId, granted }: { teacherId: string; granted: boolean }) =>
      setSendReports(teacherId, granted),
    onSuccess: () => queryClient.invalidateQueries({ queryKey: centerKeys.me }),
  });
}

export function useRemoveMember() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: removeMember,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: centerKeys.me }),
  });
}
