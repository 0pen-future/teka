import { z } from "zod";

/**
 * `sessions.AttendanceSummary` — per-status counts over the session's live
 * attendance records. Null (not zeros) until the session is first confirmed;
 * a confirmed-empty roster is the all-zero non-null object, so calendar
 * badges can tell "chưa điểm danh" apart from "đã điểm danh, lớp trống".
 */
export const attendanceSummarySchema = z.object({
  present: z.number().int(),
  late: z.number().int(),
  absent: z.number().int(),
  excused: z.number().int(),
});

export type AttendanceSummary = z.infer<typeof attendanceSummarySchema>;

/**
 * `sessions.SessionResponse` (`apps/api/internal/features/sessions/dto.go`).
 * `status` is `'planned' | 'held' | 'cancelled'`
 * (`docs/schema_design.sql:201`); `student_count` previews the roster size
 * attendance confirmation would cover, while `attendance_summary` is the
 * live confirmed split (null until first confirm).
 */
export const sessionSchema = z.object({
  id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  session_date: z.string(),
  start_time: z.string().nullable(),
  status: z.enum(["planned", "held", "cancelled"]),
  cancel_reason: z.string().nullable(),
  attendance_confirmed_at: z.string().nullable(),
  student_count: z.number().int(),
  attendance_summary: attendanceSummarySchema.nullable(),
  created_at: z.string(),
});

export type Session = z.infer<typeof sessionSchema>;

/**
 * `attendance.RowResponse` (`apps/api/internal/features/attendance/dto.go`).
 * `status` is one of the four marks (`present` = Đúng giờ, `late` = Muộn,
 * `absent` = Vắng, `excused` = Có lý do) or null when the session has never
 * been confirmed. All four statuses are billable — attendance never changes
 * what a family owes, only what the record shows.
 */
export const attendanceRowSchema = z.object({
  student_id: z.string(),
  student_name: z.string(),
  display_note: z.string().nullable(),
  enrollment_id: z.string(),
  status: z.enum(["present", "late", "absent", "excused"]).nullable(),
  billable: z.boolean(),
  note: z.string().nullable(),
});

export type AttendanceRow = z.infer<typeof attendanceRowSchema>;

/**
 * `attendance.Response` — flat, no nested `session`/`students`/`records`
 * objects: one row per student enrolled as of the session date, present
 * students included. `warning` is set only by confirm, only when the
 * session's date falls inside an already-closed period and the automatic
 * billing reconciliation failed after the attendance write itself succeeded.
 */
export const attendanceResponseSchema = z.object({
  session_id: z.string(),
  session_date: z.string(),
  status: z.enum(["planned", "held", "cancelled"]),
  attendance_confirmed_at: z.string().nullable(),
  rows: z.array(attendanceRowSchema),
  warning: z.string().nullable().optional(),
});

export type AttendanceResponse = z.infer<typeof attendanceResponseSchema>;

/**
 * `attendance.ConfirmMark` — one student's exception from the all-present
 * default. `present` is never sent: an unlisted roster student defaults to
 * it, which is what keeps a normal session at one tap.
 */
export const attendanceMarkSchema = z.object({
  student_id: z.string(),
  status: z.enum(["late", "absent", "excused"]),
  note: z.string().trim().max(500, "Tối đa 500 ký tự").optional(),
});

export type AttendanceMark = z.infer<typeof attendanceMarkSchema>;

/**
 * `attendance.ConfirmRequest`. `marks` deliberately has no `.min()` — an
 * empty array is the legitimate "everyone on time" case. (The server still
 * accepts the deprecated `absent_student_ids` body, but this client only
 * ever sends `marks`.)
 */
export const confirmAttendanceInputSchema = z.object({
  marks: z.array(attendanceMarkSchema),
  note: z.string().trim().max(500, "Tối đa 500 ký tự").optional(),
});

export type ConfirmAttendanceInput = z.infer<typeof confirmAttendanceInputSchema>;

/** `sessions.CancelRequest` — field is `reason`, free text up to 500 chars. */
export const cancelSessionInputSchema = z.object({
  reason: z.string().trim().min(1, "Bắt buộc nhập lý do").max(500, "Tối đa 500 ký tự"),
});

export type CancelSessionInput = z.infer<typeof cancelSessionInputSchema>;
