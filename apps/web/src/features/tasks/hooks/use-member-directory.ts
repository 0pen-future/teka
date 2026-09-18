import { useQuery } from "@tanstack/react-query";

import { getMemberDirectory } from "@/features/center";

import { tasksKeys } from "./tasks-keys";

/** Assignee-picker source for the task form and board filters. */
export function useMemberDirectory() {
  return useQuery({
    queryKey: tasksKeys.directory(),
    queryFn: getMemberDirectory,
    staleTime: 60_000,
  });
}
