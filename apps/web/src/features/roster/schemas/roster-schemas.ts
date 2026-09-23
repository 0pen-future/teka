import { z } from "zod";

/**
 * Vietnamese phone, accepting both local (`0xxxxxxxxx`) and E.164
 * (`+84xxxxxxxxx`) input forms — mirrors `vnPhonePattern`
 * (`apps/api/internal/shared/validation/validation.go`). The server
 * normalizes to E.164 on write; the client only needs to reject garbage
 * before it round-trips.
 */
const vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/;

const phoneField = z
  .string()
  .trim()
  .min(1, "Bắt buộc nhập số điện thoại")
  .regex(vnPhonePattern, "Số điện thoại không hợp lệ");

/**
 * `contacts.ContactResponse` (`apps/api/internal/features/contacts/dto.go`).
 * The Zalo fields are `omitempty` server-side: an unmapped contact has no key
 * at all rather than `null`, hence `.optional()` and not `.nullable()`.
 */
export const contactSchema = z.object({
  id: z.string(),
  full_name: z.string(),
  phone: z.string(),
  student_count: z.number().int(),
  created_at: z.string(),
  zalo_user_id: z.string().optional(),
  zalo_name: z.string().optional(),
});

export type Contact = z.infer<typeof contactSchema>;

/**
 * `contacts.ZaloMappingRequest` — both values come straight from the picked
 * `GET /me/zalo/friends` row; the name is stored so lists render without
 * refetching the live friend list.
 */
export interface ZaloMappingInput {
  zalo_user_id: string;
  zalo_name: string;
}

/**
 * `contacts.CreateRequest` / `UpdateRequest` — full replace, no partial
 * update on a two-field resource. `full_name` caps at DB `VARCHAR(100)`
 * (`docs/schema_design.sql:107`).
 */
export const contactInputSchema = z.object({
  full_name: z.string().trim().min(1, "Bắt buộc nhập họ tên").max(100, "Tối đa 100 ký tự"),
  phone: phoneField,
});

export type ContactInput = z.infer<typeof contactInputSchema>;

/**
 * `students.StudentResponse` (`apps/api/internal/features/students/dto.go`).
 * The contact's name/phone are denormalized onto the row server-side — no
 * second call is needed to render the roster table.
 */
export const studentSchema = z.object({
  id: z.string(),
  full_name: z.string(),
  display_note: z.string(),
  contact_id: z.string(),
  contact_name: z.string(),
  // Null when the caller may not see the contact's phone (phone privacy:
  // owner, oversight, and assigned hoc_vu see it; other members do not).
  contact_phone: z.string().nullable(),
  created_at: z.string(),
});

export type Student = z.infer<typeof studentSchema>;

/**
 * `students.CreateRequest` / `UpdateRequest`. This is PRD R1's closed field
 * list: full name, owning contact, and the attendance-screen disambiguator —
 * nothing else. Do not add a field here without confirming it serves fee
 * calculation; anything else (age, grade, birth date, address, school,
 * photo) is a legal liability under Nghị định 13/2023 (PRD Q2). The dialog's
 * input-count test guards this from a future accidental extension.
 */
export const studentInputSchema = z.object({
  full_name: z.string().trim().min(1, "Bắt buộc nhập họ tên").max(100, "Tối đa 100 ký tự"),
  contact_id: z.string().min(1, "Bắt buộc chọn người liên hệ"),
  // No `.optional()`/`.default()` here: both would widen the schema's input
  // type to `string | undefined`, which zodResolver's generics can't
  // reconcile with a `useForm<StudentInput>` whose default value is already
  // `""` (see `toDefaultValues`). An unconstrained-but-required string
  // covers the same "empty is fine" case without the type mismatch.
  display_note: z.string().trim().max(50, "Tối đa 50 ký tự"),
});

export type StudentInput = z.infer<typeof studentInputSchema>;

/** `classes.ScheduleResponse` (`apps/api/internal/features/classes/dto.go`). */
export const scheduleSchema = z.object({
  id: z.string(),
  weekday: z.number().int().min(0).max(6),
  start_time: z.string(),
  duration_min: z.number().int(),
  effective_from: z.string(),
  effective_to: z.string().nullable(),
});

export type Schedule = z.infer<typeof scheduleSchema>;

const hhmmPattern = /^([01]\d|2[0-3]):[0-5]\d$/;
const dateField = z.string().regex(/^\d{4}-\d{2}-\d{2}$/, "Ngày phải theo định dạng YYYY-MM-DD");

