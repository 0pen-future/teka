import { useCenter } from "@/features/center";

export interface CenterContext {
  /** Teaching-store namespace key; null until `GET /centers/me` resolves. */
  centerId: string | null;
  centerName: string | null;
  isOwner: boolean;
  /** True once /centers/me resolved — gate owner-only UI on this to avoid flicker. */
  isResolved: boolean;
  /** True when /centers/me failed after retries — callers must not blank forever. */
  isError: boolean;
}

/**
 * One place deriving center identity + role from the role-shaped
 * `GET /centers/me` (owner narrowing is `"members" in data` — the two bodies
 * share no discriminant field).
 *
 * `centerId` is the center NAME for both roles: the member shape exposes no
 * center id, and the name is the one role-independent value both `/centers/me`
 * shapes share. Teaching data itself is server-persisted and center-scoped by
 * the API; this value only feeds display and presence checks.
 */
export function useCenterContext(): CenterContext {
  const { data, isError } = useCenter();
  if (!data) {
    return { centerId: null, centerName: null, isOwner: false, isResolved: false, isError };
  }
  const centerName = "members" in data ? data.center.name : data.center_name;
  return {
    centerId: centerName,
    centerName,
    isOwner: "members" in data,
    isResolved: true,
    isError: false,
  };
}
