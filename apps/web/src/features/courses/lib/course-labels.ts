import type { HvBadgeVariant } from "@/components/hv";

import type { CourseStatus } from "../schemas/courses-schemas";

export const courseStatusLabel: Record<CourseStatus, string> = {
  draft: "Đang soạn",
  active: "Đang mở",
  archived: "Ngừng tuyển",
};

export const courseStatusVariant: Record<CourseStatus, HvBadgeVariant> = {
  draft: "warning",
  active: "success",
  archived: "neutral",
};
