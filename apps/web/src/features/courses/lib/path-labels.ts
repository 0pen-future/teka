import type { HvBadgeVariant } from "@/components/hv";

import type { LearningPath, PathStatus } from "../schemas/paths-schemas";

export const pathStatusLabel: Record<PathStatus, string> = {
  draft: "Đang soạn",
  active: "Đang dùng",
  archived: "Ngừng dùng",
};

export const pathStatusVariant: Record<PathStatus, HvBadgeVariant> = {
  draft: "warning",
  active: "success",
  archived: "neutral",
};

/** Card subtitle: stage and distinct course counts, or a hint that the path is still empty. */
export function pathSummary(path: LearningPath): string {
  if (path.stage_count === 0) return "Chưa có giai đoạn";
  return `${path.stage_count} giai đoạn · ${path.course_count} khóa học`;
}
