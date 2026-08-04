import { useQuery } from "@tanstack/react-query";

import { getStatement } from "../api/statement-api";

export const statementKeys = {
  all: ["statement"] as const,
  detail: (token: string) => [...statementKeys.all, token] as const,
};

/**
 * Always fetches fresh, overriding the app-wide 30s `staleTime`
 * (`apps/web/src/app/providers.tsx`): a parent may reload after the teacher
 * edits attendance or records a payment, and a stale cached figure here is
 * worse than a spinner. `retry: false` avoids spending three attempts (and
 * three seconds of staring at a spinner) on a token that is simply wrong.
 * `gcTime: 0` keeps a previous parent's statement out of memory on a shared
 * device.
 */
export function useStatement(token: string | undefined) {
  return useQuery({
    queryKey: statementKeys.detail(token ?? ""),
    queryFn: () => getStatement(token!),
    enabled: Boolean(token),
    staleTime: 0,
    gcTime: 0,
    retry: false,
    refetchOnMount: "always",
  });
}
