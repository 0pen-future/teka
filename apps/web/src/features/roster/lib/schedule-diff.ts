import type { UpdateScheduleInput } from "../api/classes-api";
import type { Schedule, ScheduleInput } from "../schemas/roster-schemas";

/** A row to close via `PUT /classes/:id/schedules/:sid` with `effective_to` set. */
export interface ScheduleClose {
  id: string;
  input: UpdateScheduleInput;
}

/**
 * Mutations the "Cài đặt lớp" save needs to reconcile the class's weekly
 * timetable with the form's single days-set + shared start time.
 */
export interface ScheduleDiff {
  /** Rows to `POST /classes/:id/schedules`. Applied first so a mid-sequence failure can never leave the class without a timetable. */
  toAdd: ScheduleInput[];
  /** Rows replaced or removed that already generated sessions — closed with `effective_to` = yesterday so past sessions stay explicable. */
  toClose: ScheduleClose[];
  /** Rows whose `effective_from` is today or later — they never took effect, so deleting them outright loses no history. */
  toDelete: string[];
}

/** Server times may carry seconds ("18:00:00"); the form always uses HH:MM. */
function toHhmm(time: string): string {
  return time.slice(0, 5);
}

/** The calendar date before an ISO date string, timezone-independent. */
function dayBefore(date: string): string {
  const parsed = new Date(`${date}T00:00:00`);
  parsed.setDate(parsed.getDate() - 1);
  return `${parsed.getFullYear()}-${String(parsed.getMonth() + 1).padStart(2, "0")}-${String(
    parsed.getDate(),
  ).padStart(2, "0")}`;
}

/**
 * Rows still in effect on `today` — a closed row (effective_to in the past)
 * no longer generates sessions and must not be re-closed or block a re-add
 * of the same weekday.
 */
export function activeSchedules(schedules: Schedule[], today: string): Schedule[] {
  return schedules.filter((schedule) => !schedule.effective_to || schedule.effective_to >= today);
}

/**
 * Derives the settings form's initial timetable fields from the class's
 * active rows. When rows disagree on start time (legal in the data model,
 * inexpressible in the classCfg screen's single "Giờ học" field), the most
 * common time wins; saving then unifies every selected day to it.
 */
export function deriveScheduleForm(
  schedules: Schedule[],
  today: string,
): { days: number[]; start_time: string } {
  const active = activeSchedules(schedules, today);
  const days = [...new Set(active.map((schedule) => schedule.weekday))];
  const counts = new Map<string, number>();
  for (const schedule of active) {
    const time = toHhmm(schedule.start_time);
    counts.set(time, (counts.get(time) ?? 0) + 1);
  }
  let start_time = "";
  let best = 0;
  for (const [time, count] of counts) {
    if (count > best) {
      start_time = time;
      best = count;
    }
  }
  return { days, start_time };
}

/**
 * Diffs the class's active timetable against the form's selection. The API
 * contract (`classes.UpdateScheduleRequest`) prescribes that a real timetable
 * change closes the old row and adds a new one, so sessions the old row
 * already explains stay queryable for past ranges. New rows start today
 * (`effective_from = today`) and replaced rows close yesterday, so the change
 * applies "từ buổi kế tiếp" and never rewrites attended or billed sessions.
 *
 * A row survives only if its weekday is still selected AND its time matches;
 * otherwise it is closed — or deleted when it never took effect. Selected
 * weekdays without a surviving row get a new one, preserving the replaced
 * row's duration when there was one (else the class's most common duration,
 * else 90).
 */
export function diffSchedules(
  schedules: Schedule[],
  days: number[],
  startTime: string,
  today: string,
): ScheduleDiff {
  const wantedDays = [...new Set(days)];
  const active = activeSchedules(schedules, today);
  const kept = new Set<number>();
  const toClose: ScheduleClose[] = [];
  const toDelete: string[] = [];
  const replacedDuration = new Map<number, number>();
  const closeOn = dayBefore(today);

  for (const schedule of active) {
    const wanted =
      wantedDays.includes(schedule.weekday) && toHhmm(schedule.start_time) === startTime;
    if (wanted && !kept.has(schedule.weekday)) {
      kept.add(schedule.weekday);
      continue;
    }
    replacedDuration.set(schedule.weekday, schedule.duration_min);
    if (schedule.effective_from >= today) {
      toDelete.push(schedule.id);
    } else {
      toClose.push({
        id: schedule.id,
        input: {
          weekday: schedule.weekday,
          start_time: toHhmm(schedule.start_time),
          duration_min: schedule.duration_min,
          effective_from: schedule.effective_from,
          effective_to: closeOn,
        },
      });
    }
  }

  const durationCounts = new Map<number, number>();
  for (const schedule of active) {
    durationCounts.set(schedule.duration_min, (durationCounts.get(schedule.duration_min) ?? 0) + 1);
  }
  let commonDuration = 90;
  let best = 0;
  for (const [duration, count] of durationCounts) {
    if (count > best) {
      commonDuration = duration;
      best = count;
    }
  }

  const toAdd: ScheduleInput[] = wantedDays
    .filter((day) => !kept.has(day))
    .map((day) => ({
      weekday: day,
      start_time: startTime,
      duration_min: replacedDuration.get(day) ?? commonDuration,
      effective_from: today,
    }));

  return { toAdd, toClose, toDelete };
}
