import { z } from "zod";

/**
 * Vietnamese phone, accepting both local (`0xxxxxxxxx`) and E.164
 * (`+84xxxxxxxxx`) input forms — mirrors `vnPhonePattern`
 * (`apps/api/internal/shared/validation/validation.go`). The server
 * normalizes to E.164 on write; the client only needs to reject garbage
 * before it round-trips.
 */
const vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/;

/** `centers.MemberResponse` (`apps/api/internal/features/centers/dto.go`). */
export const centerMemberSchema = z.object({
  id: z.string(),
  full_name: z.string(),
  phone: z.string(),
  is_owner: z.boolean(),
});

export type CenterMember = z.infer<typeof centerMemberSchema>;

/**
 * `center.is_owner` is the caller's role in this center, not a property of
 * the center itself — the whole page role-gates on it.
 */
export const centerSchema = z.object({
  id: z.string(),
  name: z.string(),
  is_owner: z.boolean(),
});

/** `centers.MeResponse` — the one payload the page renders from. */
export const centerMeSchema = z.object({
  center: centerSchema,
  members: z.array(centerMemberSchema),
});

export type CenterMe = z.infer<typeof centerMeSchema>;

/** `centers.JoinRequest` — the target center is addressed by its owner's phone. */
export const joinCenterInputSchema = z.object({
  owner_phone: z
    .string()
    .trim()
    .min(1, "Bắt buộc nhập số điện thoại")
    .regex(vnPhonePattern, "Số điện thoại không hợp lệ"),
});

export type JoinCenterInput = z.infer<typeof joinCenterInputSchema>;

export const joinCenterResponseSchema = z.object({
  center_id: z.string(),
  joined_at: z.string(),
});

export type JoinCenterResponse = z.infer<typeof joinCenterResponseSchema>;

/** `centers.RenameRequest` — server caps at 255 (`binding:"max=255"`). */
export const renameCenterInputSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Vui lòng nhập tên trung tâm")
    .max(255, "Tên trung tâm tối đa 255 ký tự"),
});

export type RenameCenterInput = z.infer<typeof renameCenterInputSchema>;
