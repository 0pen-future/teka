import { z } from "zod";

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

/**
 * `centers.MeResponse` — the owner's read model: the full center plus its
 * member roster.
 */
export const centerMeOwnerSchema = z.object({
  center: centerSchema,
  members: z.array(centerMemberSchema),
});

export type CenterMeOwner = z.infer<typeof centerMeOwnerSchema>;

/**
 * `centers.MemberMeResponse` — a non-owner's read model: the roster is
 * owner-only data, so a member sees only the center's name.
 */
export const centerMeMemberSchema = z.object({
  center_name: z.string(),
});

export type CenterMeMember = z.infer<typeof centerMeMemberSchema>;

/**
 * `GET /centers/me` is role-shaped: the two response bodies share no
 * discriminant field, so callers narrow on `"members" in data` (present only
 * on the owner shape) rather than reading a role flag.
 */
export const centerMeSchema = z.union([centerMeOwnerSchema, centerMeMemberSchema]);

export type CenterMe = z.infer<typeof centerMeSchema>;

/** `centers.RenameRequest` — server caps at 255 (`binding:"max=255"`). */
export const renameCenterInputSchema = z.object({
  name: z
    .string()
    .trim()
    .min(1, "Vui lòng nhập tên trung tâm")
    .max(255, "Tên trung tâm tối đa 255 ký tự"),
});

export type RenameCenterInput = z.infer<typeof renameCenterInputSchema>;
