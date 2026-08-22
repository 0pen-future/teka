import type { LessonPlanStatus } from "./teaching-store";

export const planStatusLabels: Record<LessonPlanStatus, string> = {
  none: "Chưa nộp",
  draft: "Bản nháp",
  pending: "Chờ duyệt",
  approved: "Đã duyệt",
  redo: "Cần sửa lại",
};
