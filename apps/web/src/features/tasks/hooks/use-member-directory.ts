import { useQuery } from "@tanstack/react-query";

import { getMemberDirectory } from "@/features/center";

import { tasksKeys } from "./tasks-keys";

/**
 * Assignee-picker source for the task form, board filters and the library's
 * prep assignment. `enabled` lets a caller skip the request when the viewer
 * lacks `members.list` or has nothing to resolve.
 */
export function useMemberDirectory(enabled = true) {
  return useQuery({
    queryKey: tasksKeys.directory(),
    queryFn: getMemberDirectory,
    staleTime: 60_000,
    enabled,
  });
}
