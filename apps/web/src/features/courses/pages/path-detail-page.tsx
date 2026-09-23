import { ArrowDownIcon, ArrowLeftIcon, ArrowUpIcon, XIcon } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams } from "react-router";

import {
  HvBadge,
  HvButton,
  HvConfirmDialog,
  HvSelect,
  HvStateBlock,
  hvToast,
} from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import { PathDialog } from "../components/path-dialog";
import { StageDialog } from "../components/stage-dialog";
import { useCoursesList } from "../hooks/use-courses";
import {
  useDeletePath,
  useDeleteStage,
  usePath,
  useReorderStages,
  useSetStageCourses,
} from "../hooks/use-paths";
import { courseStatusLabel } from "../lib/course-labels";
import { pathStatusLabel, pathStatusVariant } from "../lib/path-labels";
import type { Course } from "../schemas/courses-schemas";
import type { LearningPath, Stage } from "../schemas/paths-schemas";

/** The API caps a stage's recommendations; the picker closes at the cap. */
const MAX_STAGE_COURSES = 20;

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

/**
 * `/paths/:id` — one learning path as a vertical timeline of stages, each
 * listing the courses a student should take at that point.
 */
export function PathDetailPage() {
  const { id = "" } = useParams<{ id: string }>();
  const { isResolved } = useCenterContext();
  const path = usePath(id);

  // Wait for the permission set too, so editing controls never pop in late.
  if (path.isPending || !isResolved) {
    return <HvStateBlock state="loading" title="Đang tải lộ trình" />;
  }
  if (path.isError) {
    const notFound = path.error instanceof ApiError && path.error.status === 404;
    return (
      <HvStateBlock
        state="error"
        title={notFound ? "Không tìm thấy lộ trình" : "Không tải được lộ trình"}
        description={
          notFound ? "Lộ trình có thể đã bị xoá hoặc đường dẫn không đúng." : "Thử tải lại trang."
        }
        action={
          <Link
            to="/paths"
            className="font-display text-[13px] font-bold text-mint-600 hover:underline"
          >
            Về danh sách lộ trình
          </Link>
        }
      />
    );
  }

  return <PathWorkspace path={path.data} />;
}

