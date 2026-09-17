import { formatDayMonth } from "@/lib/utils";

/**
 * Due-date helpers that read "today" from the caller's local calendar day
 * (`Date`'s local getters), not UTC — the bug `isOverdue` used to have
 * (`toISOString()` is UTC and can land on the wrong day for a teacher in
 * Asia/Ho_Chi_Minh, UTC+7).
 */

/** `date`'s own calendar day as `YYYY-MM-DD`, read via local getters. */
export function localIsoDate(date: Date): string {
  const year = date.getFullYear();
  const month = String(date.getMonth() + 1).padStart(2, "0");
  const day = String(date.getDate()).padStart(2, "0");
  return `${year}-${month}-${day}`;
}

function addDays(date: Date, days: number): Date {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
}

/** The next Monday after `date`'s calendar day; a Monday itself rolls to +7 (next week's Monday, not today). */
function nextMonday(date: Date): Date {
  const day = date.getDay(); // 0 = Sunday … 6 = Saturday
  const daysUntilMonday = (8 - day) % 7 || 7;
  return addDays(date, daysUntilMonday);
}

export interface QuickDueOption {
  label: string;
  /** `""` clears `due_on` ("Bỏ hạn"). */
  value: string;
}

/** The four quick-due chips: Hôm nay, Ngày mai, Thứ 2 tới, Bỏ hạn — computed from `now`'s local calendar day. */
export function quickDueOptions(now: Date): QuickDueOption[] {
  return [
    { label: "Hôm nay", value: localIsoDate(now) },
    { label: "Ngày mai", value: localIsoDate(addDays(now, 1)) },
    { label: "Thứ 2 tới", value: localIsoDate(nextMonday(now)) },
    { label: "Bỏ hạn", value: "" },
  ];
}

export type DueStateKind = "overdue" | "today" | "tomorrow" | "upcoming" | "none";

export interface DueState {
  kind: DueStateKind;
  /** Calendar days past the due date; only meaningful for `kind === "overdue"`. */
  days: number;
  label: string;
  variant: "danger" | "warning" | "neutral";
}

/**
 * Calendar-day difference between `isoDate` (a bare `YYYY-MM-DD`) and `now`'s
 * local day, computed via `Date.UTC` of the two dates' y/m/d components so a
 * DST shift between them never turns a whole-day gap into a fractional one.
 */
function calendarDayDiff(isoDate: string, now: Date): number {
  const [year = 0, month = 0, day = 0] = isoDate.split("-").map(Number);
  const dueUtc = Date.UTC(year, month - 1, day);
  const nowUtc = Date.UTC(now.getFullYear(), now.getMonth(), now.getDate());
  return Math.round((dueUtc - nowUtc) / 86_400_000);
}

/**
 * A task's due-date badge state, read from `now`'s local calendar day (see
 * the module doc). A completed task or one with no due date always resolves
 * to `"none"` — the card renders its "Xong dd/MM" badge separately from
 * `completedAt`, not from this label.
 */
export function dueState(
  task: { dueOn: string | null; completedAt: string | null },
  now: Date = new Date(),
): DueState {
  if (task.completedAt !== null || task.dueOn === null) {
    return { kind: "none", days: 0, label: "", variant: "neutral" };
  }
  const diff = calendarDayDiff(task.dueOn, now);
  if (diff < 0) {
    const days = -diff;
    return { kind: "overdue", days, label: `Quá hạn ${days} ngày`, variant: "danger" };
  }
  if (diff === 0) {
    return { kind: "today", days: 0, label: "Hôm nay", variant: "warning" };
  }
  if (diff === 1) {
    return { kind: "tomorrow", days: 1, label: "Ngày mai", variant: "neutral" };
  }
  return {
    kind: "upcoming",
    days: diff,
    label: `Hạn ${formatDayMonth(task.dueOn)}`,
    variant: "neutral",
  };
}
