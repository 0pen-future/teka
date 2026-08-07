import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  getZaloFriends,
  getZaloLinkStatus,
  getZaloStatus,
  matchZaloFriends,
  sendZaloFriendRequest,
  startZaloLink,
  unlinkZalo,
} from "../api/zalo-api";
import { isTerminalLinkState } from "../schemas/zalo-schemas";

/** How often an in-flight attempt is polled while the modal is open. */
export const ZALO_POLL_INTERVAL_MS = 1500;

/**
 * How many failed polls end an attempt. One bad response is a glitch worth
 * riding out; a run of them means the attempt is gone and no amount of
 * polling will bring it back.
 */
export const ZALO_MAX_POLL_ERRORS = 3;

/**
 * How long a fetched friend list is served from cache. Every fetch is a live
 * call from the teacher's personal Zalo account, so opening and closing the
 * picker repeatedly must not fire a burst of requests at Zalo.
 */
export const ZALO_FRIENDS_STALE_MS = 60_000;

export const zaloKeys = {
  all: ["zalo"] as const,
  status: () => [...zaloKeys.all, "status"] as const,
  friends: () => [...zaloKeys.all, "friends"] as const,
  link: (linkId: string) => [...zaloKeys.all, "link", linkId] as const,
};

export function useZaloStatus() {
  return useQuery({
    queryKey: zaloKeys.status(),
    queryFn: getZaloStatus,
  });
}

/** Friend list for the mapping picker. Fetches only while the picker is open. */
export function useZaloFriends(enabled: boolean) {
  return useQuery({
    queryKey: zaloKeys.friends(),
    queryFn: getZaloFriends,
    enabled,
    staleTime: ZALO_FRIENDS_STALE_MS,
  });
}

/**
 * A mutation (not a query) although it only reads: every call is a live,
 * paced Zalo lookup, so it must run once per explicit user action and never
 * refetch on remount or focus. It touches no cached data, so nothing is
 * invalidated. Callers pass an `AbortSignal` so an abandoned dialog stops the
 * paced server-side work instead of leaving it running against Zalo.
 */
export function useMatchZaloFriends() {
  return useMutation({
    mutationFn: ({ phones, signal }: { phones: string[]; signal?: AbortSignal }) =>
      matchZaloFriends(phones, signal),
  });
}

/** One friend request per explicit click; there is no bulk variant to wrap. */
export function useSendZaloFriendRequest() {
  return useMutation({
    mutationFn: (userId: string) => sendZaloFriendRequest(userId),
  });
}

export function useStartZaloLink() {
  return useMutation({
    mutationFn: (consentVersion: string) => startZaloLink(consentVersion),
  });
}

/**
 * Polls one attempt while it exists. The interval switches itself off on a
 * terminal state so a finished attempt cannot leave a timer hitting the API
 * forever, and off again after a run of failed polls: the server keeps one
 * attempt per teacher, so a persistent 404 means this attempt is gone (a
 * second tab replaced it, or the API restarted) and polling cannot revive it.
 * Callers surface that and offer a fresh attempt instead.
 */
export function useZaloLinkStatus(linkId: string | undefined) {
  return useQuery({
    queryKey: zaloKeys.link(linkId ?? ""),
    queryFn: () => getZaloLinkStatus(linkId!),
    enabled: Boolean(linkId),
    refetchInterval: (query) => {
      if (query.state.errorUpdateCount >= ZALO_MAX_POLL_ERRORS) {
        return false;
      }
      return isTerminalLinkState(query.state.data?.state) ? false : ZALO_POLL_INTERVAL_MS;
    },
    // Each attempt id is used once; keeping its last poll cached would show a
    // stale state for a moment when the next attempt starts.
    gcTime: 0,
  });
}

export function useUnlinkZalo() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: unlinkZalo,
    onSuccess: () => queryClient.invalidateQueries({ queryKey: zaloKeys.status() }),
  });
}
