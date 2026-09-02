import { useQueries } from "@tanstack/react-query";

import {
  getSessionRoster,
  sessionsKeys,
  useSessionsList,
  type AttendanceResponse,
  type Session,
} from "@/features/attendance";

import { monthWindow, type MonthWindow } from "../lib/classbook-stats";

export interface MonthSessions {
  month: MonthWindow;
  sessions: Session[];
  heldSessions: Session[];
  /** Rosters by session id — only entries whose query has resolved. */
  rosters: Map<string, AttendanceResponse>;
  sessionsPending: boolean;
  sessionsError: boolean;
  refetchSessions: () => void;
}

/**
 * One month's sessions for a class plus one cached roster query per held
 * session (the sessions payload only carries a preview `student_count`).
 * `month` is "YYYY-MM"; the window ends today for the current month. Query
 * keys match `useSessionRoster`, so the classbook expand row and the student
 * record pages all dedupe into the same cache entries.
 */
export function useMonthSessions(classId: string | undefined, month: string): MonthSessions {
  const win = monthWindow(month);
  const {
    data: sessions,
    isPending,
    isError,
    refetch,
  } = useSessionsList(classId, {
    from: win.from,
    to: win.to,
  });

  const heldSessions = (sessions ?? []).filter((session) => session.status === "held");
  const rosterQueries = useQueries({
    queries: heldSessions.map((session) => ({
      queryKey: sessionsKeys.roster(session.id),
      queryFn: () => getSessionRoster(session.id),
    })),
  });
  const rosters = new Map<string, AttendanceResponse>();
  heldSessions.forEach((session, index) => {
    const data = rosterQueries[index]?.data;
    if (data) {
      rosters.set(session.id, data);
    }
  });

  return {
    month: win,
    sessions: sessions ?? [],
    heldSessions,
    rosters,
    sessionsPending: isPending,
    sessionsError: isError,
    refetchSessions: () => void refetch(),
  };
}
