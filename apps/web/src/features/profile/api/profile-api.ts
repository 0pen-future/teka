import { teacherSchema, type Teacher } from "@/features/auth";
import { apiClient } from "@/lib/api/client";
import { parseData } from "@/lib/api/envelope";

/**
 * Mirrors the API's `UpdateProfileRequest` (`teachers/dto.go`): full_name and
 * timezone only — phone is the login identifier and never moves through this
 * endpoint.
 */
export interface UpdateMeInput {
  full_name: string;
  timezone: string;
}

export async function updateMe(input: UpdateMeInput): Promise<Teacher> {
  const res = await apiClient.put<unknown>("/me", input);
  return parseData(teacherSchema, res.data);
}
