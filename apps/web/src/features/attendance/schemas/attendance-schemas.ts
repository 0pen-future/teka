import { z } from "zod";

/**
 * `sessions.SessionResponse` (`apps/api/internal/features/sessions/dto.go`).
 * `status` is `'planned' | 'held' | 'cancelled'`
 * (`docs/schema_design.sql:201`); `student_count` previews the roster size
 * attendance confirmation would cover, not a live present/absent split — the
 * session list has no confirmed-count fields, only the attendance sheet does.
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
  created_at: z.string(),
});

export type Session = z.infer<typeof sessionSchema>;

/**
 * `attendance.RowResponse` (`apps/api/internal/features/attendance/dto.go`).
 * `status` is `'present' | 'absent' | 'excused'` (`docs/schema_design.sql:233`)
 * or null when the session has never been confirmed. `excused` is parsed but
 * never offered as a UI choice — it is reserved for a later phase
 * (`docs/schema_design.sql:234`).
 */
export const attendanceRowSchema = z.object({
  student_id: z.string(),
  student_name: z.string(),
  display_note: z.string().nullable(),
  enrollment_id: z.string(),
  status: z.enum(["present", "absent", "excused"]).nullable(),
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
 * `attendance.ConfirmRequest`. `absent_student_ids` deliberately has no
 * `.min()` — an empty array is the legitimate "everyone present" case.
 */
export const confirmAttendanceInputSchema = z.object({
  absent_student_ids: z.array(z.string()),
  note: z.string().trim().max(500, "Tối đa 500 ký tự").optional(),
});

export type ConfirmAttendanceInput = z.infer<typeof confirmAttendanceInputSchema>;

/** `sessions.CancelRequest` — field is `reason`, free text up to 500 chars. */
export const cancelSessionInputSchema = z.object({
  reason: z.string().trim().min(1, "Bắt buộc nhập lý do").max(500, "Tối đa 500 ký tự"),
});

export type CancelSessionInput = z.infer<typeof cancelSessionInputSchema>;
