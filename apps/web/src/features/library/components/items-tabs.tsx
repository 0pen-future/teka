import { SearchIcon } from "lucide-react";
import { useEffect, useState, type ReactNode } from "react";

import { HvBadge, HvButton, HvConfirmDialog, HvStateBlock, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import {
  useDeleteExercise,
  useDeleteMaterial,
  useExercisesList,
  useMaterialsList,
} from "../hooks/use-library";
import { formatDifficulty, materialKindLabel } from "../lib/library-labels";
import type { Exercise, Material } from "../schemas/library-schemas";
import { ExerciseDialog } from "./exercise-dialog";
import { MaterialDialog } from "./material-dialog";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/** Debounced search text: `query` is what the input shows, `q` what the list asks for. */
function useSearch() {
  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);
  return { query, setQuery, q };
}

interface CatalogLayoutProps {
  noun: string;
  searchLabel: string;
  addLabel: string;
  canEdit: boolean;
  onAdd: () => void;
  search: ReturnType<typeof useSearch>;
  list: { isPending: boolean; isError: boolean; refetch: () => unknown };
  count: number;
  children: ReactNode;
}

/** Toolbar plus loading/error/empty states shared by both catalog tabs. */
function CatalogLayout({
  noun,
  searchLabel,
  addLabel,
  canEdit,
  onAdd,
  search,
  list,
  count,
  children,
}: CatalogLayoutProps) {
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
            aria-label={searchLabel}
            placeholder="Tìm theo tên…"
            value={search.query}
            onChange={(event) => search.setQuery(event.target.value)}
            className="pl-9"
          />
        </div>
        {canEdit ? (
          <HvButton size="sm" onClick={onAdd}>
            {addLabel}
          </HvButton>
        ) : null}
      </div>

      {list.isPending ? (
        <HvStateBlock state="loading" title={`Đang tải ${noun}`} />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title={`Không tải được ${noun}`}
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : count === 0 ? (
        <HvStateBlock
          state="empty"
          title={search.q ? `Không có ${noun} nào khớp từ khoá.` : `Chưa có ${noun} nào.`}
          description={
            search.q
              ? "Đổi từ khoá hoặc xoá ô tìm."
              : canEdit
                ? `Thêm ${noun} đầu tiên bằng nút ${addLabel}.`
                : `Người có quyền soạn sẽ thêm ${noun} tại đây.`
          }
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          {children}
        </div>
      )}
    </div>
  );
}

function RowActions({
  title,
  onEdit,
  onDelete,
}: {
  title: string;
  onEdit: () => void;
  onDelete: () => void;
}) {
  return (
    <td className={cn(cellClassName, "whitespace-nowrap text-right")}>
      <div className="inline-flex items-center gap-1">
        <HvButton
          type="button"
          size="sm"
          variant="ghost"
          aria-label={`Sửa ${title}`}
          onClick={onEdit}
        >
          Sửa
        </HvButton>
        <HvButton
          type="button"
          size="sm"
          variant="ghost"
          aria-label={`Xoá ${title}`}
          onClick={onDelete}
        >
          Xoá
        </HvButton>
      </div>
    </td>
  );
}

function TagsCell({ tags }: { tags: string[] }) {
  return (
    <td className={cn(cellClassName, "text-ink-700")}>
      {tags.length === 0 ? <span className="text-ink-400">—</span> : tags.join(", ")}
    </td>
  );
}

/** The center's material catalog: search, add, edit and delete (refused while a lesson links it). */
export function MaterialsTab({ canEdit }: { canEdit: boolean }) {
  const search = useSearch();
  const list = useMaterialsList({ q: search.q || undefined, per_page: 100, sort: "title" });
  const remove = useDeleteMaterial();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Material | null>(null);
  const [deleting, setDeleting] = useState<Material | null>(null);
  const rows = list.data?.items ?? [];

  return (
    <>
      <CatalogLayout
        noun="học liệu"
        searchLabel="Tìm học liệu"
        addLabel="Thêm học liệu"
        canEdit={canEdit}
        onAdd={() => setCreating(true)}
        search={search}
        list={list}
        count={rows.length}
      >
        <table className="w-full min-w-[720px] border-collapse text-left text-[14px]">
          <thead>
            <tr>
              <th className={headCellClassName}>Học liệu</th>
              <th className={headCellClassName}>Loại</th>
              <th className={headCellClassName}>Đường dẫn</th>
              <th className={headCellClassName}>Thẻ</th>
              {canEdit ? (
                <th className={headCellClassName}>
                  <span className="sr-only">Thao tác</span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {rows.map((material) => (
              <tr key={material.id} className="transition-colors hover:bg-cream-100">
                <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                  {material.title}
                  {material.description ? (
                    <p className="mt-0.5 line-clamp-2 text-[13px] font-medium text-ink-500">
                      {material.description}
                    </p>
                  ) : null}
                </td>
                <td className={cellClassName}>
                  <HvBadge variant="neutral" size="sm">
                    {materialKindLabel[material.kind]}
                  </HvBadge>
                </td>
                <td className={cn(cellClassName, "max-w-[260px]")}>
                  {material.url ? (
                    <a
                      href={material.url}
                      target="_blank"
                      rel="noreferrer"
                      className="block truncate text-mint-600 hover:underline"
                    >
                      {material.url}
                    </a>
                  ) : (
                    <span className="text-ink-400">—</span>
                  )}
                </td>
                <TagsCell tags={material.tags} />
                {canEdit ? (
                  <RowActions
                    title={material.title}
                    onEdit={() => setEditing(material)}
                    onDelete={() => setDeleting(material)}
                  />
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      </CatalogLayout>

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
    </>
  );
}

/** The center's exercise catalog, same shape as the materials tab. */
export function ExercisesTab({ canEdit }: { canEdit: boolean }) {
  const search = useSearch();
  const list = useExercisesList({ q: search.q || undefined, per_page: 100, sort: "title" });
  const remove = useDeleteExercise();
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Exercise | null>(null);
  const [deleting, setDeleting] = useState<Exercise | null>(null);
  const rows = list.data?.items ?? [];

  return (
    <>
      <CatalogLayout
        noun="bài tập"
        searchLabel="Tìm bài tập"
        addLabel="Thêm bài tập"
        canEdit={canEdit}
        onAdd={() => setCreating(true)}
        search={search}
        list={list}
        count={rows.length}
      >
        <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
          <thead>
            <tr>
              <th className={headCellClassName}>Bài tập</th>
              <th className={headCellClassName}>Độ khó</th>
              <th className={headCellClassName}>Thẻ</th>
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
                <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                  {exercise.title}
                  {exercise.description ? (
                    <p className="mt-0.5 line-clamp-2 text-[13px] font-medium text-ink-500">
                      {exercise.description}
                    </p>
                  ) : null}
                </td>
                <td className={cn(cellClassName, "text-ink-700")}>
                  {exercise.difficulty === null ? (
                    <span className="text-ink-400">—</span>
                  ) : (
                    formatDifficulty(exercise.difficulty)
                  )}
                </td>
                <TagsCell tags={exercise.tags} />
                {canEdit ? (
                  <RowActions
                    title={exercise.title}
                    onEdit={() => setEditing(exercise)}
                    onDelete={() => setDeleting(exercise)}
                  />
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      </CatalogLayout>

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
    </>
  );
}
