import type { Session } from "@/features/attendance";

import type { Schedule } from "../schemas/roster-schemas";

/** How far past today an open-ended class is materialised. */
export const OPEN_ENDED_HORIZON_DAYS = 90;

export function addDays(isoDate: string, days: number): string {
  const date = new Date(`${isoDate}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

/**
 * Whether a session matches a schedule row effective on its date — i.e. it
 * was generated from the weekly timetable rather than added by hand.
 */
export function isScheduledSession(session: Session, schedules: Schedule[]): boolean {
  const weekday = new Date(`${session.session_date}T00:00:00Z`).getUTCDay();
  return schedules.some(
    (schedule) =>
      schedule.weekday === weekday &&
      schedule.start_time === session.start_time &&
      schedule.effective_from <= session.session_date &&
      (schedule.effective_to === null || session.session_date <= schedule.effective_to),
  );
}
