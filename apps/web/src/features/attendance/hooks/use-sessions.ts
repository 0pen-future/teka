import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import {
  cancelSession,
  confirmAttendance,
  getPeriodForDate,
  getSession,
  getSessionRoster,
  listClassSessions,
} from "../api/attendance-api";
import type { CancelSessionInput, ConfirmAttendanceInput } from "../schemas/attendance-schemas";

export const sessionsKeys = {
  all: ["attendance", "sessions"] as const,
  lists: () => [...sessionsKeys.all, "list"] as const,
  list: (classId: string, params: { from: string; to: string }) =>
    [...sessionsKeys.lists(), classId, params] as const,
  details: () => [...sessionsKeys.all, "detail"] as const,
  detail: (id: string) => [...sessionsKeys.details(), id] as const,
  rosters: () => [...sessionsKeys.all, "roster"] as const,
  roster: (id: string) => [...sessionsKeys.rosters(), id] as const,
  periods: () => [...sessionsKeys.all, "period-for-date"] as const,
  periodForDate: (sessionDate: string) => [...sessionsKeys.periods(), sessionDate] as const,
};

/**
 * `dashboard.dashboardKeys.pendingSessions()`
 * (`apps/web/src/features/dashboard/hooks/use-dashboard.ts`) has no barrel
 * export — the dashboard feature exposes no `index.ts` — so the literal key
 * is duplicated here rather than imported. Keep in sync if that key changes.
 */
const dashboardPendingSessionsKey = ["dashboard", "pending-sessions"] as const;

/**
 * `billing.billingKeys.currentPeriod()`
 * (`apps/web/src/features/billing/hooks/use-billing.ts`) is not exported
 * from the billing barrel either; same duplication rationale as above. An
 * attendance save can flip a period's computed totals, so the sidebar/dash
 * card that reads the current period must refetch too.
 */
const billingCurrentPeriodKey = ["billing", "period", "current"] as const;

export function useSessionsList(classId: string | undefined, params: { from: string; to: string }) {
  return useQuery({
    queryKey: sessionsKeys.list(classId ?? "", params),
    queryFn: () => listClassSessions(classId!, params),
    enabled: Boolean(classId),
  });
}

export function useSession(sessionId: string | undefined) {
  return useQuery({
    queryKey: sessionsKeys.detail(sessionId ?? ""),
    queryFn: () => getSession(sessionId!),
    enabled: Boolean(sessionId),
  });
}

export function useSessionRoster(sessionId: string | undefined) {
  return useQuery({
    queryKey: sessionsKeys.roster(sessionId ?? ""),
    queryFn: () => getSessionRoster(sessionId!),
    enabled: Boolean(sessionId),
  });
}

/**
 * Resolves whether the billing period covering `sessionDate` is closed, so
 * `AttendancePage` can warn before the teacher commits rather than after
 * (see the api module's `getPeriodForDate` doc comment for why this workaround
 * exists). A five-minute staleTime matches `useCurrentPeriod`'s.
 */
export function usePeriodForDate(sessionDate: string | undefined) {
  return useQuery({
    queryKey: sessionsKeys.periodForDate(sessionDate ?? ""),
    queryFn: () => getPeriodForDate(sessionDate!),
    enabled: Boolean(sessionDate),
    staleTime: 5 * 60 * 1000,
  });
}

/**
 * The one request the whole one-touch screen issues. Invalidates: this
 * session's roster and detail (reopening must show the just-saved marks),
 * the class's session list (confirmed status/absent count changes), the
 * dashboard's pending feed (a newly confirmed session must drop off it), and
 * the current-period card (attendance changes are exactly what billing totals
 * read).
 */
export function useSaveAttendance(sessionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: ConfirmAttendanceInput) => confirmAttendance(sessionId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.roster(sessionId) });
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.detail(sessionId) });
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: dashboardPendingSessionsKey });
      void queryClient.invalidateQueries({ queryKey: billingCurrentPeriodKey });
    },
  });
}

export function useCancelSession(sessionId: string) {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: (input: CancelSessionInput) => cancelSession(sessionId, input),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.detail(sessionId) });
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.roster(sessionId) });
      void queryClient.invalidateQueries({ queryKey: sessionsKeys.lists() });
      void queryClient.invalidateQueries({ queryKey: dashboardPendingSessionsKey });
    },
  });
}
