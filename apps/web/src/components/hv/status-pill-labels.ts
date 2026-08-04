export type StatusPillStatus = "paid" | "partial" | "unpaid";

/** Vietnamese labels for each collections status, reusable by screens/tests. */
export const statusPillLabels: Record<StatusPillStatus, string> = {
  paid: "Đã đóng",
  partial: "Đóng thiếu",
  unpaid: "Chưa đóng",
};
