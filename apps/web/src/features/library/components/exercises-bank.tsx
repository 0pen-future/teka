import { SearchIcon } from "lucide-react";
import { useState } from "react";
import { useSearchParams } from "react-router";

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

import { useDeleteExercise, useExercisesList, useSetExerciseStatus } from "../hooks/use-library";
import { useSearch } from "../hooks/use-search";
import type { Exercise } from "../schemas/library-schemas";
import { CopyCodeButton } from "./copy-code-button";
import { ExerciseDialog } from "./exercise-dialog";

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
 * The center's exercise bank, same shape as the material bank. Reads an
 * initial `?q=` so the lesson page's "Tìm bài tập theo mã" link
 * (`/library/exercises?q={code}`) lands with the search already filled.
 */
export function ExercisesBank({ canEdit }: { canEdit: boolean }) {
  const [searchParams] = useSearchParams();
  const search = useSearch(searchParams.get("q") ?? "");
  const [statusFilter, setStatusFilter] = useState<StatusFilter>("all");
  const list = useExercisesList({
    q: search.q || undefined,
    active: toActiveParam(statusFilter),
    per_page: 100,
    sort: "title",
  });
  const remove = useDeleteExercise();
  const setStatus = useSetExerciseStatus();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Exercise | null>(null);
  const [deleting, setDeleting] = useState<Exercise | null>(null);
  const rows = list.data?.items ?? [];

  function toggleStatus(exercise: Exercise) {
    setStatus.mutate(
      { id: exercise.id, active: !exercise.active },
      {
        onSuccess: () => hvToast(exercise.active ? "Đã ngừng bài tập" : "Đã kích hoạt bài tập"),
        onError: (error) =>
          hvToast(apiMessage(error, "Không đổi được trạng thái bài tập."), { variant: "danger" }),
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
            aria-label="Tìm bài tập"
            placeholder="Tìm theo tên hoặc mã…"
            value={search.query}
            onChange={(event) => search.setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        <HvSegmented
          aria-label="Lọc trạng thái bài tập"
          options={statusFilterOptions}
          value={statusFilter}
          onValueChange={setStatusFilter}
        />
        {canEdit ? (
          <HvButton size="sm" onClick={() => setCreating(true)}>
            Thêm bài tập
          </HvButton>
        ) : null}
      </div>

      {list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải bài tập" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được bài tập"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : rows.length === 0 ? (
        <HvStateBlock
          state="empty"
          title={search.q ? "Không có bài tập nào khớp từ khoá." : "Chưa có bài tập nào."}
          description={
            search.q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : canEdit
                ? "Thêm bài tập đầu tiên bằng nút Thêm bài tập."
                : "Người có quyền soạn sẽ thêm bài tập tại đây."
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[860px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Mã</th>
                <th className={headCellClassName}>Bài tập</th>
                <th className={headCellClassName}>Kỹ năng</th>
                <th className={headCellClassName}>Cấp độ</th>
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
              {rows.map((exercise) => (
                <tr key={exercise.id} className="transition-colors hover:bg-cream-100">
                  <td className={cellClassName}>
                    <CopyCodeButton code={exercise.code} />
                  </td>
                  <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                    {exercise.title}
                    <TagsRow tags={exercise.tags} />
                  </td>
                  <td className={cn(cellClassName, "text-ink-700")}>{exercise.skill ?? "—"}</td>
                  <td className={cn(cellClassName, "text-ink-700")}>{exercise.level ?? "—"}</td>
                  <td className={cn(cellClassName, "text-ink-500")}>{usageLabel(exercise)}</td>
                  <td className={cellClassName}>
                    <HvBadge variant={exercise.active ? "success" : "neutral"} size="sm" dot>
                      {exercise.active ? "Hoạt động" : "Ngừng"}
                    </HvBadge>
                  </td>
                  {canEdit ? (
                    <td className={cn(cellClassName, "whitespace-nowrap text-right")}>
                      <div className="inline-flex items-center gap-1">
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Sửa ${exercise.title}`}
                          onClick={() => setEditing(exercise)}
                        >
                          Sửa
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`${exercise.active ? "Ngừng" : "Kích hoạt"} ${exercise.title}`}
                          disabled={setStatus.isPending}
                          onClick={() => toggleStatus(exercise)}
                        >
                          {exercise.active ? "Ngừng" : "Kích hoạt"}
                        </HvButton>
                        <HvButton
                          type="button"
                          size="sm"
                          variant="ghost"
                          aria-label={`Xoá ${exercise.title}`}
                          disabled={exercise.lesson_count > 0}
                          title={
                            exercise.lesson_count > 0
                              ? "Bài tập đang gắn vào buổi học mẫu, ngừng hoạt động thay vì xoá."
                              : undefined
                          }
                          onClick={() => setDeleting(exercise)}
                        >
                          Xoá
                        </HvButton>
                      </div>
                    </td>
                  ) : null}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {canEdit ? (
        <>
          <ExerciseDialog
            mode="create"
            open={creating}
            onOpenChange={setCreating}
            onCreated={() => hvToast("Đã thêm bài tập")}
          />
          {editing ? (
            <ExerciseDialog
              mode="edit"
              exercise={editing}
              open
              onOpenChange={(open) => {
                if (!open) setEditing(null);
              }}
              onSaved={() => hvToast("Đã lưu bài tập")}
            />
          ) : null}
          <HvConfirmDialog
            open={deleting != null}
            onOpenChange={(open) => {
              if (!open) setDeleting(null);
            }}
            title={`Xoá bài tập "${deleting?.title ?? ""}"?`}
            description="Bài tập đang gắn vào một buổi học mẫu sẽ không xoá được."
            confirmLabel="Xoá bài tập"
            tone="danger"
            pending={remove.isPending}
            onConfirm={() => {
              if (!deleting) return;
              remove.mutate(deleting.id, {
                onSuccess: () => {
                  setDeleting(null);
                  hvToast("Đã xoá bài tập");
                },
                onError: (error) => {
                  setDeleting(null);
                  hvToast(apiMessage(error, "Không xoá được bài tập."), { variant: "danger" });
                },
              });
            }}
          />
        </>
      ) : null}
    </div>
  );
}

function TagsRow({ tags }: { tags: string[] }) {
  if (tags.length === 0) return null;
  return <p className="mt-0.5 text-[13px] font-medium text-ink-500">{tags.join(", ")}</p>;
}
