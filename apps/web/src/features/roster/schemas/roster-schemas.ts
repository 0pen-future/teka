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
  contact_phone: z.string(),
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

/** `classes.ClassResponse`. `default_unit_price` is integer đồng, never a decimal. */
export const classSchema = z.object({
  id: z.string(),
  name: z.string(),
  start_date: z.string(),
  end_date: z.string().nullable(),
  default_unit_price: z.number().int(),
  status: z.enum(["active", "archived"]),
  schedules: z.array(scheduleSchema),
  created_at: z.string(),
});

export type Class = z.infer<typeof classSchema>;

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
});

export type ClassCreateInput = z.infer<typeof classCreateInputSchema>;

/** `classes.UpdateClassRequest` — schedules and status are separate endpoints. */
export const classUpdateInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên lớp").max(100, "Tối đa 100 ký tự"),
  start_date: dateField,
  end_date: z.union([dateField, z.literal("")]).optional(),
  default_unit_price: z.number().int().min(0, "Học phí không được âm"),
});

export type ClassUpdateInput = z.infer<typeof classUpdateInputSchema>;

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
});

export type ClassDialogInput = z.infer<typeof classDialogInputSchema>;

export function toClassCreateInput(values: ClassDialogInput): ClassCreateInput {
  const { slots, duration_min, ...rest } = values;
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
  return { ...rest, schedules };
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
