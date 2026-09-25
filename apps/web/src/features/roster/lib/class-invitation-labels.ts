import type { HvBadgeVariant } from "@/components/hv";

import type { ClassInvitationStatus } from "../schemas/roster-schemas";

/**
 * Row badge copy per invitation status. The prototype knows four states
 * (chờ / nhận / từ chối / hủy); the API adds `accepted` — the invitee said
 * yes but the owner has not written the stint yet — which keeps its own
 * sky badge so the owner can tell it from a finished `assigned` row.
 */
export const invitationStatusLabel: Record<ClassInvitationStatus, string> = {
  pending: "Chờ xác nhận",
  accepted: "Đã đồng ý",
  declined: "Đã từ chối",
  cancelled: "Đã hủy",
  assigned: "Đã nhận lớp",
};

export const invitationStatusVariant: Record<ClassInvitationStatus, HvBadgeVariant> = {
  pending: "warning",
  accepted: "info",
  declined: "danger",
  cancelled: "neutral",
  assigned: "success",
};

/**
 * The filter strip's buckets, as in the prototype: cancelled rows only show
 * under "Tất cả". "Đã nhận" folds accepted and assigned together: to the
 * invitee both mean "I said yes", and the row badge still tells the owner
 * which of the two a row is in.
 */
export const invitationViews = ["all", "pending", "received", "declined"] as const;

export type InvitationView = (typeof invitationViews)[number];

export const invitationViewLabel: Record<InvitationView, string> = {
  all: "Tất cả",
  pending: "Chờ xác nhận",
  received: "Đã nhận",
  declined: "Từ chối",
};

export function invitationInView(status: ClassInvitationStatus, view: InvitationView): boolean {
  switch (view) {
    case "all":
      return true;
    case "received":
      return status === "accepted" || status === "assigned";
    default:
      return status === view;
  }
}
