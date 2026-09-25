import { Fragment, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { z } from "zod";

import { HvBadge, HvButton, HvChip, HvConfirmDialog, HvStateBlock, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { PathDialog } from "../components/path-dialog";
import { StageCoursesDialog } from "../components/stage-courses-dialog";
import { StageDialog } from "../components/stage-dialog";
import { useDeleteStage, usePathsList, useReorderStages } from "../hooks/use-paths";
import { pathStatusLabel, pathStatusVariant } from "../lib/path-labels";
import {
  cellClassName,
  dashedAddClassName,
  headCellClassName,
  mintActionClassName,
  skyActionClassName,
  tableCardClassName,
} from "../lib/table-classes";
import { pathStatusSchema, type LearningPath, type Stage } from "../schemas/paths-schemas";

type StatusFilter = "all" | z.infer<typeof pathStatusSchema>;

const statusFilterSchema = z.union([z.literal("all"), pathStatusSchema]).catch("all");

const filters: { value: StatusFilter; label: string }[] = [
  { value: "all", label: "Tất cả" },
  { value: "draft", label: pathStatusLabel.draft },
  { value: "active", label: pathStatusLabel.active },
  { value: "archived", label: pathStatusLabel.archived },
];

const stepButtonClassName =
  "flex size-[30px] items-center justify-center rounded-[10px] border-[1.5px] border-line-200 bg-white font-extrabold text-ink-500 hover:border-mint-400 disabled:cursor-not-allowed disabled:opacity-40";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/** Distinct courses across the path's stages — a course may sit in two stages. */
function courseCount(path: LearningPath): number {
  return new Set(path.stages.flatMap((stage) => stage.courses.map((course) => course.id))).size;
}

/**
 * `/paths` — the prototype's "Lộ trình học" table. Paths load once and the
 * status chips filter client-side so every chip can show its count; each
 * row expands into its stages, which are managed in place.
 */
export function LearningPathsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const status: StatusFilter = statusFilterSchema.parse(searchParams.get("status") ?? undefined);
  const { has, isResolved } = useCenterContext();
  const canEdit = has("paths.edit");
  const canOpenCourse = has("courses.read");

  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<LearningPath | null>(null);
  const [open, setOpen] = useState<Record<string, boolean>>({});

  const list = usePathsList({ per_page: 100, sort: "name" }, isResolved);
  const all = list.data?.items ?? [];
  const rows = status === "all" ? all : all.filter((path) => path.status === status);

  function selectStatus(next: StatusFilter) {
    const params = new URLSearchParams(searchParams);
    if (next === "all") {
      params.delete("status");
    } else {
      params.set("status", next);
    }
    setSearchParams(params, { replace: true });
  }

  function toggle(id: string) {
    setOpen((current) => ({ ...current, [id]: !current[id] }));
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start gap-3">
        <div className="min-w-[260px] flex-1">
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Lộ trình học</h1>
          <p className="mt-1 text-[14px] text-ink-500">
            Starters → Movers → KET → PET. Một lộ trình gồm nhiều chặng, mỗi chặng chứa các khóa
            học.
          </p>
        </div>
        {canEdit ? <HvButton onClick={() => setCreating(true)}>+ Tạo lộ trình</HvButton> : null}
      </div>

      <div role="radiogroup" aria-label="Lọc theo trạng thái" className="flex flex-wrap gap-2">
        {filters.map((filter) => (
          <HvChip
            key={filter.value}
            role="radio"
            size="sm"
            pressed={filter.value === status}
            count={
              list.data
                ? filter.value === "all"
                  ? all.length
                  : all.filter((path) => path.status === filter.value).length
                : undefined
            }
            onClick={() => selectStatus(filter.value)}
          >
            {filter.label}
          </HvChip>
        ))}
      </div>

      {!isResolved || list.isPending ? (
        <HvStateBlock state="loading" title="Đang tải lộ trình học" />
      ) : list.isError ? (
        <HvStateBlock
          state="error"
          title="Không tải được lộ trình học"
          action={
            <HvButton size="sm" variant="ghost" onClick={() => void list.refetch()}>
              Thử lại
            </HvButton>
          }
        />
      ) : (
        <div className={tableCardClassName}>
          <table className="w-full min-w-[760px] table-fixed border-collapse text-left text-[13.5px]">
            <colgroup>
              <col className="w-[56px]" />
              <col className="w-[34%]" />
              <col className="w-[20%]" />
              <col className="w-[13%]" />
              <col className="w-[13%]" />
              <col className="w-[200px]" />
            </colgroup>
            <thead>
              <tr>
                <th className={headCellClassName}>STT</th>
                <th className={headCellClassName}>Tên lộ trình</th>
                <th className={headCellClassName}>Trạng thái</th>
                <th className={headCellClassName}>Số chặng</th>
                <th className={headCellClassName}>Khóa học</th>
                <th className={headCellClassName}>
                  <span className="sr-only">Thao tác</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {rows.length === 0 ? (
                <tr>
                  <td
                    colSpan={6}
                    className="border-t border-line-100 p-[30px] text-center font-bold text-ink-400"
                  >
                    Không có lộ trình nào ở trạng thái này.
                  </td>
                </tr>
              ) : null}
              {rows.map((path, index) => {
                const expanded = Boolean(open[path.id]);
                return (
                  <Fragment key={path.id}>
                    <tr
                      className="cursor-pointer transition-colors hover:bg-cream-100"
                      onClick={() => toggle(path.id)}
                    >
                      <td className={cn(cellClassName, "font-extrabold text-ink-400")}>
                        {index + 1}
                      </td>
                      <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                        {path.name}
                      </td>
                      <td className={cellClassName}>
                        <HvBadge variant={pathStatusVariant[path.status]} size="sm">
                          {pathStatusLabel[path.status]}
                        </HvBadge>
                      </td>
                      <td className={cn(cellClassName, "font-bold")}>{path.stage_count}</td>
                      <td className={cn(cellClassName, "font-bold")}>{courseCount(path)}</td>
                      <td className={cellClassName}>
                        <div className="flex justify-end gap-1">
                          {canEdit ? (
                            <button
                              type="button"
                              className={skyActionClassName}
                              aria-label={`Sửa ${path.name}`}
                              onClick={(event) => {
                                event.stopPropagation();
                                setEditing(path);
                              }}
                            >
                              Sửa
                            </button>
                          ) : null}
                          <button
                            type="button"
                            className={mintActionClassName}
                            aria-expanded={expanded}
                            aria-label={`${expanded ? "Thu gọn" : "Quản lý chặng"} ${path.name}`}
                            onClick={(event) => {
                              event.stopPropagation();
                              toggle(path.id);
                            }}
                          >
                            {expanded ? "Thu gọn ▴" : "Quản lý chặng ▾"}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {expanded ? (
                      <tr>
                        <td colSpan={6} className="bg-cream-50 px-[18px] pt-1.5 pb-4">
                          <PathStages path={path} canEdit={canEdit} canOpenCourse={canOpenCourse} />
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {canEdit ? (
        <>
          <PathDialog
            mode="create"
            open={creating}
            onOpenChange={setCreating}
            onCreated={(path) => {
              hvToast(`Đã tạo lộ trình ${path.code}`);
              setOpen((current) => ({ ...current, [path.id]: true }));
            }}
          />
          {editing ? (
            <PathDialog
              mode="edit"
              path={editing}
              open
              onOpenChange={(next) => {
                if (!next) setEditing(null);
              }}
              onSaved={() => hvToast("Đã lưu lộ trình")}
            />
          ) : null}
        </>
      ) : null}
    </div>
  );
}

interface PathStagesProps {
  path: LearningPath;
  canEdit: boolean;
  canOpenCourse: boolean;
}

/** The expanded row: one line per stage plus the "+ Thêm chặng" affordance. */
function PathStages({ path, canEdit, canOpenCourse }: PathStagesProps) {
  const reorder = useReorderStages(path.id);
  const [addingStage, setAddingStage] = useState(false);

  function move(index: number, delta: number) {
    const ids = path.stages.map((stage) => stage.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate(ids, {
      onError: (error) =>
        hvToast(apiMessage(error, "Không sắp xếp được chặng."), { variant: "danger" }),
    });
  }

  return (
    <div className="flex flex-col gap-2">
      {path.stages.length === 0 && !canEdit ? (
        <p className="py-2 text-[13px] font-bold text-ink-400">Lộ trình chưa có chặng nào.</p>
      ) : null}
      {path.stages.map((stage, index) => (
        <StageRow
          key={stage.id}
          pathId={path.id}
          stage={stage}
          number={index + 1}
          canEdit={canEdit}
          canOpenCourse={canOpenCourse}
          isFirst={index === 0}
          isLast={index === path.stages.length - 1}
          moving={reorder.isPending}
          onMoveUp={() => move(index, -1)}
          onMoveDown={() => move(index, 1)}
        />
      ))}
      {canEdit ? (
        <>
          <button
            type="button"
            className={cn(
              dashedAddClassName,
              "rounded-[14px] border-2 px-3.5 py-[9px] text-[12.5px]",
            )}
            onClick={() => setAddingStage(true)}
          >
            + Thêm chặng
          </button>
          <StageDialog
            mode="create"
            pathId={path.id}
            open={addingStage}
            onOpenChange={setAddingStage}
          />
        </>
      ) : null}
    </div>
  );
}

interface StageRowProps {
  pathId: string;
  stage: Stage;
  number: number;
  canEdit: boolean;
  canOpenCourse: boolean;
  isFirst: boolean;
  isLast: boolean;
  moving: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
}

function StageRow({
  pathId,
  stage,
  number,
  canEdit,
  canOpenCourse,
  isFirst,
  isLast,
  moving,
  onMoveUp,
  onMoveDown,
}: StageRowProps) {
  const removeStage = useDeleteStage(pathId);
  const [renaming, setRenaming] = useState(false);
  const [picking, setPicking] = useState(false);
  const [deleting, setDeleting] = useState(false);

  return (
    <div
      role="group"
      aria-label={`Chặng ${number}: ${stage.name}`}
      className="flex flex-wrap items-center gap-3 rounded-2xl bg-cream-100 px-3.5 py-2.5"
    >
      <span
        aria-hidden="true"
        className="flex size-[30px] flex-none items-center justify-center rounded-full bg-mint-400 font-display text-[14px] font-extrabold text-white"
      >
        {number}
      </span>
      {canEdit ? (
        <button
          type="button"
          title="Đổi tên chặng"
          className="w-[120px] text-left text-[14.5px] font-extrabold text-ink-900 hover:text-mint-600"
          onClick={() => setRenaming(true)}
        >
          {stage.name}
        </button>
      ) : (
        <span className="w-[120px] text-[14.5px] font-extrabold text-ink-900">{stage.name}</span>
      )}
      <div className="flex min-w-[160px] flex-1 flex-wrap gap-1.5">
        {stage.courses.map((course) => {
          const chipClassName = cn(
            "inline-flex items-center rounded-full px-2.5 py-1 text-[12px] font-extrabold",
            course.status === "active" ? "bg-mint-50 text-mint-600" : "bg-cream-200 text-ink-400",
          );
          const label = `${course.code} · ${course.name}`;
          return canOpenCourse ? (
            <Link key={course.id} to={`/courses/${course.id}`} className={chipClassName}>
              {label}
            </Link>
          ) : (
            <span key={course.id} className={chipClassName}>
              {label}
            </span>
          );
        })}
        {canEdit ? (
          <button
            type="button"
            aria-label={`Thêm khóa vào ${stage.name}`}
            className={cn(
              dashedAddClassName,
              "rounded-full border-[1.5px] px-2.5 py-1 text-[12px]",
            )}
            onClick={() => setPicking(true)}
          >
            + Khóa
          </button>
        ) : null}
      </div>
      {canEdit ? (
        <div className="flex gap-1">
          <button
            type="button"
            className={stepButtonClassName}
            aria-label={`Đưa ${stage.name} lên`}
            disabled={isFirst || moving}
            onClick={onMoveUp}
          >
            ↑
          </button>
          <button
            type="button"
            className={stepButtonClassName}
            aria-label={`Đưa ${stage.name} xuống`}
            disabled={isLast || moving}
            onClick={onMoveDown}
          >
            ↓
          </button>
          <button
            type="button"
            title="Xoá chặng"
            aria-label={`Xoá chặng ${stage.name}`}
            className={cn(
              stepButtonClassName,
              "text-coral-400 hover:border-coral-300 hover:bg-coral-100",
            )}
            onClick={() => setDeleting(true)}
          >
            ×
          </button>
        </div>
      ) : null}

      {canEdit ? (
        <>
          <StageDialog
            mode="edit"
            pathId={pathId}
            stage={stage}
            open={renaming}
            onOpenChange={setRenaming}
          />
          <StageCoursesDialog
            pathId={pathId}
            stage={stage}
            open={picking}
            onOpenChange={setPicking}
          />
          <HvConfirmDialog
            open={deleting}
            onOpenChange={setDeleting}
            title={`Xoá chặng "${stage.name}"?`}
            description="Các chặng sau được đánh số lại. Khóa học vẫn giữ nguyên trong danh sách khóa học."
            confirmLabel="Xoá chặng"
            tone="danger"
            pending={removeStage.isPending}
            onConfirm={() =>
              removeStage.mutate(stage.id, {
                onSuccess: () => setDeleting(false),
                onError: (error) => {
                  setDeleting(false);
                  hvToast(apiMessage(error, "Không xoá được chặng."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}
    </div>
  );
}
