import type { Class } from "../schemas/roster-schemas";

/**
 * Whether the caller may write to a class (class edit, schedule, roster,
 * classbook notes/marks/scores, attendance confirm): the center owner, or
 * whoever holds the class's `giao_vien` staff role. `hoc_vu`/`tro_giang`
 * staff can read everything about the class but the API 403s their writes,
 * so callers gate mutation triggers on this to avoid manufacturing them.
 */
export function canWriteClass(isOwner: boolean, klass: Pick<Class, "my_staff_roles">): boolean {
  return isOwner || klass.my_staff_roles.includes("giao_vien");
}

/**
 * Whether the caller may record/confirm attendance on the class. Wider than
 * canWriteClass: the API's attendance.write capability also grants the
 * `tro_giang` staff role, while every other class write stays giao_vien-only.
 */
export function canRecordAttendance(
  isOwner: boolean,
  klass: Pick<Class, "my_staff_roles">,
): boolean {
  return (
    isOwner ||
    klass.my_staff_roles.includes("giao_vien") ||
    klass.my_staff_roles.includes("tro_giang")
  );
}

/**
 * Whether the caller may send this class's statement copies (the class-scoped
 * bulk send): reports oversight (owner or can_send_reports delegate) passes
 * center-wide, and the class's `hoc_vu` staff role grants it per class. The
 * teaching roles read the class but the API 403s their sends.
 */
export function canSendClassReports(
  hasReportsOversight: boolean,
  klass: Pick<Class, "my_staff_roles">,
): boolean {
  return hasReportsOversight || klass.my_staff_roles.includes("hoc_vu");
}
