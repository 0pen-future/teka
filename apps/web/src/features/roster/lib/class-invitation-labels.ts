import type { HvBadgeVariant } from "@/components/hv";

import type { ClassInvitationStatus } from "../schemas/roster-schemas";

/** Row badge copy per invitation status. */
export const invitationStatusLabel: Record<ClassInvitationStatus, string> = {
  pending: "Đang chờ",
  accepted: "Đã đồng ý",
  declined: "Đã từ chối",
  cancelled: "Đã hủy",
  assigned: "Đã phân công",
};

export const invitationStatusVariant: Record<ClassInvitationStatus, HvBadgeVariant> = {
  pending: "warning",
  accepted: "info",
  declined: "neutral",
  cancelled: "neutral",
  assigned: "success",
};

/**
 * The filter strip's buckets. "Đã nhận" folds accepted and assigned together:
 * to the invitee both mean "I said yes", and the row badge still tells the
 * owner which of the two a row is in.
 */
export const invitationViews = ["all", "pending", "received", "declined", "cancelled"] as const;

export type InvitationView = (typeof invitationViews)[number];

export const invitationViewLabel: Record<InvitationView, string> = {
  all: "Tất cả",
  pending: "Đang chờ",
  received: "Đã nhận",
  declined: "Đã từ chối",
  cancelled: "Đã hủy",
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