/**
 * `classes.ScheduleRequest` — one weekly timetable row. Weekday 0 = Chủ nhật,
 * matching `class_schedules.weekday` (`docs/schema_design.sql:149`).
 */
export const scheduleInputSchema = z.object({
  weekday: z.number().int().min(0, "Chọn một ngày trong tuần").max(6),
  start_time: z.string().regex(hhmmPattern, "Giờ phải theo định dạng HH:MM"),
  duration_min: z.number().int().min(1, "Thời lượng phải lớn hơn 0"),
  effective_from: z.union([dateField, z.literal("")]).optional(),
  effective_to: z.union([dateField, z.literal("")]).optional(),
});

export type ScheduleInput = z.infer<typeof scheduleInputSchema>;

/**
 * One "khung giờ" as the class-timetable forms edit it (prototype
 * `modalClass.slots` / `classCfg.slots`): a shared start time plus every
 * weekday it repeats on. The wire shape stays one `ScheduleRequest` row per
 * (weekday, time) pair — see `toClassCreateInput` and `diffSchedules`
 * (`../lib/schedule-diff.ts`).
 */
export const scheduleSlotInputSchema = z.object({
  start_time: z.string().regex(hhmmPattern, "Giờ phải theo định dạng HH:MM"),
  days: z
    .array(z.number().int().min(0).max(6))
    .min(1, "Mỗi khung giờ cần ít nhất một ngày trong tuần"),
});

export type ScheduleSlotInput = z.infer<typeof scheduleSlotInputSchema>;

/**
 * The khung-giờ list both class forms share. A weekday may appear in only
 * one slot: the session generator materializes at most one session per class
 * per calendar date (`uq_class_sessions_per_day`,
 * `apps/api/migrations/000001_baseline_schema.up.sql`) and matches rows by
 * weekday alone, so a second row on the same weekday would be written but
 * silently never generate its session — under-billing with no error.
 */
const classSlotsField = z
  .array(scheduleSlotInputSchema)
  .min(1, "Thêm ít nhất một khung giờ")
  .superRefine((slots, ctx) => {
    const seen = new Set<number>();
    slots.forEach((slot, index) => {
      if (slot.days.some((day) => seen.has(day))) {
        ctx.addIssue({
          code: "custom",
          path: [index, "days"],
          message: "Ngày này đã có ở khung giờ khác — mỗi ngày chỉ một khung giờ",
        });
      }
      for (const day of slot.days) {
        seen.add(day);
      }
    });
  });

/** The lifecycle bucket the API assigns a class; the list filter and stats use the same values. */
export const classPhases = ["upcoming", "running", "ended", "archived"] as const;
export const classPhaseSchema = z.enum(classPhases);
export type ClassPhase = z.infer<typeof classPhaseSchema>;

/** The course chip embedded in a class: enough to label it and link to `/courses/:id`. */
export const courseRefSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
});

export type CourseRef = z.infer<typeof courseRefSchema>;

/**
 * The slice of `courses.CourseResponse` the class dialog's picker needs.
 * Roster owns this lookup so it never imports the courses feature (which
 * imports roster for the class list on the course page).
 */
export const courseOptionSchema = z.object({
  id: z.string(),
  code: z.string(),
  name: z.string(),
  default_unit_price: z.number().int(),
});

export type CourseOption = z.infer<typeof courseOptionSchema>;

/**
 * `classes.ClassResponse`. `default_unit_price` is integer đồng, never a
 * decimal. `my_staff_roles` is the caller's own active class-staff role keys
 * (e.g. `["giao_vien"]` for the class teacher, `[]` for the center owner or
 * an unassigned caller); dashboard endpoints reuse the same mapper but always
 * send `[]`, and some cached/older responses omit the field entirely, so it
 * defaults rather than requires.
 */
export const classSchema = z.object({
  id: z.string(),
  name: z.string(),
  teacher_id: z.string(),
  start_date: z.string(),
  end_date: z.string().nullable(),
  default_unit_price: z.number().int(),
  status: z.enum(["active", "archived"]),
  schedules: z.array(scheduleSchema),
  created_at: z.string(),
  my_staff_roles: z.array(z.string()).default([]),
  /** Open enrollments on the class — what `GET /enrollments?active=true` lists. */
  student_count: z.number().int().nonnegative().default(0),
  // Catalog fields. They default rather than require so a cached or older
  // response that predates them still parses.
  /** Display code (mã lớp), unique per center among live classes. */
  code: z.string().default(""),
  tags: z.array(z.string()).default([]),
  /** Still taking enrolments (cần tuyển sinh). */
  recruiting: z.boolean().default(false),
  /** Operational note shown on the class detail; null when none. */
  note: z.string().nullable().default(null),
  /**
   * Derived server-side from status and the dates against today
   * (`classes.PhaseOf`); the client only renders it and never recomputes it.
   */
  phase: classPhaseSchema.default("running"),
  /** The course the class is attached to (`classes.CourseRefResponse`); null when none. */
  course: courseRefSchema.nullable().default(null),
});