function PathWorkspace({ path }: { path: LearningPath }) {
  const navigate = useNavigate();
  const { has } = useCenterContext();
  const canEdit = has("paths.edit");

  const remove = useDeletePath();
  const reorder = useReorderStages(path.id);
  // The picker offers the catalog's active courses; a stage keeps an archived
  // one it already holds until an editor drops it.
  const catalog = useCoursesList({ status: "active", per_page: 100, sort: "name" }, canEdit);
  const activeCourses = catalog.data?.items ?? [];

  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);
  const [addingStage, setAddingStage] = useState(false);

  function move(index: number, delta: number) {
    const ids = path.stages.map((stage) => stage.id);
    const target = index + delta;
    if (target < 0 || target >= ids.length) return;
    [ids[index], ids[target]] = [ids[target]!, ids[index]!];
    reorder.mutate(ids, {
      onError: (error) =>
        hvToast(apiMessage(error, "Không sắp xếp được giai đoạn."), { variant: "danger" }),
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-3">
        <Link
          to="/paths"
          className="inline-flex items-center gap-1 self-start font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          <ArrowLeftIcon aria-hidden="true" className="size-4" />
          Lộ trình học
        </Link>
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="font-display text-[26px] font-extrabold text-ink-900">{path.name}</h1>
            <div className="mt-1 flex flex-wrap items-center gap-2 text-[14px] text-ink-500">
              <span className="font-mono text-[13px]">Mã: {path.code}</span>
              <HvBadge variant={pathStatusVariant[path.status]} size="sm" dot>
                {pathStatusLabel[path.status]}
              </HvBadge>
            </div>
            {path.description ? (
              <p className="mt-2 max-w-[720px] text-[14px] whitespace-pre-line text-ink-700">
                {path.description}
              </p>
            ) : null}
          </div>
          {canEdit ? (
            <div className="flex flex-wrap gap-2">
              <HvButton size="sm" variant="secondary" onClick={() => setEditing(true)}>
                Sửa lộ trình
              </HvButton>
              <HvButton size="sm" variant="ghost" onClick={() => setDeleting(true)}>
                Xoá lộ trình
              </HvButton>
            </div>
          ) : null}
        </div>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="font-display text-[18px] font-extrabold text-ink-900">
          Các giai đoạn ({path.stages.length})
        </h2>
        {canEdit ? (
          <HvButton size="sm" variant="secondary" onClick={() => setAddingStage(true)}>
            Thêm giai đoạn
          </HvButton>
        ) : null}
      </div>

      {path.stages.length === 0 ? (
        <HvStateBlock
          state="empty"
          title="Chưa có giai đoạn nào."
          description={
            canEdit
              ? "Thêm giai đoạn đầu tiên rồi gắn khóa học vào từng giai đoạn."
              : "Người có quyền quản lý lộ trình sẽ thêm giai đoạn tại đây."
          }
        />
      ) : (
        <ol className="relative flex flex-col gap-4 border-l-2 border-line-200 pl-6">
          {path.stages.map((stage, index) => (
            <li key={stage.id} className="relative">
              <span
                aria-hidden="true"
                className="absolute top-4 -left-[35px] flex size-6 items-center justify-center rounded-full bg-mint-600 font-display text-[12px] font-extrabold text-white"
              >
                {stage.position}
              </span>
              <StageCard
                pathId={path.id}
                stage={stage}
                canEdit={canEdit}
                activeCourses={activeCourses}
                isFirst={index === 0}
                isLast={index === path.stages.length - 1}
                moving={reorder.isPending}
                onMoveUp={() => move(index, -1)}
                onMoveDown={() => move(index, 1)}
              />
            </li>
          ))}
        </ol>
      )}

      {canEdit ? (
        <>
          <PathDialog
            mode="edit"
            path={path}
            open={editing}
            onOpenChange={setEditing}
            onSaved={() => hvToast("Đã lưu lộ trình")}
          />
          <StageDialog
            mode="create"
            pathId={path.id}
            open={addingStage}
            onOpenChange={setAddingStage}
          />
          <HvConfirmDialog
            open={deleting}
            onOpenChange={setDeleting}
            title={`Xoá lộ trình "${path.name}"?`}
            description="Các giai đoạn và khóa học gợi ý trong lộ trình bị gỡ theo. Khóa học vẫn giữ nguyên trong danh mục."
            confirmLabel="Xoá lộ trình"
            tone="danger"
            pending={remove.isPending}
            onConfirm={() =>
              remove.mutate(path.id, {
                onSuccess: () => {
                  hvToast(`Đã xoá lộ trình ${path.code}`);
                  void navigate("/paths");
                },
                onError: (error) => {
                  setDeleting(false);
                  hvToast(apiMessage(error, "Không xoá được lộ trình."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}
    </div>
  );
}

interface StageCardProps {
  pathId: string;
  stage: Stage;
  canEdit: boolean;
  activeCourses: Course[];
  isFirst: boolean;
  isLast: boolean;
  moving: boolean;
  onMoveUp: () => void;
  onMoveDown: () => void;
}

function StageCard({
  pathId,
  stage,
  canEdit,
  activeCourses,
  isFirst,
  isLast,
  moving,
  onMoveUp,
  onMoveDown,
}: StageCardProps) {
  const setCourses = useSetStageCourses(pathId);
  const removeStage = useDeleteStage(pathId);
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const held = new Set(stage.courses.map((course) => course.id));
  const options = activeCourses
    .filter((course) => !held.has(course.id))
    .map((course) => ({ value: course.id, label: course.name, meta: course.code }));
  const full = stage.courses.length >= MAX_STAGE_COURSES;

  function save(courseIds: string[]) {
    setCourses.mutate(
      { stageId: stage.id, courseIds },
      {
        onError: (error) =>
          hvToast(apiMessage(error, "Không lưu được khóa học của giai đoạn."), {
            variant: "danger",
          }),
      },
    );
  }

  const label = `Giai đoạn ${stage.position}: ${stage.name}`;
  return (
    <section
      aria-label={label}
      className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <div className="text-[12px] font-extrabold tracking-[0.4px] text-ink-500 uppercase">
            Giai đoạn {stage.position}
          </div>
          <h3 className="font-display text-[17px] font-extrabold text-ink-900">{stage.name}</h3>
          {stage.goal ? (
            <p className="mt-1 text-[14px] whitespace-pre-line text-ink-700">{stage.goal}</p>
          ) : null}
        </div>
        {canEdit ? (
          <div className="flex flex-wrap gap-1">
            <HvButton
              size="sm"
              variant="ghost"
              disabled={isFirst || moving}
              onClick={onMoveUp}
              aria-label="Lên"
            >
              <ArrowUpIcon aria-hidden="true" className="size-4" />
            </HvButton>
            <HvButton
              size="sm"
              variant="ghost"
              disabled={isLast || moving}
              onClick={onMoveDown}
              aria-label="Xuống"
            >
              <ArrowDownIcon aria-hidden="true" className="size-4" />
            </HvButton>
            <HvButton size="sm" variant="ghost" onClick={() => setEditing(true)}>
              Sửa
            </HvButton>
            <HvButton size="sm" variant="ghost" onClick={() => setDeleting(true)}>
              Xoá
            </HvButton>
          </div>
        ) : null}
      </div>

      {stage.courses.length === 0 ? (
        <p className="text-[14px] text-ink-500">Chưa gắn khóa học nào.</p>
      ) : (
        <ul className="flex flex-wrap gap-2">
          {stage.courses.map((course) => (
            <li
              key={course.id}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-full border-2 border-line-200 bg-white py-1 pl-3 text-[13px] font-bold text-ink-700",
                canEdit ? "pr-1" : "pr-3",
              )}
            >
              <Link to={`/courses/${course.id}`} className="hover:text-mint-600">
                {course.name}
              </Link>
              <span className="font-mono text-[12px] text-ink-400">{course.code}</span>
              {course.status !== "active" ? (
                <HvBadge variant="neutral" size="sm">
                  {courseStatusLabel[course.status]}
                </HvBadge>
              ) : null}
              {canEdit ? (
                <button
                  type="button"
                  aria-label={`Gỡ ${course.name}`}
                  disabled={setCourses.isPending}
                  onClick={() =>
                    save(stage.courses.map((c) => c.id).filter((id) => id !== course.id))
                  }
                  className="flex size-6 items-center justify-center rounded-full text-ink-400 hover:bg-cream-100 hover:text-coral-600 disabled:opacity-50"
                >
                  <XIcon aria-hidden="true" className="size-3.5" />
                </button>
              ) : null}
            </li>
          ))}
        </ul>
      )}

      {canEdit ? (
        <div className="flex flex-wrap items-center gap-3">
          <HvSelect
            options={options}
            value=""
            onValueChange={(courseId) => save([...stage.courses.map((c) => c.id), courseId])}
            sheetTitle="Chọn khóa học"
            placeholder="Thêm khóa học…"
            searchNoun="khóa học"
            searchThreshold={5}
            aria-label="Thêm khóa học"
            disabled={full || setCourses.isPending}
            className="min-w-[240px] max-sm:w-full"
          />
          {full ? (
            <span className="text-[13px] text-ink-500">
              Mỗi giai đoạn tối đa {MAX_STAGE_COURSES} khóa học.
            </span>
          ) : null}
        </div>
      ) : null}

      {canEdit ? (
        <>
          <StageDialog
            mode="edit"
            pathId={pathId}
            stage={stage}
            open={editing}
            onOpenChange={setEditing}
          />
          <HvConfirmDialog
            open={deleting}
            onOpenChange={setDeleting}
            title={`Xoá giai đoạn "${stage.name}"?`}
            description="Các giai đoạn sau được đánh số lại. Khóa học vẫn giữ nguyên trong danh mục."
            confirmLabel="Xoá giai đoạn"
            tone="danger"
            pending={removeStage.isPending}
            onConfirm={() =>
              removeStage.mutate(stage.id, {
                onSuccess: () => setDeleting(false),
                onError: (error) => {
                  setDeleting(false);
                  hvToast(apiMessage(error, "Không xoá được giai đoạn."), { variant: "danger" });
                },
              })
            }
          />
        </>
      ) : null}
    </section>
  );
}
