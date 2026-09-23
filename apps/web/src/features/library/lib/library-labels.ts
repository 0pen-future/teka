import type { HvBadgeVariant } from "@/components/hv";

import type {
  LogFieldKind,
  MaterialKind,
  TemplateVersion,
  TemplateVersionStatus,
} from "../schemas/library-schemas";

export const versionStatusLabel: Record<TemplateVersionStatus, string> = {
  draft: "Bản nháp",
  published: "Đã phát hành",
  archived: "Đã lưu trữ",
};

export const versionStatusVariant: Record<TemplateVersionStatus, HvBadgeVariant> = {
  draft: "warning",
  published: "success",
  archived: "neutral",
};

/** "v2 · Đã phát hành" — the one-line form used by pickers and summaries. */
export function versionLabel(version: Pick<TemplateVersion, "version_no" | "status">): string {
  return `v${version.version_no} · ${versionStatusLabel[version.status]}`;
}

/**
 * The version a reader lands on: the open draft when there is one (that is
 * where editing happens), else the published version, else the newest row.
 */
export function defaultVersion(versions: TemplateVersion[]): TemplateVersion | undefined {
  return (
    versions.find((v) => v.status === "draft") ??
    versions.find((v) => v.status === "published") ??
    [...versions].sort((a, b) => b.version_no - a.version_no)[0]
  );
}

export function formatDuration(minutes: number | null): string {
  return minutes === null ? "—" : `${minutes} phút`;
}

export const materialKindLabel: Record<MaterialKind, string> = {
  link: "Liên kết",
  doc: "Tài liệu",
  video: "Video",
  other: "Khác",
};

export const logFieldKindLabel: Record<LogFieldKind, string> = {
  text: "Văn bản",
  number: "Số",
  select: "Chọn một",
  checkbox: "Đánh dấu",
};

export function formatDifficulty(difficulty: number | null): string {
  return difficulty === null ? "—" : `Mức ${difficulty}`;
}