export type Class = z.infer<typeof classSchema>;

/** `classes.ClassStatsResponse` (`GET /classes/stats`): counts within the caller's read scope. */
export const classStatsSchema = z.object({
  all: z.number().int().nonnegative(),
  upcoming: z.number().int().nonnegative(),
  running: z.number().int().nonnegative(),
  ended: z.number().int().nonnegative(),
  archived: z.number().int().nonnegative(),
  recruiting: z.number().int().nonnegative(),
});

export type ClassStats = z.infer<typeof classStatsSchema>;

/**
 * `classes.CreateClassRequest` — schedules are required atomically; a class
 * with no timetable generates no sessions.
 */
export const classCreateInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên lớp").max(100, "Tối đa 100 ký tự"),
  start_date: dateField,
  end_date: z.union([dateField, z.literal("")]).optional(),
  default_unit_price: z.number().int().min(0, "Học phí không được âm"),
  schedules: z.array(scheduleInputSchema).min(1, "Chọn ít nhất một buổi trong tuần"),
  /** A live course of the center; absent means no course. */
  course_id: z.string().optional(),
});

export type ClassCreateInput = z.infer<typeof classCreateInputSchema>;

const classCodeField = z
  .string()
  .trim()
  .toUpperCase()
  .regex(/^[A-Z0-9-]{2,20}$/, "Mã lớp gồm 2–20 ký tự chữ, số hoặc gạch ngang");

const classTagsField = z
  .array(z.string().trim().min(1).max(30, "Mỗi thẻ tối đa 30 ký tự"))
  .max(10, "Tối đa 10 thẻ");

/**
 * `classes.UpdateClassRequest` — schedules and status are separate endpoints.
 * The catalog fields are pointers server-side: a key left out means "keep",
 * so callers send only what they changed (see `toClassUpdateInput`).
 */
export const classUpdateInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên lớp").max(100, "Tối đa 100 ký tự"),
  start_date: dateField,
  end_date: z.union([dateField, z.literal("")]).optional(),
  default_unit_price: z.number().int().min(0, "Học phí không được âm"),
  code: classCodeField.optional(),
  tags: classTagsField.optional(),
  recruiting: z.boolean().optional(),
  /** Replaces the stored note; `""` clears it. */
  note: z.string().trim().max(1000, "Tối đa 1000 ký tự").optional(),
  /** Same patch rule: absent keeps the course, `""` detaches, an id attaches. */
  course_id: z.string().optional(),
});

export type ClassUpdateInput = z.infer<typeof classUpdateInputSchema>;

/**
 * Form shape of the operational card on the class detail ("Thông tin vận
 * hành"): the recruiting flag, the tag list and the note. Dates and price
 * stay on the settings screen.
 */
export const classOpsInputSchema = z.object({
  recruiting: z.boolean(),
  tags: classTagsField,
  note: z.string().trim().max(1000, "Tối đa 1000 ký tự"),
});

export type ClassOpsInput = z.infer<typeof classOpsInputSchema>;

/**
 * Builds the `PUT /classes/:id` body for an operational edit: the required
 * base fields copied from the class unchanged, plus only the catalog fields
 * whose value differs from what the class already holds. Sending an
 * unchanged `tags` would still be harmless, but sending an unchanged `code`
 * from a stale read could clobber a rename made elsewhere — hence the diff.
 */
export function toClassUpdateInput(klass: Class, values: Partial<ClassOpsInput>): ClassUpdateInput {
  const input: ClassUpdateInput = {
    name: klass.name,
    start_date: klass.start_date,
    end_date: klass.end_date ?? "",
    default_unit_price: klass.default_unit_price,
  };
  if (values.recruiting !== undefined && values.recruiting !== klass.recruiting) {
    input.recruiting = values.recruiting;
  }
  if (values.tags !== undefined && !sameTags(values.tags, klass.tags)) {
    input.tags = values.tags;
  }
  if (values.note !== undefined && values.note !== (klass.note ?? "")) {
    input.note = values.note;
  }
  return input;
}

