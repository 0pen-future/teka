/**
 * Formats an ISO `YYYY-MM-DD` session date as `dd/MM` with plain string
 * slicing rather than `Date` parsing — a session date is a calendar day, not
 * an instant, so there is no timezone conversion to get wrong.
 */
export function formatChipDate(isoDate: string): string {
  const [, month, day] = isoDate.split("-");
  return month && day ? `${day}/${month}` : isoDate;
}
