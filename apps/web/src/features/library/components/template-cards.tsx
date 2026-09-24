import { SearchIcon } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate } from "react-router";

import {
  HvBadge,
  HvButton,
  HvCard,
  HvChip,
  HvStateBlock,
  hvToast,
  type HvBadgeVariant,
} from "@/components/hv";
import { Input } from "@/components/ui/input";

import { useTemplatesList } from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import { templateStatus, templateStatusLabel, versionStatusVariant } from "../lib/library-labels";
import type { ProgramTemplate } from "../schemas/library-schemas";
import { TemplateDialog } from "./template-dialog";

/**
 * `StatusPill` is typed to the billing statuses (`paid`/`partial`/`unpaid`)
 * and cannot represent a template's own status vocabulary, so this mirrors
 * `versionStatusVariant`'s established `HvBadge` variant map instead.
 */
const templateStatusVariant: Record<ReturnType<typeof templateStatus>, HvBadgeVariant> = {
  active: "success",
  draft: "warning",
  archived: "neutral",
};

function subjectLine(template: ProgramTemplate): string {
  return [template.subject, template.level].filter(Boolean).join(" · ");
}

function TemplateCard({ template, canEdit }: { template: ProgramTemplate; canEdit: boolean }) {
  const navigate = useNavigate();
  const [editing, setEditing] = useState(false);
  const status = templateStatus(template);
  const sortedVersions = [...template.versions].sort((a, b) => b.version_no - a.version_no);

  return (
    <HvCard role="article" aria-label={template.name} className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-3">
        <div>
          <Link
            to={`/library/templates/${template.id}`}
            className="font-display text-[17px] font-extrabold text-ink-900 hover:text-mint-600"
          >
            {template.name}
          </Link>
          {subjectLine(template) ? (
            <p className="mt-0.5 text-[13px] font-medium text-ink-500">{subjectLine(template)}</p>
          ) : null}
        </div>
        <HvBadge variant={templateStatusVariant[status]} size="sm" dot>
          {templateStatusLabel[status]}
        </HvBadge>
      </div>

      <p className="text-[13px] font-medium text-ink-500">
        {template.lesson_count} buổi mẫu · {template.version_count} phiên bản ·{" "}
        {template.class_count} lớp đang gắn
      </p>

      {sortedVersions.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {sortedVersions.map((version) => (
            <HvChip
              key={version.id}
              size="sm"
              dot={versionStatusVariant[version.status]}
              onClick={() => void navigate(`/library/templates/${template.id}?v=${version.id}`)}
            >
              v{version.version_no}
            </HvChip>
          ))}
        </div>
      ) : null}

      {canEdit ? (
        <div>
          <HvButton size="sm" variant="ghost" onClick={() => setEditing(true)}>
            Sửa
          </HvButton>
        </div>
      ) : null}

      {canEdit ? (
        <TemplateDialog
          mode="edit"
          template={template}
          open={editing}
          onOpenChange={setEditing}
          onSaved={() => hvToast(`Đã lưu ${template.code}`)}
        />
      ) : null}
    </HvCard>
  );
}

/**
 * `/library` — the program-template hub tab: a searchable card grid, each
 * card linking to the template's detail page and its own version chips.
 * Templates only reference the material/exercise banks, never copy them.
 */
export function TemplateCards({ canEdit }: { canEdit: boolean }) {
  const navigate = useNavigate();
  const search = useSearch();
  const [creating, setCreating] = useState(false);
  const list = useTemplatesList({ q: search.q || undefined, per_page: 100, sort: "name" });
  const templates = list.data?.items ?? [];

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-col gap-2 sm:flex-row sm:items-center">
        <div className="relative flex-1">
          <SearchIcon
            aria-hidden="true"
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
          />
          <Input
            type="search"
            aria-label="Tìm chương trình mẫu"
            placeholder="Tìm theo tên hoặc mã…"
            value={search.query}
            onChange={(event) => search.setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            + Chương trình mẫu
          </HvButton>
        ) : null}
      </div>

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải chương trình mẫu" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được kho học liệu"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : templates.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={
            search.q ? "Không có chương trình nào khớp từ khoá." : "Chưa có chương trình mẫu nào."
          }
          description={
            search.q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : canEdit
                ? "Tạo chương trình đầu tiên bằng nút + Chương trình mẫu."
                : "Người có quyền soạn sẽ thêm chương trình tại đây."
          }
        />
      ) : (
        <div className="grid gap-3 md:grid-cols-2">
          {templates.map((template) => (
            <TemplateCard key={template.id} template={template} canEdit={canEdit} />
          ))}
        </div>
      )}

      {canEdit ? (
        <TemplateDialog
          mode="create"
          open={creating}
          onOpenChange={setCreating}
          onCreated={(template) => {
            hvToast(`Đã tạo chương trình ${template.code}`);
            void navigate(`/library/templates/${template.id}`);
          }}
        />
      ) : null}
    </div>
  );
}
