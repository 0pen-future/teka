import { useQueries } from "@tanstack/react-query";

import {
  getSessionRoster,
  sessionsKeys,
  useSessionsList,
  type AttendanceResponse,
  type Session,
} from "@/features/attendance";
import { currentMonth } from "@/features/roster";

export interface MonthSessions {
  month: ReturnType<typeof currentMonth>;
  sessions: Session[];
  heldSessions: Session[];
  /** Rosters by session id — only entries whose query has resolved. */
  rosters: Map<string, AttendanceResponse>;
  sessionsPending: boolean;
}

/**
 * The current month's sessions for a class plus one cached roster query per
 * held session (the sessions payload only carries a preview `student_count`).
 * Query keys match `useSessionRoster`, so the classbook detail panel and the
 * student record pages all dedupe into the same cache entries.
 */
export function useMonthSessions(classId: string | undefined): MonthSessions {
  const month = currentMonth();
  const { data: sessions, isPending } = useSessionsList(classId, {
    from: month.from,
    to: month.to,
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

  return { month, sessions: sessions ?? [], heldSessions, rosters, sessionsPending: isPending };
}
