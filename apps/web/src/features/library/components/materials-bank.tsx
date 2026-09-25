import { SearchIcon } from "lucide-react";
import { useState } from "react";

import {
  HvBadge,
  HvButton,
  HvConfirmDialog,
  HvSegmented,
  HvStateBlock,
  hvToast,
} from "@/components/hv";
import type { HvSegmentedOption } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { useDeleteMaterial, useMaterialsList, useSetMaterialStatus } from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import { materialFormatLabel, materialKindIcon, materialKindLabel } from "../lib/library-labels";
import type { Material } from "../schemas/library-schemas";
import { MaterialDialog } from "./material-dialog";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/** "12 buổi · 3 CT mẫu" or "—" when nothing references the item yet. */
function usageLabel(row: { lesson_count: number; template_count: number }): string {
  return row.lesson_count === 0 ? "—" : `${row.lesson_count} buổi · ${row.template_count} CT mẫu`;
}

type StatusFilter = "all" | "active" | "inactive";

const statusFilterOptions: HvSegmentedOption<StatusFilter>[] = [
  { value: "all", label: "Tất cả" },
  { value: "active", label: "Hoạt động" },
  { value: "inactive", label: "Ngừng" },
];

function toActiveParam(filter: StatusFilter): boolean | undefined {
  if (filter === "all") return undefined;
  return filter === "active";
}

/**
 * The center's content bank: search, a status filter, and per-row edit,
 * stop/activate, and delete. Deletion stays refused (409) while an active
 * version's lesson still links the item — the row keeps the button visible
 * but disabled so the reason (usage count) is legible without a failed call.
 */
