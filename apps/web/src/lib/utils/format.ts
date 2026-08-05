/**
 * Shared display formatting. Every feature imports from here instead of
 * inlining its own `toLocaleDateString` / manual money math — money is a
 * đồng-denominated BIGINT server-side (`docs/schema_design.sql:24`), so it
 * always renders as a locale-grouped integer, never a decimal.
 */

const moneyFormatter = new Intl.NumberFormat("vi-VN", {
  style: "currency",
  currency: "VND",
  maximumFractionDigits: 0,
});

// Fixed labels instead of Intl.DateTimeFormat: the vi-VN short-weekday form
// differs across ICU/CLDR versions ("Th 4" vs "Thứ 4"), so Intl output would
// vary by Node/browser build. Indexed by Date#getUTCDay (0 = Sunday).
const weekdayLabels = ["CN", "Th 2", "Th 3", "Th 4", "Th 5", "Th 6", "Th 7"];

/** Renders whole đồng with no fraction digits, e.g. `formatMoney(1500000)` → `"1.500.000 ₫"`. */
export function formatMoney(amountDong: number): string {
  return moneyFormatter.format(amountDong);
}

/**
 * Renders a `YYYY-MM-DD` session date as `"<thứ>, dd/MM"` (e.g. `"Th 4, 15/07"`).
 * Parsed and formatted in UTC so the calendar day never shifts with the
 * caller's local timezone — session_date is a DATE column, not an instant.
 * An unparseable value passes through unchanged rather than throwing, so a
 * malformed API response degrades to raw text instead of a crash.
 */
export function formatSessionDate(sessionDate: string): string {
  const date = new Date(`${sessionDate}T00:00:00Z`);
  if (Number.isNaN(date.getTime())) {
    return sessionDate;
  }
  const weekday = weekdayLabels[date.getUTCDay()];
  const day = String(date.getUTCDate()).padStart(2, "0");
  const month = String(date.getUTCMonth() + 1).padStart(2, "0");
  return `${weekday}, ${day}/${month}`;
}

/** Converts a stored E.164 Vietnamese number back to the local `0…` display form. */
export function formatPhoneLocal(phone: string): string {
  return phone.startsWith("+84") ? `0${phone.slice(3)}` : phone;
}
