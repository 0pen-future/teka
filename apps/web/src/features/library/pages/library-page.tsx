import { SearchIcon } from "lucide-react";
import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";
import { z } from "zod";

import {
  HvBadge,
  HvButton,
  HvSegmented,
  HvSelect,
  HvStateBlock,
  hvToast,
  type HvSegmentedOption,
} from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useCenterContext } from "@/features/teaching";
import { cn } from "@/lib/utils";

import { LessonsTable } from "../components/lessons-table";
import { TemplateDialog } from "../components/template-dialog";
import { useLessons, useTemplatesList, useVersions } from "../hooks/use-library";
import { defaultVersion, versionLabel, versionStatusVariant } from "../lib/library-labels";
import type { ProgramTemplate } from "../schemas/library-schemas";

const liveTabs = ["templates", "lessons"] as const;
type LiveTab = (typeof liveTabs)[number];

const tabSchema = z.enum(liveTabs).catch("templates");

const LATER_PHASE_HINT = "Có ở phase sau";

const tabOptions: HvSegmentedOption<string>[] = [
  { value: "templates", label: "Chương trình mẫu" },
  { value: "lessons", label: "Buổi học mẫu" },
  { value: "materials", label: "Học liệu", disabled: true, title: LATER_PHASE_HINT },
  { value: "homework", label: "Bài tập", disabled: true, title: LATER_PHASE_HINT },
];

const TAB_ID_BASE = "library";

/** `/library` — the center's program-template catalog and a browser for one version's lessons. */
export function LibraryPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tab: LiveTab = tabSchema.parse(searchParams.get("tab") ?? undefined);
  const { has, isResolved } = useCenterContext();
  const canEdit = has("library.edit");

  function selectTab(next: string) {
    const params = new URLSearchParams(searchParams);
    if (next === "templates") {
      params.delete("tab");
    } else {
      params.set("tab", next);
    }
    setSearchParams(params, { replace: true });
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Kho học liệu</h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Chương trình mẫu và buổi học mẫu dùng chung cho các lớp của trung tâm.
        </p>
      </div>

      <HvSegmented
        variant="tabs"
        idBase={TAB_ID_BASE}
        aria-label="Các mục của kho học liệu"
        options={tabOptions}
        value={tab}
        onValueChange={selectTab}
      />

      <div
        role="tabpanel"
        id={`${TAB_ID_BASE}-panel-${tab}`}
        aria-labelledby={`${TAB_ID_BASE}-tab-${tab}`}
      >
        {!isResolved ? (
          <HvStateBlock state="loading" title="Đang tải kho học liệu" />
        ) : tab === "templates" ? (
          <TemplatesTab canEdit={canEdit} />
        ) : (
          <LessonsTab />
        )}
      </div>
    </div>
  );
}

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

function subjectLine(template: ProgramTemplate): string {
  return [template.subject, template.level].filter(Boolean).join(" · ");
}

function TemplatesTab({ canEdit }: { canEdit: boolean }) {
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  const [creating, setCreating] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const list = useTemplatesList({ q: q || undefined, per_page: 100, sort: "name" });
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
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            Tạo chương trình mẫu
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
          title={q ? "Không có chương trình nào khớp từ khoá." : "Chưa có chương trình mẫu nào."}
          description={
            q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : canEdit
                ? "Tạo chương trình đầu tiên bằng nút Tạo chương trình mẫu."
                : "Người có quyền soạn sẽ thêm chương trình tại đây."
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[720px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Chương trình</th>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Môn · Trình độ</th>
                <th className={headCellClassName}>Phát hành</th>
                <th className={headCellClassName}>Bản nháp</th>
              </tr>
            </thead>
            <tbody>
              {templates.map((template) => (
                <tr key={template.id} className="transition-colors hover:bg-cream-100">
                  <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                    <Link to={`/library/templates/${template.id}`} className="hover:text-mint-600">
                      {template.name}
                    </Link>
                  </td>
                  <td className={cn(cellClassName, "font-mono text-[13px] text-ink-700")}>
                    {template.code}
                  </td>
                  <td className={cn(cellClassName, "text-ink-500")}>
                    {subjectLine(template) || "—"}
                  </td>
                  <td className={cellClassName}>
                    {template.published_version_no == null ? (
                      <span className="text-ink-400">Chưa phát hành</span>
                    ) : (
                      <HvBadge variant="success" size="sm" dot>
                        {versionLabel({
                          version_no: template.published_version_no,
                          status: "published",
                        })}
                      </HvBadge>
                    )}
                  </td>
                  <td className={cellClassName}>
                    {template.draft_version_no == null ? (
                      <span className="text-ink-400">—</span>
                    ) : (
                      <HvBadge variant="warning" size="sm" dot>
                        {versionLabel({ version_no: template.draft_version_no, status: "draft" })}
                      </HvBadge>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
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

/**
 * Browses the lessons of one template's working version: the open draft
 * when there is one, else the published version. Authoring happens on the
 * template page, so this tab is read-only by design.
 */
function LessonsTab() {
  const [templateId, setTemplateId] = useState("");
  const templates = useTemplatesList({ per_page: 100, sort: "name" });
  const versions = useVersions(templateId || undefined);
  const version = versions.data ? defaultVersion(versions.data) : undefined;
  const lessons = useLessons(version?.id);
  const options = (templates.data?.items ?? []).map((template) => ({
    value: template.id,
    label: template.name,
    meta: template.code,
  }));

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center gap-3">
        <HvSelect
          options={options}
          value={templateId}
          onValueChange={setTemplateId}
          sheetTitle="Chọn chương trình mẫu"
          placeholder="Chọn chương trình mẫu…"
          searchNoun="chương trình"
          aria-label="Chọn chương trình mẫu"
          className="min-w-[260px] max-sm:w-full"
        />
        {version ? (
          <HvBadge variant={versionStatusVariant[version.status]} dot>
            {versionLabel(version)}
          </HvBadge>
        ) : null}
      </div>

      {templateId === "" ? (
        <HvStateBlock
          state="empty"
          title="Chọn một chương trình mẫu để xem các buổi học."
          description="Tab này chỉ để xem; soạn buổi học trong trang của chương trình."
        />
      ) : versions.isPending || (version && lessons.isPending) ? (
        <HvStateBlock state="loading" title="Đang tải buổi học" />
      ) : versions.isError || lessons.isError ? (
        <HvStateBlock state="error" title="Không tải được buổi học" />
      ) : !version ? (
        <HvStateBlock state="empty" title="Chương trình này chưa có phiên bản nào." />
      ) : (lessons.data ?? []).length === 0 ? (
        <HvStateBlock state="empty" title="Phiên bản này chưa có buổi học nào." />
      ) : (
        <LessonsTable lessons={lessons.data ?? []} templateId={templateId} />
      )}
    </div>
  );
}
