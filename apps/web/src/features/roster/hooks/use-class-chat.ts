import { useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";

import { deleteClassMessage, listClassMessages, postClassMessage } from "../api/class-chat-api";
import { classMessagesKeys } from "./roster-keys";

/**
 * Cursor-paged class chat, newest page first. Each further page passes the
 * previous page's oldest id as `before`, so a message posted meanwhile never
 * shifts what an older page returns.
 */
export function useClassMessages(classId: string, enabled = true) {
  return useInfiniteQuery({
    queryKey: classMessagesKeys.list(classId),
    queryFn: ({ pageParam }) => listClassMessages(classId, pageParam || undefined),
    initialPageParam: "",
    getNextPageParam: (lastPage) =>
      lastPage.next_cursor === "" ? undefined : lastPage.next_cursor,
    enabled,
  });
}

/** A post or delete restarts the list from the first page rather than patching cursors. */
function useMessageWrite<TVars>(classId: string, mutationFn: (vars: TVars) => Promise<unknown>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn,
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classMessagesKeys.list(classId) });
    },
  });
}

export function usePostClassMessage(classId: string) {
  return useMessageWrite(classId, (body: string) => postClassMessage(classId, body));
}

export function useDeleteClassMessage(classId: string) {
  return useMessageWrite(classId, (messageId: string) => deleteClassMessage(classId, messageId));
}
