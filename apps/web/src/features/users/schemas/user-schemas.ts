import { z } from "zod";

/** Wire shape of users.Response from the API. */
export const userSchema = z.object({
  id: z.uuid(),
  email: z.string(),
  name: z.string(),
  role: z.enum(["admin", "user"]),
  created_at: z.string(),
  updated_at: z.string(),
});

/** Mirrors the API's CreateRequest binding rules so errors show pre-flight. */
export const createUserSchema = z.object({
  email: z.email("Enter a valid email address"),
  password: z
    .string()
    .min(8, "Password must be at least 8 characters")
    .max(72, "Password must be at most 72 characters"),
  name: z.string().min(1, "Name is required").max(100, "Name must be at most 100 characters"),
  role: z.enum(["admin", "user"]),
});

/** PATCH is partial; only name and role are updatable (role by admins only). */
export const updateUserSchema = z.object({
  name: z.string().min(1, "Name is required").max(100, "Name must be at most 100 characters"),
  role: z.enum(["admin", "user"]),
});

/** Sort keys the API whitelists for GET /users. */
export const userSortKeys = [
  "created_at",
  "-created_at",
  "name",
  "-name",
  "email",
  "-email",
] as const;
