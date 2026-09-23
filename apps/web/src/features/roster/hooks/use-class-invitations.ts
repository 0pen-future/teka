import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  acceptClassInvitation,
  cancelClassInvitation,
  confirmClassInvitation,
  declineClassInvitation,
  listClassInvitations,
  remindClassInvitation,
  sendClassInvitation,
  type ListClassInvitationsParams,
} from "../api/class-invitations-api";
import { sessionsKeys } from "@/features/attendance";

import type { ClassInvitationSendInput } from "../schemas/roster-schemas";
import { classInvitationsKeys, classStaffKeys, classesKeys } from "./roster-keys";

export { classInvitationsKeys };

export function useClassInvitations(params: ListClassInvitationsParams = {}, enabled = true) {
  return useQuery({
    queryKey: classInvitationsKeys.list(params),
    queryFn: () => listClassInvitations(params),
    enabled,
  });
}

export function useSendClassInvitation(classId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ClassInvitationSendInput) => sendClassInvitation(classId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: classInvitationsKeys.lists() });
    },
  });
}

/** One hook for the four status-only transitions: every list keyed on invitations refetches. */
function useInvitationTransition(transition: (id: string) => Promise<unknown>) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => transition(id),
    // Settled, not success: a 409 means the row changed under the caller, and
    // the refetch is what replaces the stale row and its buttons.
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: classInvitationsKeys.lists() });
    },
  });
}

export function useAcceptClassInvitation() {
  return useInvitationTransition(acceptClassInvitation);
}

export function useDeclineClassInvitation() {
  return useInvitationTransition(declineClassInvitation);
}

export function useCancelClassInvitation() {
  return useInvitationTransition(cancelClassInvitation);
}

export function useRemindClassInvitation() {
  return useInvitationTransition(remindClassInvitation);
}

/**
 * Confirm writes a stint, so the class's staff list and detail refetch
 * alongside the invitation lists. A giao_vien confirm is a handoff: it
 * repoints the class and moves its upcoming sessions, so it drops every
 * class and session query the way `useReassignTeacher` does.
 */
export function useConfirmClassInvitation() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => confirmClassInvitation(id),
    onSuccess: (confirmed) => {
      void queryClient.invalidateQueries({ queryKey: classStaffKeys.list(confirmed.class_id) });
      if (confirmed.role_key === "giao_vien") {
        void queryClient.invalidateQueries({ queryKey: classesKeys.all });
        void queryClient.invalidateQueries({ queryKey: sessionsKeys.all });
      } else {
        void queryClient.invalidateQueries({ queryKey: classesKeys.detail(confirmed.class_id) });
        void queryClient.invalidateQueries({ queryKey: classesKeys.lists() });
      }
    },
    onSettled: () => {
      void queryClient.invalidateQueries({ queryKey: classInvitationsKeys.lists() });
    },
  });
}
