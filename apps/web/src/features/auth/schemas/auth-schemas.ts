import { z } from "zod";

/**
 * Matches the API's `vnPhonePattern`
 * (`apps/api/internal/shared/validation/validation.go`) exactly — a local
 * `0…` or E.164 `+84…` prefix followed by a mobile carrier digit
 * (3/5/7/8/9) and 8 more digits. Kept in lockstep with the server so the
 * client never accepts a number the API would reject.
 */
const vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/;

/** Normalizes a local `0…` Vietnamese number to the E.164 form the API stores and expects on the wire. */
export function normalizePhone(phone: string): string {
  return phone.startsWith("0") ? `+84${phone.slice(1)}` : phone;
}

const phoneSchema = z
  .string()
  .min(1, "Vui lòng nhập số điện thoại")
  .regex(vnPhonePattern, "Số điện thoại không hợp lệ")
  .transform(normalizePhone);

export const loginSchema = z.object({
  phone: phoneSchema,
  password: z.string().min(1, "Vui lòng nhập mật khẩu"),
});

/** `auth.ForgotPasswordRequest` — the phone to send a reset link for. */
export const forgotPasswordInputSchema = z.object({
  phone: phoneSchema,
});

export type ForgotPasswordInput = z.infer<typeof forgotPasswordInputSchema>;

/**
 * `auth.ForgotPasswordResponse` — a byte-identical generic body regardless of
 * whether the phone matched an eligible account (anti-enumeration); the
 * message text itself is never shown verbatim, only its success is used to
 * drive the always-generic confirmation screen.
 */
export const forgotPasswordResponseSchema = z.object({
  message: z.string(),
});

/**
 * Client-only form shape for the reset page: `confirm_password` never
 * leaves the browser. `password` mirrors the API's `ResetPasswordRequest`
 * bound exactly so a client-side rejection never differs from the server's.
 */
export const resetPasswordFormSchema = z
  .object({
    password: z.string().min(8, "Mật khẩu tối thiểu 8 ký tự").max(72, "Mật khẩu tối đa 72 ký tự"),
    confirm_password: z.string(),
  })
  .refine((values) => values.password === values.confirm_password, {
    message: "Mật khẩu xác nhận không khớp",
    path: ["confirm_password"],
  });

export type ResetPasswordFormInput = z.infer<typeof resetPasswordFormSchema>;

/** `auth.ResetPasswordRequest` — the wire shape sent to the API. */
export interface ResetPasswordInput {
  token: string;
  password: string;
}

/** `teachers.TeacherResponse` — the authenticated teacher's profile. */
export const teacherSchema = z.object({
  id: z.string(),
  phone: z.string(),
  full_name: z.string(),
  timezone: z.string(),
  status: z.string(),
  created_at: z.string(),
});

/**
 * Wire shape returned by register, login, and refresh
 * (`auth.TokenResponse`, `apps/api/internal/features/auth/dto.go`). The
 * profile key is `teacher`, not `user` — the API embeds the full teacher
 * profile so no separate `/me` round-trip is needed after auth.
 */
export const sessionSchema = z.object({
  access_token: z.string(),
  token_type: z.string(),
  expires_in: z.number(),
  teacher: teacherSchema,
});

export type LoginInput = z.infer<typeof loginSchema>;
export type Teacher = z.infer<typeof teacherSchema>;
export type Session = z.infer<typeof sessionSchema>;
