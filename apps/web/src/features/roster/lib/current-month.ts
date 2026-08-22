/**
 * Month-start → today, plus the month's padded label ("08"). Shared by the
 * roster screens that show a per-month session stat so their
 * `useSessionsList` keys are identical and React Query dedupes the fetch.
 *
 * The range must stop at today: `GET /classes/:id/sessions` materializes
 * every session in the requested range, and rows written for future dates
 * would freeze the current timetable ahead of schedule changes and surface
 * as pending work on the dashboard.
 */
export function currentMonth() {
  const now = new Date();
  const first = new Date(now.getFullYear(), now.getMonth(), 1);
  const iso = (date: Date) =>
    `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(
      date.getDate(),
    ).padStart(2, "0")}`;
  return { from: iso(first), to: iso(now), label: String(now.getMonth() + 1).padStart(2, "0") };
}
