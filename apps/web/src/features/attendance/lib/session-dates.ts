import type { Session } from "../schemas/attendance-schemas";

/**
 * Pure date helpers for the session picker and month calendar. Everything
 * works on `YYYY-MM-DD` / `YYYY-MM` strings in UTC — session dates are DATE
 * columns, not instants, so local-timezone Date math would shift days near
 * midnight.
 */

export function todayIso(): string {
  return new Date().toISOString().slice(0, 10);
}

export function addDaysIso(iso: string, days: number): string {
  const date = new Date(`${iso}T00:00:00Z`);
  date.setUTCDate(date.getUTCDate() + days);
  return date.toISOString().slice(0, 10);
}

/** Stable chronological order: date, then start time, then id as tiebreak. */
export function bySessionOrder(a: Session, b: Session): number {
  const dateCmp = a.session_date.localeCompare(b.session_date);
  if (dateCmp !== 0) {
    return dateCmp;
  }
  const timeCmp = (a.start_time ?? "").localeCompare(b.start_time ?? "");
  if (timeCmp !== 0) {
    return timeCmp;
  }
  return a.id.localeCompare(b.id);
}

/**
 * The default anchor when the route names no session (or names one outside
 * the window): today's session, else the nearest upcoming, else the most
 * recent past one. `sessions` must already be in `bySessionOrder`.
 */
export function resolveAnchor(
  sessions: Session[],
  selectedId: string | undefined,
  today: string,
): Session | null {
  return (
    (selectedId ? sessions.find((session) => session.id === selectedId) : undefined) ??
    sessions.find((session) => session.session_date === today) ??
    sessions.find((session) => session.session_date > today) ??
    sessions.at(-1) ??
    null
  );
}

export function monthOf(iso: string): string {
  return iso.slice(0, 7);
}

export function shiftMonth(month: string, delta: number): string {
  const [year, monthNumber] = month.split("-").map(Number);
  return new Date(Date.UTC(year!, monthNumber! - 1 + delta, 1)).toISOString().slice(0, 7);
}

/** First and last day of `month` (`YYYY-MM`) as ISO dates. */
export function monthBounds(month: string): { from: string; to: string } {
  const [year, monthNumber] = month.split("-").map(Number);
  return {
    from: `${month}-01`,
    // Day 0 of the next month = the last day of this one.
    to: new Date(Date.UTC(year!, monthNumber, 0)).toISOString().slice(0, 10),
  };
}

/** Every day of `month` in order, as ISO dates. */
export function monthDays(month: string): string[] {
  const { to } = monthBounds(month);
  const dayCount = Number(to.slice(8, 10));
  return Array.from({ length: dayCount }, (_, index) => {
    return `${month}-${String(index + 1).padStart(2, "0")}`;
  });
}

/** Monday-first column index (0–6) of an ISO date, for the 7-column grid. */
export function mondayFirstWeekday(iso: string): number {
  return (new Date(`${iso}T00:00:00Z`).getUTCDay() + 6) % 7;
}
