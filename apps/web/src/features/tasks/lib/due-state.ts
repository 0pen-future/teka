/**
 * Due-date helpers that read "today" from the caller's local calendar day
 * (`Date`'s local getters), not UTC — the bug `isOverdue` still has
 * (`task-card-styles.ts` uses `toISOString()`, which is UTC and can land on
 * the wrong day for a teacher in Asia/Ho_Chi_Minh, UTC+7). Phase 4 adds a
 * `dueState(task, now)` helper to this same file; this phase only needs the
 * quick-due-chip pieces below.
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