function sameTags(a: string[], b: string[]): boolean {
  return a.length === b.length && a.every((tag, index) => tag === b[index]);
}

/**
 * `handoff.ReassignResponse` (`PUT /classes/:id/teacher`). `teacher_id` is the
 * class's new owner; `moved_planned_sessions` is 0 on an idempotent no-op.
 */
export const reassignTeacherResponseSchema = z.object({
  class_id: z.string(),
  teacher_id: z.string(),
  moved_planned_sessions: z.number().int(),
});

export type ReassignTeacherResponse = z.infer<typeof reassignTeacherResponseSchema>;

/**
 * `classstaff.StaffResponse` (`apps/api/internal/features/classstaff/dto.go`).
 * `role_key` mirrors `authctx.StaffRoleGiaoVien/HocVu/TroGiang`. `role_label`
 * is the API's copy for a live row; the UI falls back to a local Vietnamese
 * label map only where no row exists yet (missing-role badges, empty-group
 * copy, add/remove action text) since there is nothing to read a label from.
 * `ended_at` non-null marks a soft-closed stint that still grants history reads.
 */
export const classStaffSchema = z.object({
  id: z.string(),
  teacher_id: z.string(),
  teacher_name: z.string(),
  role_key: z.string(),
  role_label: z.string(),
  started_at: z.string(),
  ended_at: z.string().nullable(),
});

export type ClassStaff = z.infer<typeof classStaffSchema>;

/**
 * Assignable class-staff roles — `giao_vien` is deliberately excluded: the
 * handoff flow owns it and the API refuses it with 409.
 */
export const assignableStaffRoleKeys = ["hoc_vu", "tro_giang"] as const;

export type AssignableStaffRoleKey = (typeof assignableStaffRoleKeys)[number];

/** `classstaff.AssignRequest`. */
export const classStaffAssignInputSchema = z.object({
  teacher_id: z.string().min(1, "Bắt buộc chọn thành viên"),
  role_key: z.enum(assignableStaffRoleKeys),
});

export type ClassStaffAssignInput = z.infer<typeof classStaffAssignInputSchema>;

/**
 * Form shape for the "Cài đặt lớp" screen (prototype `classCfg`): one name,
 * a list of khung-giờ slots, one unit price. The screen fans this out into
 * `PUT /classes/:id` plus schedule add/delete calls — see `diffSchedules`
 * (`../lib/schedule-diff.ts`). Unlike `classUpdateInputSchema`, price must
 * be positive here: the prototype's onSave rejects a zero rate with
 * "Nhập đơn giá mỗi buổi".
 */
export const classSettingsInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên lớp").max(100, "Tối đa 100 ký tự"),
  slots: classSlotsField,
  default_unit_price: z.number().int().min(1, "Nhập đơn giá mỗi buổi"),
});

export type ClassSettingsInput = z.infer<typeof classSettingsInputSchema>;

/**
 * Form shape for `ClassDialog`'s create mode: the class fields plus the
 * khung-giờ slot list from the `modalClass` Design Spec. Duration is one
 * shared field (the prototype omits it entirely; a per-slot input would only
 * add noise) applied to every generated row. `toClassCreateInput`
 * reassembles the wire shape that `POST /classes` expects.
 */
export const classDialogInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên lớp").max(100, "Tối đa 100 ký tự"),
  start_date: dateField,
  end_date: z.union([dateField, z.literal("")]).optional(),
  default_unit_price: z.number().int().min(0, "Học phí không được âm"),
  slots: classSlotsField,
  duration_min: z.number().int().min(1, "Thời lượng phải lớn hơn 0"),
  /** Picked course id; `""` means the class stays unattached. */
  course_id: z.string(),
});

export type ClassDialogInput = z.infer<typeof classDialogInputSchema>;

export function toClassCreateInput(values: ClassDialogInput): ClassCreateInput {
  const { slots, duration_min, course_id, ...rest } = values;
  // One wire row per (weekday, time) pair; two slots naming the same pair
  // would otherwise duplicate a session generator server-side.
  const seen = new Set<string>();
  const schedules: ScheduleInput[] = [];
  for (const slot of slots) {
    for (const weekday of slot.days) {
      const key = `${weekday}|${slot.start_time}`;
      if (seen.has(key)) continue;
      seen.add(key);
      schedules.push({
        weekday,
        start_time: slot.start_time,
        duration_min,
        effective_from: values.start_date,
      });
    }
  }
  return course_id === "" ? { ...rest, schedules } : { ...rest, schedules, course_id };
}