export function MaterialsBank({ canEdit }: { canEdit: boolean }) {
  const search = useSearch();
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const list = useMaterialsList({
    q: search.q || undefined,
    active: toActiveParam(statusFilter),
    per_page: 100,
    sort: "title",
  });
  const remove = useDeleteMaterial();
  const setStatus = useSetMaterialStatus();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Material | null>(null);
  const [deleting, setDeleting] = useState<Material | null>(null);
  const rows = list.data?.items ?? [];

  function toggleStatus(material: Material) {
    setStatus.mutate(
      { id: material.id, active: !material.active },
      {
        onSuccess: () => hvToast(material.active ? "Đã ngừng học liệu" : "Đã kích hoạt học liệu"),
        onError: (error) =>
          hvToast(apiMessage(error, "Không đổi được trạng thái học liệu."), { variant: "danger" }),
      },
    );
  }

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
            aria-label="Tìm học liệu"
            placeholder="Tìm theo tên…"
            value={search.query}
            onChange={(event) => search.setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        <HvSegmented
          aria-label="Lọc trạng thái học liệu"
          options={statusFilterOptions}
          value={statusFilter}
          onValueChange={setStatusFilter}
        />
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            Thêm học liệu
          </HvButton>
        ) : null}
      </div>

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải học liệu" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được học liệu"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : rows.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={search.q ? "Không có học liệu nào khớp từ khoá." : "Chưa có học liệu nào."}
          description={
            search.q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : canEdit
                ? "Thêm học liệu đầu tiên bằng nút Thêm học liệu."
                : "Người có quyền soạn sẽ thêm học liệu tại đây."
          }
        />
      ) : (
        <>
          <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
            <table className="w-full min-w-[860px] border-collapse text-left text-[14px]">
              <thead>
                <tr>
                  <th className={headCellClassName}>Loại</th>
                  <th className={headCellClassName}>Tiêu đề</th>
                  <th className={headCellClassName}>Định dạng</th>
                  <th className={headCellClassName}>Dùng trong</th>
                  <th className={headCellClassName}>Trạng thái</th>
                  {canEdit ? (
                    <th className={headCellClassName}>
                      <span className="sr-only">Thao tác</span>
                    </th>
                  ) : null}
                </tr>
              </thead>
              <tbody>
                {rows.map((material) => {
                  const Icon = materialKindIcon[material.kind];
                  return (
                    <tr key={material.id} className="transition-colors hover:bg-cream-100">
                      <td className={cellClassName}>
                        <span className="inline-flex items-center gap-1.5 text-ink-700">
                          <Icon className="size-4 text-ink-400" aria-hidden="true" />
                          {materialKindLabel[material.kind]}
                        </span>
                      </td>
                      <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                        {material.title}
                        {material.description ? (
                          <p className="mt-0.5 line-clamp-2 text-[13px] font-medium text-ink-500">
                            {material.description}
                          </p>
                        ) : null}
                      </td>
                      <td className={cn(cellClassName, "text-ink-700")}>
                        {materialFormatLabel(material.url)}
                      </td>
                      <td className={cn(cellClassName, "text-ink-500")}>{usageLabel(material)}</td>
                      <td className={cellClassName}>
                        <HvBadge variant={material.active ? "success" : "neutral"} size="sm" dot>
                          {material.active ? "Hoạt động" : "Ngừng"}
                        </HvBadge>
                      </td>
                      {canEdit ? (
                        <td className={cn(cellClassName, "whitespace-nowrap text-right")}>
                          <div className="inline-flex items-center gap-1">
                            <HvButton
                              type="button"
                              size="sm"
                              variant="ghost"
                              aria-label={`Sửa ${material.title}`}
                              onClick={() => setEditing(material)}
                            >
                              Sửa
                            </HvButton>
                            <HvButton
                              type="button"
                              size="sm"
                              variant="ghost"
                              aria-label={`${material.active ? "Ngừng" : "Kích hoạt"} ${material.title}`}
                              disabled={setStatus.isPending}
                              onClick={() => toggleStatus(material)}
                            >
                              {material.active ? "Ngừng" : "Kích hoạt"}
                            </HvButton>
                            <HvButton
                              type="button"
                              size="sm"
                              variant="ghost"
                              aria-label={`Xoá ${material.title}`}
                              disabled={material.lesson_count > 0}
                              title={
                                material.lesson_count > 0
                                  ? "Học liệu đang gắn vào buổi học mẫu, ngừng hoạt động thay vì xoá."
                                  : undefined
                              }
                              onClick={() => setDeleting(material)}
                            >
                              Xoá
                            </HvButton>
                          </div>
                        </td>
                      ) : null}
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
          <p className="text-[13px] text-ink-500">
            Không xóa cứng nội dung còn được phiên bản đang hoạt động tham chiếu — chỉ ngừng hoạt
            động. Cờ &quot;chia sẻ cho học viên&quot; nằm ở từng buổi, không nằm ở file.
          </p>
        </>
      )}

      {canEdit ? (
        <>
          <MaterialDialog
            mode="create"
            open={creating}
            onOpenChange={setCreating}
            onCreated={() => hvToast("Đã thêm học liệu")}
          />
          {editing ? (
            <MaterialDialog
              mode="edit"
              material={editing}
              open
              onOpenChange={(open) => {
                if (!open) setEditing(null);
              }}
              onSaved={() => hvToast("Đã lưu học liệu")}
            />
          ) : null}
          <HvConfirmDialog
            open={deleting != null}
            onOpenChange={(open) => {
              if (!open) setDeleting(null);
            }}
            title={`Xoá học liệu "${deleting?.title ?? ""}"?`}
            description="Học liệu đang gắn vào một buổi học mẫu sẽ không xoá được."
            confirmLabel="Xoá học liệu"
            tone="danger"
            pending={remove.isPending}
            onConfirm={() => {
              if (!deleting) return;
              remove.mutate(deleting.id, {
                onSuccess: () => {
                  setDeleting(null);
                  hvToast("Đã xoá học liệu");
                },
                onError: (error) => {
                  setDeleting(null);
                  hvToast(apiMessage(error, "Không xoá được học liệu."), { variant: "danger" });
                },
              });
            }}
          />
        </>
      ) : null}
    </div>
  );
}
