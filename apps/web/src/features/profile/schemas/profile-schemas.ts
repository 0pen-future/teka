import { z } from "zod";

/**
 * Only `full_name` persists — `PUT /me` carries full_name + timezone, and the
 * remaining prototype fields (môn dạy, ngân hàng) have no server columns yet.
 * They stay in the form so the Zalo-footer preview updates live, but reset to
 * empty on reload.
 */
export const profileFormSchema = z.object({
  full_name: z
    .string()
    .min(1, "Vui lòng nhập tên hiển thị")
    .max(100, "Tên hiển thị tối đa 100 ký tự"),
  subject: z.string(),
  bank: z.string(),
  account: z.string(),
  holder: z.string(),
});

export type ProfileFormInput = z.infer<typeof profileFormSchema>;
