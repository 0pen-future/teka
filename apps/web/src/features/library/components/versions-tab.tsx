import { useState, type KeyboardEvent } from "react";
import { Link } from "react-router";

import { HvBadge, HvButton, HvConfirmDialog, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useArchiveVersion, usePublishVersion } from "../hooks/use-library";
import { versionStatusLabel, versionStatusVariant } from "../lib/library-labels";
import type { TemplateVersion } from "../schemas/library-schemas";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

interface VersionsTabProps {
  versions: TemplateVersion[];
  templateId: string;
  canEdit: boolean;
  canPublish: boolean;
  onSelect: (versionNo: number) => void;
}

/** Version history: every draft, published, and archived version of the program, oldest last. */
export function VersionsTab({
  versions,
  templateId,
  canEdit,
  canPublish,
  onSelect,
}: VersionsTabProps) {
  const publish = usePublishVersion(templateId);
  const archive = useArchiveVersion(templateId);
  const [archiving, setArchiving] = useState<TemplateVersion | null>(null);

  const ordered = [...versions].sort((a, b) => b.version_no - a.version_no);

  function requestSelect(versionNo: number) {
    onSelect(versionNo);
  }

  function handleKeyDown(event: KeyboardEvent<HTMLDivElement>, versionNo: number) {
    if (event.key === "Enter" || event.key === " ") {
      event.preventDefault();
      requestSelect(versionNo);
    }
  }

  return (
    <div className="flex flex-col gap-3">
      <ul className="flex flex-col rounded-[var(--radius-lg)] border border-line-200 bg-white">
        {ordered.map((version) => (
          <li key={version.id} className="border-t border-line-100 first:border-t-0">
            <div
              role="button"
              tabIndex={0}
              aria-label={`Chọn v${version.version_no}`}
              onClick={() => requestSelect(version.version_no)}
              onKeyDown={(event) => handleKeyDown(event, version.version_no)}
              className="flex cursor-pointer flex-col gap-2 px-4 py-3 hover:bg-cream-100 sm:flex-row sm:items-center sm:justify-between"
            >
              <div className="flex flex-wrap items-center gap-2">
                <span className="font-display text-[15px] font-extrabold text-ink-900">
                  v{version.version_no}
                </span>
                <HvBadge variant={versionStatusVariant[version.status]}>
                  {versionStatusLabel[version.status]}
                </HvBadge>
                {version.classes.length > 0 ? (
                  <span className="flex flex-wrap items-center gap-1">
                    {version.classes.map((klass) => (
                      <Link
                        key={klass.id}
                        to={`/classes/${klass.id}`}
                        onClick={(event) => event.stopPropagation()}
                        className="rounded-full border border-line-200 bg-cream-100 px-2 py-0.5 text-[12px] font-bold text-ink-700 hover:text-mint-600"
                      >
                        {klass.name}
                      </Link>
                    ))}
                  </span>
                ) : (
                  <span className="text-[13px] text-ink-400">Chưa có lớp nào gắn</span>
                )}
              </div>
              <div className="flex items-center gap-2 self-end sm:self-auto">
                {version.status === "draft" && canPublish ? (
                  <HvButton
                    type="button"
                    size="sm"
                    disabled={publish.isPending}
                    onClick={(event) => {
                      event.stopPropagation();
                      publish.mutate(version.id, {
                        onSuccess: () => hvToast(`Đã kích hoạt v${version.version_no}`),
                        onError: (error) =>
                          hvToast(apiMessage(error, "Không kích hoạt được phiên bản."), {
                            variant: "danger",
                          }),
                      });
                    }}
                  >
                    Kích hoạt
                  </HvButton>
                ) : null}
                {version.status === "published" && canEdit ? (
                  <HvButton
                    type="button"
                    size="sm"
                    variant="ghost"
                    disabled={archive.isPending}
                    onClick={(event) => {
                      event.stopPropagation();
                      setArchiving(version);
                    }}
                  >
                    Ngừng
                  </HvButton>
                ) : null}
              </div>
            </div>
          </li>
        ))}
      </ul>
      <HvConfirmDialog
        open={archiving != null}
        onOpenChange={(open) => {
          if (!open) setArchiving(null);
        }}
        title={`Ngừng phiên bản v${archiving?.version_no ?? ""}?`}
        description={
          archiving && archiving.class_count > 0
            ? `${archiving.class_count} lớp đang gắn phiên bản này sẽ không còn dùng được bản mới.`
            : "Phiên bản sẽ chuyển sang chỉ xem."
        }
        confirmLabel="Ngừng"
        tone="danger"
        pending={archive.isPending}
        onConfirm={() => {
          if (!archiving) return;
          archive.mutate(archiving.id, {
            onSuccess: () => {
              hvToast(`Đã ngừng v${archiving.version_no}`);
              setArchiving(null);
            },
            onError: (error) => {
              hvToast(apiMessage(error, "Không ngừng được phiên bản."), { variant: "danger" });
              setArchiving(null);
            },
          });
        }}
      />
    </div>
  );
}
