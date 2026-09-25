import { XIcon } from "lucide-react";

import { HvButton, HvModal, HvSelect, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useCoursesList } from "../hooks/use-courses";
import { useSetStageCourses } from "../hooks/use-paths";
import type { Stage } from "../schemas/paths-schemas";

/** The API caps a stage's recommendations; the picker closes at the cap. */
const MAX_STAGE_COURSES = 20;

interface StageCoursesDialogProps {
  pathId: string;
  stage: Stage;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * "+ Khóa" on a stage row: add an active catalog course to the stage or drop
 * one it holds. Each change saves at once — the stage's course set is
 * replaced wholesale by the API, so there is no draft to lose.
 */
export function StageCoursesDialog({ pathId, stage, open, onOpenChange }: StageCoursesDialogProps) {
  const setCourses = useSetStageCourses(pathId);
  // A stage keeps an archived course it already holds until an editor drops it,
  // but only active courses can be added.
  const catalog = useCoursesList({ status: "active", per_page: 100, sort: "name" }, open);
  const held = new Set(stage.courses.map((course) => course.id));
  const options = (catalog.data?.items ?? [])
    .filter((course) => !held.has(course.id))
    .map((course) => ({ value: course.id, label: course.name, meta: course.code }));
  const full = stage.courses.length >= MAX_STAGE_COURSES;

  function save(courseIds: string[]) {
    setCourses.mutate(
      { stageId: stage.id, courseIds },
      {
        onError: (error) =>
          hvToast(
            error instanceof ApiError ? error.message : "Không lưu được khóa học của chặng.",
            { variant: "danger" },
          ),
      },
    );
  }

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={`Khóa học của chặng ${stage.name}`}
      description="Khóa học đang hoạt động có thể gắn vào chặng. Một khóa có thể nằm trong nhiều lộ trình."
      footer={
        <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
          Đóng
        </HvButton>
      }
    >
      <div className="flex flex-col gap-3">
        {stage.courses.length === 0 ? (
          <p className="text-[14px] text-ink-500">Chưa gắn khóa học nào.</p>
        ) : (
          <ul className="flex flex-wrap gap-2">
            {stage.courses.map((course) => (
              <li
                key={course.id}
                className="inline-flex items-center gap-1.5 rounded-full bg-cream-100 py-1 pr-1 pl-3 text-[13px] font-bold text-ink-700"
              >
                {course.code} · {course.name}
                <button
                  type="button"
                  aria-label={`Gỡ ${course.name}`}
                  disabled={setCourses.isPending}
                  onClick={() =>
                    save(stage.courses.map((c) => c.id).filter((id) => id !== course.id))
                  }
                  className="flex size-6 items-center justify-center rounded-full text-ink-400 hover:bg-cream-200 hover:text-coral-600 disabled:opacity-50"
                >
                  <XIcon aria-hidden="true" className="size-3.5" />
                </button>
              </li>
            ))}
          </ul>
        )}
        {catalog.isError ? (
          <p role="alert" className="text-[13px] text-coral-600">
            Không tải được danh sách khóa học. Bạn vẫn có thể gỡ khóa đã gắn.
          </p>
        ) : null}
        <HvSelect
          options={options}
          value=""
          onValueChange={(courseId) => save([...stage.courses.map((c) => c.id), courseId])}
          sheetTitle="Chọn khóa học"
          placeholder="Thêm khóa học…"
          searchNoun="khóa học"
          searchThreshold={5}
          aria-label={`Thêm khóa học vào ${stage.name}`}
          disabled={full || catalog.isError || setCourses.isPending}
          className="w-full"
        />
        {full ? (
          <span className="text-[13px] text-ink-500">
            Mỗi chặng tối đa {MAX_STAGE_COURSES} khóa học.
          </span>
        ) : null}
      </div>
    </HvModal>
  );
}
