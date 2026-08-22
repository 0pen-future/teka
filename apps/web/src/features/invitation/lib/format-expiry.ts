/**
 * Renders an ISO expiry timestamp as a short vi-VN date, e.g.
 * `formatExpiresAt("2026-08-19T10:00:00Z")` → `"19/08/2026"`. An unparseable
 * value passes through unchanged rather than throwing, so a malformed API
 * response degrades to raw text instead of a crash.
 */
export function formatExpiresAt(isoDate: string): string {
  const date = new Date(isoDate);
  if (Number.isNaN(date.getTime())) {
    return isoDate;
  }
  return date.toLocaleDateString("vi-VN");
}
