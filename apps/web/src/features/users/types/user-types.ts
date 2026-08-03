import type { z } from "zod";

import type {
  createUserSchema,
  updateUserSchema,
  userSchema,
  userSortKeys,
} from "../schemas/user-schemas";

export type User = z.infer<typeof userSchema>;
export type CreateUserInput = z.infer<typeof createUserSchema>;
export type UpdateUserInput = z.infer<typeof updateUserSchema>;
export type UserSort = (typeof userSortKeys)[number];

/** Query-string parameters for GET /users. */
export interface UsersListParams {
  page?: number;
  per_page?: number;
  sort?: UserSort;
  q?: string;
  role?: "admin" | "user";
}
