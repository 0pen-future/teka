import { hvToast, type HvBadgeVariant } from "@/components/hv";

import type { Course, CourseStatus } from "../schemas/courses-schemas";

/** Prototype vocabulary: a stopped course takes no new classes but keeps its history. */
export const courseStatusLabel: Record<CourseStatus, string> = {
  draft: "Nháp",
  active: "Đang hoạt động",
  archived: "Dừng hoạt động",
};

export const courseStatusVariant: Record<CourseStatus, HvBadgeVariant> = {
  draft: "warning",
  active: "success",
  archived: "neutral",
};

/** "Starters Core · v2", or "Chưa gắn" for a course without a default template. */
export function templateLabel(course: Course): string {
  const template = course.default_template;
  return template ? `${template.name} · v${template.version_no}` : "Chưa gắn";
}

export async function copyCourseCode(code: string) {
  try {
    await navigator.clipboard.writeText(code);
    hvToast(`Đã sao chép mã ${code}`);
  } catch {
    hvToast("Không sao chép được mã", { variant: "danger" });
  }
}