/** `enrollments.EnrollmentResponse`. `unit_price` is integer đồng. */
export const enrollmentSchema = z.object({
  id: z.string(),
  student_id: z.string(),
  student_name: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  started_on: z.string(),
  ended_on: z.string().nullable(),
  unit_price: z.number().int(),
  created_at: z.string(),
});

export type Enrollment = z.infer<typeof enrollmentSchema>;

/**
 * `enrollments.PickerStudent` — the enrollable-student autocomplete row.
 * Names only by design: the picker serves teachers who may not see contact
 * data, so no phone or contact id ever travels through it.
 */
export const enrollableStudentSchema = z.object({
  id: z.string(),
  full_name: z.string(),
});

export type EnrollableStudent = z.infer<typeof enrollableStudentSchema>;

/**
 * `enrollments.CreateRequest`. `unit_price` is deliberately absent — the
 * server copies it from `classes.default_unit_price`, enforcing PRD section
 * 4's single V1 pricing model.
 */
export const enrollmentCreateInputSchema = z.object({
  student_id: z.string().min(1, "Bắt buộc chọn học sinh"),
  class_id: z.string().min(1, "Bắt buộc chọn lớp"),
  started_on: z.union([dateField, z.literal("")]).optional(),
});

export type EnrollmentCreateInput = z.infer<typeof enrollmentCreateInputSchema>;

/**
 * `enrollments.EndRequest`. `startedOn` guards `ended_on >= started_on`
 * client-side before the request ever reaches the API.
 */
export function endEnrollmentInputSchema(startedOn: string) {
  return z.object({
    ended_on: z
      .union([dateField, z.literal("")])
      .optional()
      .refine(
        (value) => !value || value >= startedOn,
        "Ngày kết thúc phải từ ngày nhập học trở đi",
      ),
  });
}

export type EndEnrollmentInput = z.infer<ReturnType<typeof endEnrollmentInputSchema>>;

/**
 * `classinvites.InvitationResponse` (`apps/api/internal/features/classinvites/dto.go`).
 * An invitation moves pending → accepted/declined (invitee) → assigned
 * (owner confirm writes the stint) or cancelled (owner, or the member left
 * the center). `role_label` is the API's Vietnamese copy of `role_key`.
 */
export const classInvitationStatuses = [
  "pending",
  "accepted",
  "declined",
  "cancelled",
  "assigned",
] as const;

export type ClassInvitationStatus = (typeof classInvitationStatuses)[number];

export const classInvitationSchema = z.object({
  id: z.string(),
  class_id: z.string(),
  class_name: z.string(),
  teacher_id: z.string(),
  teacher_name: z.string(),
  role_key: z.string(),
  role_label: z.string(),
  status: z.enum(classInvitationStatuses),
  invited_by: z.string(),
  invited_by_name: z.string(),
  message: z.string().nullable(),
  sent_at: z.string(),
  reminded_at: z.string().nullable(),
  responded_at: z.string().nullable(),
  assigned_at: z.string().nullable(),
});

export type ClassInvitation = z.infer<typeof classInvitationSchema>;

/**
 * `classinvites.ConfirmResponse` — the assigned invitation plus what the
 * stint write moved: a giao_vien confirm is a handoff and carries the
 * class's future planned sessions to the new teacher.
 */
export const classInvitationConfirmSchema = classInvitationSchema.extend({
  moved_planned_sessions: z.number().int(),
});

export type ClassInvitationConfirm = z.infer<typeof classInvitationConfirmSchema>;

/** Roles an invitation may propose — unlike direct staff assignment, giao_vien is allowed. */
export const invitableStaffRoleKeys = ["giao_vien", "tro_giang", "hoc_vu"] as const;

export type InvitableStaffRoleKey = (typeof invitableStaffRoleKeys)[number];

/** `classinvites.SendRequest` (`POST /classes/:id/invitations`). */
export const classInvitationSendInputSchema = z.object({
  teacher_id: z.string().min(1, "Bắt buộc chọn thành viên"),
  role_key: z.enum(invitableStaffRoleKeys),
  message: z.string().trim().max(500, "Tối đa 500 ký tự").optional(),
});

export type ClassInvitationSendInput = z.infer<typeof classInvitationSendInputSchema>;
