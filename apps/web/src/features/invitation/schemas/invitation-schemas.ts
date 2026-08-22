import { z } from "zod";

/**
 * Vietnamese phone, accepting both local (`0xxxxxxxxx`) and E.164
 * (`+84xxxxxxxxx`) input forms — mirrors `vnPhonePattern`
 * (`apps/api/internal/shared/validation/validation.go`).
 */
const vnPhonePattern = /^(0|\+84)(3|5|7|8|9)\d{8}$/;

/**
 * `invitations.InvitationResponse` (`apps/api/internal/features/invitations/dto.go`).
 * `status` is derived server-side at read time from the stored row, never
 * trusted as a client-owned value.
 */
export const invitationSchema = z.object({
  id: z.string(),
  phone: z.string(),
  status: z.enum(["pending", "accepted", "revoked", "expired"]),
  expires_at: z.string(),
  created_at: z.string(),
});

export type Invitation = z.infer<typeof invitationSchema>;

/** `invitations.CreateRequest` — the phone to invite. */
export const createInviteInputSchema = z.object({
  phone: z
    .string()
    .trim()
    .min(1, "Vui lòng nhập số điện thoại")
    .regex(vnPhonePattern, "Số điện thoại không hợp lệ"),
});

export type CreateInviteInput = z.infer<typeof createInviteInputSchema>;

/**
 * `invitations.CreateResponse`. `link` is the full, ready-to-share URL — the
 * server builds it (`Service.buildLink`), so the client only ever displays or
 * copies this string verbatim. `dm_status` never blocks the create itself;
 * it just reports whether the best-effort Zalo delivery went out.
 */
export const createInviteResponseSchema = z.object({
  id: z.string(),
  phone: z.string(),
  expires_at: z.string(),
  link: z.string(),
  dm_status: z.enum(["sent", "skipped", "failed"]),
});

export type CreateInviteResponse = z.infer<typeof createInviteResponseSchema>;

/** `invitations.PreviewResponse` — `phone_masked` arrives pre-masked, no client-side masking needed. */
export const invitePreviewSchema = z.object({
  center_name: z.string(),
  phone_masked: z.string(),
});

export type InvitePreview = z.infer<typeof invitePreviewSchema>;

/**
 * Client-only form shape for the accept page: `confirm_password` never
 * leaves the browser, it just guards against a typo before the request goes
 * out. `full_name`/`password` mirror the API's `AcceptRequest` bounds
 * exactly so a client-side rejection never differs from what the server
 * would also reject.
 */
export const acceptInviteFormSchema = z
  .object({
    full_name: z.string().trim().min(1, "Vui lòng nhập họ tên").max(100, "Họ tên tối đa 100 ký tự"),
    password: z.string().min(8, "Mật khẩu tối thiểu 8 ký tự").max(72, "Mật khẩu tối đa 72 ký tự"),
    confirm_password: z.string(),
  })
  .refine((values) => values.password === values.confirm_password, {
    message: "Mật khẩu xác nhận không khớp",
    path: ["confirm_password"],
  });

export type AcceptInviteFormInput = z.infer<typeof acceptInviteFormSchema>;

/** `invitations.AcceptRequest` — the wire shape sent to the API. */
export interface AcceptInviteInput {
  token: string;
  full_name: string;
  password: string;
}
