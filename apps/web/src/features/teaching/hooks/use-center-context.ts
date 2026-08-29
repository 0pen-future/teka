import { useCenter } from "@/features/center";

export interface CenterContext {
  /** Teaching-store namespace key; null until `GET /centers/me` resolves. */
  centerId: string | null;
  centerName: string | null;
  isOwner: boolean;
  /** The member's own send-reports flag; always false for the owner (flag is member-only). */
  canSendReports: boolean;
  /** Mirrors the server's ReportsOversight(): owner or flagged member may create sends. */
  canRunSends: boolean;
  /** True once /centers/me resolved — gate owner-only UI on this to avoid flicker. */
  isResolved: boolean;
  /** True when /centers/me failed after retries — callers must not blank forever. */
  isError: boolean;
  /** The caller's effective permission keys from `/centers/me` (owner: full catalog). */
  permissions: string[];
  /** True when the effective set holds `key`; false until `/centers/me` resolves. */
  has: (key: string) => boolean;
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
    return {
      centerId: null,
      centerName: null,
      isOwner: false,
      canSendReports: false,
      canRunSends: false,
      isResolved: false,
      isError,
      permissions: [],
      has: () => false,
    };
  }
  const isOwner = "members" in data;
  const centerName = isOwner ? data.center.name : data.center_name;
  const canSendReports = isOwner ? false : data.can_send_reports;
  const permissions = data.permissions;
  return {
    centerId: centerName,
    centerName,
    isOwner,
    canSendReports,
    canRunSends: isOwner || canSendReports,
    isResolved: true,
    isError: false,
    permissions,
    // The server already folds the owner bypass into the array (an owner's
    // effective set is the whole catalog). The explicit owner short-circuit
    // covers a rollout skew where an older API omits `permissions` (schema
    // defaults it to []) — the owner must never lose owner-only surfaces.
    has: (key: string) => isOwner || permissions.includes(key),
  };
}
