/**
 * Roster-local display formatting. `parseMoney` / `formatWeekday` live here
 * instead of the shared `apps/web/src/lib/utils/format.ts` — this feature is
 * the only consumer of either helper, and keeping them local avoids touching
 * a file outside this feature's ownership (see `adr.md`).
 */

import { deriveScheduleSlots } from "./schedule-diff";
import type { Schedule } from "../schemas/roster-schemas";

/**
 * Parses a thousands-separated đồng string (e.g. `"1.500.000"` or
 * `"1500000"`) back to an integer. Strips every non-digit character, so a
 * stray `"₫"` or space from a formatted preview never breaks the parse.
 * Returns 0 for an empty or fully non-numeric input.
 */
export function parseMoney(value: string): number {
  const digits = value.replace(/[^\d]/g, "");
  return digits === "" ? 0 : Number.parseInt(digits, 10);
}

const weekdayLabels = ["Chủ nhật", "Thứ 2", "Thứ 3", "Thứ 4", "Thứ 5", "Thứ 6", "Thứ 7"];
const weekdayShortLabels = ["CN", "T2", "T3", "T4", "T5", "T6", "T7"];

/**
 * Renders a `class_schedules.weekday` integer (0 = Chủ nhật … 6 = Thứ 7,
 * `docs/schema_design.sql:149`) as its Vietnamese label. `short` gives the
 * two-letter chip form used by the weekday picker.
 */
export function formatWeekday(weekday: number, options?: { short?: boolean }): string {
  const labels = options?.short ? weekdayShortLabels : weekdayLabels;
  return labels[weekday] ?? String(weekday);
}

/**
 * "T2 · T4 — 18:00, T6 — 20:00" — one segment per khung giờ (start time),
 * weekdays Monday-first within each. Only rows still in effect count; closed
 * rows are how a timetable change is recorded and would render the
 * pre-change weekdays too. Returns "" for a class with no active timetable.
 */
export function formatScheduleSummary(schedules: Schedule[], today: string): string {
  const mondayFirst = (weekday: number) => (weekday === 0 ? 7 : weekday);
  return deriveScheduleSlots(schedules, today)
    .map((slot) => {
      const days = [...slot.days]
        .sort((a, b) => mondayFirst(a) - mondayFirst(b))
        .map((day) => formatWeekday(day, { short: true }))
        .join(" · ");
      return `${days} — ${slot.start_time}`;
    })
    .join(", ");
}
