import { z } from "zod";

/** `zalo.StatusResponse` (`apps/api/internal/features/zalo/dto.go`). */
export const zaloStatusSchema = z.object({
  linked: z.boolean(),
  display_name: z.string().optional(),
  status: z.enum(["linked", "expired"]).optional(),
  linked_at: z.string().optional(),
});

export type ZaloStatus = z.infer<typeof zaloStatusSchema>;

/**
 * One row of `GET /me/zalo/friends` (`zalo.FriendResponse`). `display_name`
 * is never empty — the server already falls back to the friend's profile name
 * when the teacher gave them no alias.
 */
export const zaloFriendSchema = z.object({
  user_id: z.string(),
  display_name: z.string(),
  avatar: z.string().optional(),
});

export type ZaloFriend = z.infer<typeof zaloFriendSchema>;

/**
 * One row of `POST /me/zalo/friends/match` (`zalo.FriendMatchResponse`).
 * `phone` echoes the phone exactly as the caller sent it — it is the join key
 * back to the contact — and every other field is absent on a miss.
 */
export const zaloFriendMatchSchema = z.object({
  phone: z.string(),
  matched: z.boolean(),
  user_id: z.string().optional(),
  display_name: z.string().optional(),
  zalo_name: z.string().optional(),
  avatar: z.string().optional(),
  is_friend: z.boolean(),
});

export type ZaloFriendMatch = z.infer<typeof zaloFriendMatchSchema>;

/** `zalo.LinkStartResponse` — the id every later poll carries. */
export const zaloLinkStartSchema = z.object({
  link_id: z.string(),
});

export type ZaloLinkStart = z.infer<typeof zaloLinkStartSchema>;

/**
 * Stages of one QR attempt, mirroring `zalo.LinkState`. `scanned` and
 * `confirmed` are separate from `qr_ready` because the teacher has to act on
 * their phone at that point and the UI says so.
 */
export const zaloLinkStateSchema = z.enum([
  "pending",
  "qr_ready",
  "scanned",
  "confirmed",
  "linked",
  "expired",
  "error",
]);

export type ZaloLinkState = z.infer<typeof zaloLinkStateSchema>;

/** `zalo.LinkStatusResponse`. `qr_png` is base64 for a `data:` URI. */
export const zaloLinkStatusSchema = z.object({
  state: zaloLinkStateSchema,
  qr_png: z.string().optional(),
  display_name: z.string().optional(),
  error_message: z.string().optional(),
});

export type ZaloLinkStatus = z.infer<typeof zaloLinkStatusSchema>;

/**
 * States the attempt cannot leave. Polling stops here — `scanned` and
 * `confirmed` are still in flight and must keep polling.
 */
export function isTerminalLinkState(state: ZaloLinkState | undefined): boolean {
  return state === "linked" || state === "expired" || state === "error";
}

/**
 * The consent text and the version string sent to the server live in one
 * constant so what the teacher read and what gets recorded against their
 * account cannot drift. Editing the wording means bumping the version.
 */
export const ZALO_CONSENT = {
  version: "2026-08-personal-v1",
  points: [
    "Teka đăng nhập vào Zalo cá nhân của bạn để gửi thông báo học phí thay bạn.",
    "Phiên đăng nhập được mã hoá và chỉ dùng để gửi tin nhắn bạn yêu cầu.",
    "Bạn có thể ngắt kết nối bất cứ lúc nào; Teka sẽ xoá phiên đăng nhập đã lưu.",
  ],
  checkboxLabel: "Tôi hiểu và đồng ý kết nối tài khoản Zalo cá nhân của tôi",
} as const;
