import {
  FileIcon,
  FileTextIcon,
  ImageIcon,
  LinkIcon,
  type LucideIcon,
  MusicIcon,
  RadioIcon,
  StickyNoteIcon,
  VideoIcon,
} from "lucide-react";

import type { HvBadgeVariant } from "@/components/hv";

import type {
  LessonMode,
  LogFieldKind,
  MaterialKind,
  ProgramTemplate,
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
  video: "Video",
  audio: "Âm thanh",
  image: "Hình ảnh",
  doc: "Tài liệu",
  note: "Ghi chú",
  live: "Buổi học trực tuyến",
  link: "Liên kết ngoài",
  other: "Khác",
};

export const logFieldKindLabel: Record<LogFieldKind, string> = {
  text: "Văn bản",
  long_text: "Đoạn dài",
  checkbox: "Tick",
  student: "Chọn học sinh",
  number: "Số",
  select: "Chọn một",
};

export const lessonModeLabel: Record<LessonMode, string> = {
  scheduled: "Buổi học có lịch",
  self_study: "Không lịch",
};

export const materialKindIcon: Record<MaterialKind, LucideIcon> = {
  video: VideoIcon,
  audio: MusicIcon,
  image: ImageIcon,
  doc: FileTextIcon,
  note: StickyNoteIcon,
  live: RadioIcon,
  link: LinkIcon,
  other: FileIcon,
};

const FORMAT_BY_EXTENSION: Record<string, string> = {
  pdf: "PDF",
  doc: "DOCX",
  docx: "DOCX",
  ppt: "PPTX",
  pptx: "PPTX",
  xls: "XLSX",
  xlsx: "XLSX",
  mp4: "MP4",
  mov: "MOV",
  mp3: "MP3",
  wav: "WAV",
  m4a: "M4A",
  png: "PNG",
  jpg: "JPG",
  jpeg: "JPG",
  gif: "GIF",
  webp: "WEBP",
};

/**
 * ĐỊNH DẠNG column of the content bank: derived from the URL alone (known
 * hosts first, then the file extension) because the API stores no format.
 */
export function materialFormatLabel(url: string | null | undefined): string {
  if (!url) return "—";
  let parsed: URL;
  try {
    parsed = new URL(url);
  } catch {
    return "Link";
  }
  const host = parsed.hostname.replace(/^www\./, "");
  if (host === "youtube.com" || host === "youtu.be" || host.endsWith(".youtube.com"))
    return "YouTube";
  if (host === "drive.google.com" || host === "docs.google.com") return "Drive";
  const ext = parsed.pathname.split(".").pop()?.toLowerCase() ?? "";
  return FORMAT_BY_EXTENSION[ext] ?? "Link";
}

/**
 * Hub card pill: a template is live once any version is published, a draft
 * while it only has drafts, and stopped when every version is archived.
 */
export function templateStatus(
  template: Pick<ProgramTemplate, "versions">,
): "active" | "draft" | "archived" {
  const statuses = template.versions.map((v) => v.status);
  if (statuses.includes("published")) return "active";
  if (statuses.includes("draft") || statuses.length === 0) return "draft";
  return "archived";
}

export const templateStatusLabel: Record<ReturnType<typeof templateStatus>, string> = {
  active: "Đang hoạt động",
  draft: "Bản nháp",
  archived: "Ngừng",
};

export function formatDifficulty(difficulty: number | null): string {
  return difficulty === null ? "—" : `Mức ${difficulty}`;
}
