import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";

import { useAuthStore } from "./stores/auth-store";

/**
 * Renders nothing. Watches the signed-in user id and clears the whole
 * TanStack Query cache whenever a session ends (id -> null) or the signed-in
 * identity changes on the same tab (id A -> id B), so the next user never
 * sees a response cached for the previous one. Query keys intentionally
 * carry no user id — this is the single place that boundary is enforced, so
 * every current and future way a session ends or swaps (logout, a dead
 * refresh, switching accounts) is covered without each call site having to
 * remember to clear the cache itself. Mounting while already signed in, a
 * token rotation, or a profile edit under the same id must not clear.
 */
export function SessionCacheReset(): null {
  const id = useAuthStore((state) => state.user?.id ?? null);
  const queryClient = useQueryClient();
  const prevRef = useRef(id);

  useEffect(() => {
    const prev = prevRef.current;
    if (prev !== null && id !== prev) {
      queryClient.clear();
    }
    prevRef.current = id;
  }, [id, queryClient]);

  return null;
}
