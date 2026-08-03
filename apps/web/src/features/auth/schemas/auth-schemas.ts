import { z } from "zod";

import { userSchema } from "@/features/users";

export const loginSchema = z.object({
  email: z.email("Enter a valid email address"),
  password: z.string().min(1, "Password is required"),
});

/** Mirrors the API's RegisterRequest binding rules. */
export const registerSchema = z.object({
  name: z.string().min(1, "Name is required").max(255, "Name must be at most 255 characters"),
  email: z.email("Enter a valid email address"),
  password: z
    .string()
    .min(8, "Password must be at least 8 characters")
    .max(72, "Password must be at most 72 characters"),
});

/** Wire shape returned by register, login, and refresh. */
export const sessionSchema = z.object({
  access_token: z.string(),
  token_type: z.string(),
  expires_in: z.number(),
  user: userSchema,
});

export type LoginInput = z.infer<typeof loginSchema>;
export type RegisterInput = z.infer<typeof registerSchema>;
export type Session = z.infer<typeof sessionSchema>;
