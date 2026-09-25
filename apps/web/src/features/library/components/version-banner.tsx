import { HvButton, HvNotice } from "@/components/hv";

import type { TemplateVersion } from "../schemas/library-schemas";

interface VersionBannerProps {
  version: TemplateVersion;
  canPublish: boolean;
  canEdit: boolean;
  hasDraft: boolean;
  onPublish: () => void;
  onCreateDraft: () => void;
  onArchive: () => void;
}

/** Contextual status strip above the tabs: what the version's lock state means and what to do about it. */
export function VersionBanner({
  version,
  canPublish,
  canEdit,
  hasDraft,
  onPublish,
  onCreateDraft,
  onArchive,
}: VersionBannerProps) {
  if (version.status === "draft") {
    return (
      <HvNotice tone="info">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span>Bản nháp — chỉnh sửa tự do. Kích hoạt để lớp có thể gắn.</span>
          {canPublish ? (
            <HvButton size="sm" onClick={onPublish}>
              Kích hoạt
            </HvButton>
          ) : null}
        </div>
      </HvNotice>
    );
  }
  if (version.status === "published") {
    return (
      <HvNotice tone="success">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <span>
            Phiên bản đã phát hành ({version.class_count} lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản
            nháp mới.
          </span>
          <div className="flex flex-wrap gap-2">
            {canEdit && !hasDraft ? (
              <HvButton size="sm" variant="secondary" onClick={onCreateDraft}>
                Tạo bản nháp
              </HvButton>
            ) : null}
            {canEdit ? (
              <HvButton size="sm" variant="ghost" onClick={onArchive}>
                Ngừng
              </HvButton>
            ) : null}
          </div>
        </div>
      </HvNotice>
    );
  }
  return <HvNotice tone="warning">Phiên bản đã ngừng — chỉ xem.</HvNotice>;
}
