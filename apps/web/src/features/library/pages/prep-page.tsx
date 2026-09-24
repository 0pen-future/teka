import { Link } from "react-router";

import { HvBadge, HvNotice, HvStateBlock, ProgressBar } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { cn } from "@/lib/utils";

import { useTemplatesList } from "../hooks/use-library";
import type { ProgramTemplate } from "../schemas/library-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";
const linkClassName = "font-display text-[13px] font-bold text-mint-600 hover:underline";
/** A route link dressed as the small primary button (HvButton renders a real button). */
const buttonLinkClassName = cn(
  "inline-flex min-h-[44px] items-center justify-center rounded-[var(--radius-md)] bg-mint-400 px-[18px]",
  "font-display text-[length:var(--text-sm)] font-bold text-white shadow-press-mint",
  "hover:brightness-[1.04] focus-visible:outline-none focus-visible:ring-4",
);

/**
 * `/prep` — every template with an open draft, with how far its lesson
 * preparation has come. Each row leads to the draft's board and, for a
 * holder of `prep.assign`, to the assignment page.
 */
export function PrepPage() {
  const { has, isResolved } = useCenterContext();
  const canRead = has("library.read");
  const canEdit = has("library.edit");
  const canAssign = has("prep.assign");
  const list = useTemplatesList(
    { has_draft: true, per_page: 100, sort: "name" },
    isResolved && canRead,
  );

  if (!isResolved) {
    return <HvStateBlock state="loading" title="Đang tải chuẩn bị tài liệu" />;
  }
  if (!canRead) {
    return <HvStateBlock state="error" title="Bạn không có quyền xem mục này" />;
  }

  const templates = (list.data?.items ?? []).filter((template) => template.draft_version_id);
  // The page fetches one API page (100 rows); a center with more open
  // drafts than that would otherwise see the tail silently disappear.
  const fetched = list.data?.items.length ?? 0;
  const total = list.data?.meta.total ?? 0;
  const truncated = fetched < total;

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">
            Chuẩn bị tài liệu
          </h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Tiến độ soạn tài liệu cho các bản nháp chương trình mẫu đang mở.
          </p>
        </div>
        {canEdit ? (
          <Link to="/library/templates/new" className={buttonLinkClassName}>
            Tạo chương trình
          </Link>
        ) : null}
      </div>

      {truncated ? (
        <HvNotice tone="warning">
          Đang hiển thị {fetched}/{total} bản nháp. Trang này chỉ tải tối đa {fetched} bản nháp mỗi
          lần; hoàn tất hoặc phát hành bớt bản nháp để thấy phần còn lại.
        </HvNotice>
      ) : null}

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải bản nháp" />
      ) : list.isError ? (
        <HvStateBlock state="error" title="Không tải được danh sách bản nháp" />
      ) : templates.length === 0 ? (
        <HvStateBlock
          state="empty"
          title="Chưa có bản nháp nào cần chuẩn bị"
          description="Tạo chương trình mẫu mới hoặc mở bản nháp từ kho học liệu."
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[640px] border-collapse text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Chương trình</th>
                <th className={headCellClassName}>Bản nháp</th>
                <th className={headCellClassName}>Tiến độ</th>
                <th className={headCellClassName}>Phụ trách</th>
                <th className={cn(headCellClassName, "text-right")}>Thao tác</th>
              </tr>
            </thead>
            <tbody>
              {templates.map((template) => (
                <PrepRow
                  key={template.id}
                  template={template}
                  canAssign={canAssign}
                  versionId={template.draft_version_id ?? ""}
                />
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

function PrepRow({
  template,
  versionId,
  canAssign,
}: {
  template: ProgramTemplate;
  versionId: string;
  canAssign: boolean;
}) {
  const lessonCount = template.prep?.lesson_count ?? 0;
  const doneCount = template.prep?.done_count ?? 0;
  const assignees = template.prep?.assignees ?? [];
  const percent = lessonCount === 0 ? 0 : Math.round((doneCount / lessonCount) * 100);

  return (
    <tr>
      <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
        <Link to={`/library/templates/${template.id}`} className="hover:underline">
          {template.name}
        </Link>
        <span className="ml-2 font-mono text-[12px] font-normal text-ink-500">{template.code}</span>
      </td>
      <td className={cellClassName}>
        <HvBadge variant="warning" dot>
          Nháp v{template.draft_version_no ?? "?"}
        </HvBadge>
      </td>
      <td className={cn(cellClassName, "min-w-[180px]")}>
        <ProgressBar
          value={percent}
          size="sm"
          color={percent === 100 ? "mint" : "viet"}
          label={`${doneCount}/${lessonCount} buổi`}
        />
      </td>
      <td className={cn(cellClassName, "text-ink-700")}>
        {assignees.length === 0 ? (
          <span className="text-ink-400">Chưa phân công</span>
        ) : (
          assignees.join(", ")
        )}
      </td>
      <td className={cn(cellClassName, "text-right whitespace-nowrap")}>
        <div className="flex justify-end gap-3">
          <Link to={`/prep/${versionId}/board`} className={linkClassName}>
            Bảng chuẩn bị
          </Link>
          {canAssign ? (
            <Link to={`/prep/${versionId}/assign`} className={linkClassName}>
              Phân công
            </Link>
          ) : null}
        </div>
      </td>
    </tr>
  );
}
