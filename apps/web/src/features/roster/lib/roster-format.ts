/**
 * Roster-local display formatting. `parseMoney` / `formatWeekday` live here
 * instead of the shared `apps/web/src/lib/utils/format.ts` — this feature is
 * the only consumer of either helper, and keeping them local avoids touching
 * a file outside this feature's ownership (see `adr.md`).
 */

import { activeSchedules, deriveScheduleSlots } from "./schedule-diff";
import type { ClassShift } from "../api/classes-api";
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
/** Spelled-out form for a single-day label ("Tối Thứ Ba"), title-cased like a class name. */
const weekdayWordLabels = [
  "Chủ Nhật",
  "Thứ Hai",
  "Thứ Ba",
  "Thứ Tư",
  "Thứ Năm",
  "Thứ Sáu",
  "Thứ Bảy",
];

/**
 * Renders a `class_schedules.weekday` integer (0 = Chủ nhật … 6 = Thứ 7,
 * `docs/schema_design.sql:149`) as its Vietnamese label. `short` gives the
 * two-letter chip form used by the weekday picker; `word` the spelled-out
 * "Thứ Ba" form.
 */
export function formatWeekday(
  weekday: number,
  options?: { short?: boolean; word?: boolean },
): string {
  const labels = options?.short
    ? weekdayShortLabels
    : options?.word
      ? weekdayWordLabels
      : weekdayLabels;
  return labels[weekday] ?? String(weekday);
}

/** Sunday last, the way a Vietnamese timetable reads. */
const mondayFirst = (weekday: number) => (weekday === 0 ? 7 : weekday);

/**
 * "T2 · T4 — 18:00, T6 — 20:00" — one segment per khung giờ (start time),
 * weekdays Monday-first within each. Only rows still in effect count; closed
 * rows are how a timetable change is recorded and would render the
 * pre-change weekdays too. Returns "" for a class with no active timetable.
 */
export function formatScheduleSummary(schedules: Schedule[], today: string): string {
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

/** "Sáng" before noon, "Chiều" until 18:00, "Tối" after — how teachers name a khung giờ. */
function formatDayPart(hhmm: string): string {
  const hour = Number.parseInt(hhmm.slice(0, 2), 10);
  if (Number.isNaN(hour)) return "";
  if (hour < 12) return "Sáng";
  if (hour < 18) return "Chiều";
  return "Tối";
}

/**
 * The spoken name of a class's timetable, for labels that sit next to the
 * class name ("Toán 8 · Tối Thứ Ba"): day part + weekday. One day spells the
 * weekday out; several days in the same khung giờ collapse to "Tối T2-T4-T6";
 * several khung giờ join with ", " in start-time order ("Sáng T7, Tối T3-T5"). Only rows still in
 * effect count, like `formatScheduleSummary`. Returns "" with no timetable.
 */
export function formatScheduleLabel(schedules: Schedule[], today: string): string {
  return deriveScheduleSlots(schedules, today)
    .map((slot) => {
      const days = [...slot.days].sort((a, b) => mondayFirst(a) - mondayFirst(b));
      const dayLabel =
        days.length === 1
          ? (weekdayWordLabels[days[0]!] ?? String(days[0]))
          : days.map((day) => formatWeekday(day, { short: true })).join("-");
      const part = formatDayPart(slot.start_time);
      return part ? `${part} ${dayLabel}` : dayLabel;
    })
    .join(", ");
}

/** One weekly session of a class as the list row prints it. */
export interface ScheduleLine {
  key: string;
  /** "Thứ Ba, 19:00 - 20:30" */
  text: string;
  shift: ClassShift;
}

/** The API's shift bands (`classes.ShiftOf`): before 12:00, before 17:30, then evening. */
function shiftOf(hhmm: string): ClassShift {
  if (hhmm < "12:00") return "morning";
  if (hhmm < "17:30") return "afternoon";
  return "evening";
}

export function addMinutes(hhmm: string, minutes: number): string {
  const [hour = 0, minute = 0] = hhmm.split(":").map(Number);
  const total = (hour * 60 + minute + minutes) % (24 * 60);
  return `${String(Math.floor(total / 60)).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}

/**
 * One line per weekly session still in effect ("Thứ Ba, 19:00 - 20:30"),
 * Monday-first then by start time, each tagged with the shift the list's
 * dot colour and the shift filter share. Empty with no timetable.
 */
export function formatScheduleLines(schedules: Schedule[], today: string): ScheduleLine[] {
  return activeSchedules(schedules, today)
    .map((schedule) => ({ ...schedule, start: schedule.start_time.slice(0, 5) }))
    .sort(
      (a, b) => mondayFirst(a.weekday) - mondayFirst(b.weekday) || a.start.localeCompare(b.start),
    )
    .map((schedule) => ({
      key: schedule.id,
      text: `${weekdayWordLabels[schedule.weekday] ?? schedule.weekday}, ${schedule.start} - ${addMinutes(schedule.start, schedule.duration_min)}`,
      shift: shiftOf(schedule.start),
    }));
}

/** "05/01/2026" for a DATE column value; anything unparsable passes through. */
export function formatFullDate(isoDate: string): string {
  const [year, month, day] = isoDate.split("-");
  if (!year || !month || !day) {
    return isoDate;
  }
  return `${day}/${month}/${year}`;
}
