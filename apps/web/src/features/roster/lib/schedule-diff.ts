import type { UpdateScheduleInput } from "../api/classes-api";
import type {
  Schedule,
  ScheduleInput,
  ScheduleRowInput,
  ScheduleSlotInput,
} from "../schemas/roster-schemas";

/** A row to close via `PUT /classes/:id/schedules/:sid` with `effective_to` set. */
export interface ScheduleClose {
  id: string;
  input: UpdateScheduleInput;
}

/**
 * Mutations the "Sửa lớp học" save needs to reconcile the class's weekly
 * timetable with the wizard's schedule rows.
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

/**
 * The class's active rows as the wizard edits them — one row per schedule,
 * Monday first, then by start time.
 */
export function activeScheduleRows(schedules: Schedule[], today: string): ScheduleRowInput[] {
  const mondayFirst = (weekday: number) => (weekday === 0 ? 7 : weekday);
  return activeSchedules(schedules, today)
    .map((schedule) => ({
      weekday: schedule.weekday,
      start_time: toHhmm(schedule.start_time),
      duration_min: schedule.duration_min,
    }))
    .sort(
      (a, b) =>
        mondayFirst(a.weekday) - mondayFirst(b.weekday) || a.start_time.localeCompare(b.start_time),
    );
}

/** The identity a schedule row has from the wizard's viewpoint. */
function rowKey(weekday: number, hhmm: string, duration: number): string {
  return `${weekday}|${hhmm}|${duration}`;
}

/**
 * Diffs the class's active timetable against the wizard's rows. The API
 * contract (`classes.UpdateScheduleRequest`) prescribes that a real timetable
 * change closes the old row and adds a new one, so sessions the old row
 * already explains stay queryable for past ranges. New rows start today
 * (`effective_from = today`) and replaced rows close yesterday, so attended
 * or billed sessions are never rewritten.
 *
 * A row survives only if the wizard still lists its exact (weekday, time,
 * duration); otherwise it is closed — or deleted when it never took effect.
 * Pass no rows to retire the whole timetable (a self-paced class).
 */
export function diffScheduleRows(
  schedules: Schedule[],
  rows: ScheduleRowInput[],
  today: string,
): ScheduleDiff {
  const wanted = new Map<string, ScheduleInput>();
  for (const row of rows) {
    if (row.duration_min === null) continue;
    wanted.set(rowKey(row.weekday, row.start_time, row.duration_min), {
      weekday: row.weekday,
      start_time: row.start_time,
      duration_min: row.duration_min,
      effective_from: today,
    });
  }
  const kept = new Set<string>();
  const toClose: ScheduleClose[] = [];
  const toDelete: string[] = [];
  const closeOn = dayBefore(today);

  for (const schedule of activeSchedules(schedules, today)) {
    const key = rowKey(schedule.weekday, toHhmm(schedule.start_time), schedule.duration_min);
    if (wanted.has(key) && !kept.has(key)) {
      kept.add(key);
      continue;
    }
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

  const toAdd = [...wanted.entries()].filter(([key]) => !kept.has(key)).map(([, input]) => input);
  return { toAdd, toClose, toDelete };
}
