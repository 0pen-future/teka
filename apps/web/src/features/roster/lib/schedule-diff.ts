import type { UpdateScheduleInput } from "../api/classes-api";
import type { Schedule, ScheduleInput, ScheduleSlotInput } from "../schemas/roster-schemas";

/** A row to close via `PUT /classes/:id/schedules/:sid` with `effective_to` set. */
export interface ScheduleClose {
  id: string;
  input: UpdateScheduleInput;
}

/**
 * Mutations the "Cài đặt lớp" save needs to reconcile the class's weekly
 * timetable with the form's khung-giờ slot list.
 */
export interface ScheduleDiff {
  /** Rows to `POST /classes/:id/schedules`. Applied first so a mid-sequence failure can never leave the class without a timetable. */
  toAdd: ScheduleInput[];
  /** Rows replaced or removed that already generated sessions — closed with `effective_to` = yesterday so past sessions stay explicable. */
  toClose: ScheduleClose[];
  /** Rows whose `effective_from` is today or later — they never took effect, so deleting them outright loses no history. */
  toDelete: string[];
}

/** The prototype's starter slot: 19:00, no day picked yet. */
export function emptySlot(): ScheduleSlotInput {
  return { start_time: "19:00", days: [] };
}

/** Sessions per week across every slot — feeds the "· N buổi/tuần" header. */
export function weeklySessionCount(slots: ScheduleSlotInput[]): number {
  return slots.reduce((total, slot) => total + slot.days.length, 0);
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
 * Derives the settings form's khung-giờ slots from the class's active rows:
 * rows sharing a start time collapse into one slot listing every weekday it
 * repeats on, ordered by time so the earliest slot renders first. Returns an
 * empty list when nothing is active — the caller supplies the blank starter
 * slot.
 */
export function deriveScheduleSlots(schedules: Schedule[], today: string): ScheduleSlotInput[] {
  const active = activeSchedules(schedules, today);
  const byTime = new Map<string, number[]>();
  for (const schedule of active) {
    const time = toHhmm(schedule.start_time);
    const days = byTime.get(time) ?? [];
    if (!days.includes(schedule.weekday)) {
      days.push(schedule.weekday);
    }
    byTime.set(time, days);
  }
  return [...byTime.entries()]
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([start_time, days]) => ({ start_time, days }));
}

/** The (weekday, time) identity a schedule row has from the forms' viewpoint. */
function pairKey(weekday: number, hhmm: string): string {
  return `${weekday}|${hhmm}`;
}

/**
 * Diffs the class's active timetable against the form's slot list. The API
 * contract (`classes.UpdateScheduleRequest`) prescribes that a real timetable
 * change closes the old row and adds a new one, so sessions the old row
 * already explains stay queryable for past ranges. New rows start today
 * (`effective_from = today`) and replaced rows close yesterday, so the change
 * applies "từ buổi kế tiếp" and never rewrites attended or billed sessions.
 *
 * A row survives only if some slot still names its (weekday, time) pair;
 * otherwise it is closed — or deleted when it never took effect. Wanted pairs
 * without a surviving row get a new one, preserving the replaced row's
 * duration when that weekday had one (else the class's most common duration,
 * else 90).
 */
export function diffSchedules(
  schedules: Schedule[],
  slots: ScheduleSlotInput[],
  today: string,
): ScheduleDiff {
  const wanted = new Map<string, { weekday: number; start_time: string }>();
  for (const slot of slots) {
    for (const weekday of slot.days) {
      wanted.set(pairKey(weekday, slot.start_time), { weekday, start_time: slot.start_time });
    }
  }
  const active = activeSchedules(schedules, today);
  const kept = new Set<string>();
  const toClose: ScheduleClose[] = [];
  const toDelete: string[] = [];
  const replacedDuration = new Map<number, number>();
  const closeOn = dayBefore(today);

  for (const schedule of active) {
    const key = pairKey(schedule.weekday, toHhmm(schedule.start_time));
    if (wanted.has(key) && !kept.has(key)) {
      kept.add(key);
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

  const toAdd: ScheduleInput[] = [...wanted.entries()]
    .filter(([key]) => !kept.has(key))
    .map(([, pair]) => ({
      weekday: pair.weekday,
      start_time: pair.start_time,
      duration_min: replacedDuration.get(pair.weekday) ?? commonDuration,
      effective_from: today,
    }));

  return { toAdd, toClose, toDelete };
}
