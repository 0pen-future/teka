import type { HvBadgeVariant } from "@/components/hv";

import type { PathStatus } from "../schemas/paths-schemas";

export const pathStatusLabel: Record<PathStatus, string> = {
  draft: "Nháp",
  active: "Đang hoạt động",
  archived: "Ngừng hoạt động",
};

export const pathStatusVariant: Record<PathStatus, HvBadgeVariant> = {
  draft: "warning",
  active: "success",
  archived: "neutral",
};
