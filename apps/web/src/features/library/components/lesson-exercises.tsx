import { useState } from "react";
import { Link } from "react-router";

import { HvButton, HvSelect, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";

import { useSearch } from "../hooks/use-search";
import { useExerciseGroups, useExercisesList, useSetLessonExercises } from "../hooks/use-library";
import { formatDifficulty } from "../lib/library-labels";
import type {
  Exercise,
  LessonExercise,
  LessonExerciseInput,
  TemplateLessonDetail,
} from "../schemas/library-schemas";
import { BankPickerModal } from "./bank-picker-modal";
import { CopyCodeButton } from "./copy-code-button";
import { ExerciseDialog } from "./exercise-dialog";

interface LessonExercisesProps {
  lesson: TemplateLessonDetail;
  templateId: string;
  /** True only on a draft the viewer may author; otherwise the list renders read-only. */
  editable: boolean;
}

/** The group select cannot carry `null` as a real option, so "no group" gets its own value. */
const NOT_SET = "none";

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

function toInput(exercises: LessonExercise[]): LessonExerciseInput[] {
  return exercises.map((exercise) => ({ exercise_id: exercise.id, group_id: exercise.group_id }));
}

/**
 * "Bài tập": the exercises attached to one template lesson, grouped by the
 * version's own exercise groups. Every change rewrites the full attachment
 * list — the API has no partial-update endpoint for this link either.
 */
export function LessonExercises({ lesson, templateId, editable }: LessonExercisesProps) {
  const save = useSetLessonExercises(lesson.id, lesson.version_id, templateId);
  const groups = useExerciseGroups(lesson.version_id);
  const [creating, setCreating] = useState(false);
  const [pickerOpen, setPickerOpen] = useState(false);
  const search = useSearch();

  const attachedIds = new Set(lesson.exercises.map((e) => e.id));
  const catalog = useExercisesList(
    { q: search.q || undefined, active: true, per_page: 100, sort: "title" },
    pickerOpen,
  );
  const pickerItems = catalog.data?.items.filter((e) => !attachedIds.has(e.id));

  function persist(items: LessonExerciseInput[], message: string) {
    save.mutate(items, {
      onSuccess: () => hvToast(message),
      onError: (error) =>
        hvToast(apiMessage(error, "Không lưu được bài tập của buổi học."), { variant: "danger" }),
    });
  }

  function remove(id: string) {
    persist(toInput(lesson.exercises.filter((e) => e.id !== id)), "Đã gỡ bài tập khỏi buổi học");
  }

  function setGroup(id: string, groupId: string | null) {
    persist(
      toInput(lesson.exercises.map((e) => (e.id === id ? { ...e, group_id: groupId } : e))),
      "Đã cập nhật nhóm bài tập",
    );
  }

  function addCreated(exercise: Exercise) {
    setCreating(false);
    persist(
      [...toInput(lesson.exercises), { exercise_id: exercise.id, group_id: null }],
      "Đã thêm bài tập vào buổi học",
    );
  }

  function addFromBank(exercises: Exercise[]) {
    setPickerOpen(false);
    persist(
      [
        ...toInput(lesson.exercises),
        ...exercises.map((e) => ({ exercise_id: e.id, group_id: null })),
      ],
      "Đã thêm bài tập vào buổi học",
    );
  }

  const groupOptions = [
    { value: NOT_SET, label: "Chưa phân nhóm" },
    ...(groups.data ?? []).map((group) => ({ value: group.id, label: group.name })),
  ];

  return (
    <section
      aria-label="Bài tập"
      className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div>
          <h2 className="font-display text-[17px] font-extrabold text-ink-900">Bài tập</h2>
          <p className="text-[13px] text-ink-500">
            Bài tập tự soạn hoặc chọn từ ngân hàng bài tập chung của trung tâm.
          </p>
        </div>
        {editable ? (
          <div className="flex flex-wrap items-center gap-2">
            <HvButton type="button" variant="secondary" size="sm" onClick={() => setCreating(true)}>
              Bài tập tự soạn
            </HvButton>
            <HvButton type="button" size="sm" onClick={() => setPickerOpen(true)}>
              Bài tập có sẵn
            </HvButton>
          </div>
        ) : null}
      </div>

      {lesson.exercises.length === 0 ? (
        <p className="text-[14px] text-ink-400">Chưa có bài tập nào trong buổi.</p>
      ) : (
        <table className="w-full border-collapse text-left text-[14px]">
          <thead>
            <tr className="border-b border-line-200 text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
              <th scope="col" className="py-2 pr-2 font-extrabold">
                STT
              </th>
              <th scope="col" className="py-2 pr-2 font-extrabold">
                Tiêu đề
              </th>
              <th scope="col" className="py-2 pr-2 font-extrabold">
                Nhóm bài tập
              </th>
              <th scope="col" className="py-2 pr-2 font-extrabold">
                Ngân hàng
              </th>
              {editable ? (
                <th scope="col" className="py-2 pr-2 font-extrabold">
                  <span className="sr-only">Hành động</span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {lesson.exercises.map((exercise, index) => (
              <tr key={exercise.id} className="border-b border-line-100 align-top last:border-b-0">
                <td className="py-2 pr-2 text-ink-500">{index + 1}</td>
                <td className="max-w-[280px] py-2 pr-2">
                  <p className="truncate font-bold text-ink-900">{exercise.title}</p>
                  <p className="flex flex-wrap items-center gap-1 text-[12.5px] text-ink-500">
                    <span>
                      {[exercise.skill, exercise.level].filter(Boolean).join(" · ") ||
                        formatDifficulty(exercise.difficulty)}
                    </span>
                    <CopyCodeButton code={exercise.code} />
                  </p>
                </td>
                <td className="py-2 pr-2">
                  {editable ? (
                    <HvSelect
                      aria-label={`Nhóm bài tập cho ${exercise.title}`}
                      options={groupOptions}
                      value={exercise.group_id ?? NOT_SET}
                      onValueChange={(value) =>
                        setGroup(exercise.id, value === NOT_SET ? null : value)
                      }
                      sheetTitle="Chọn nhóm bài tập"
                      searchThreshold={Infinity}
                      className="w-full min-w-[160px]"
                    />
                  ) : (
                    <span className="text-ink-700">
                      {groups.data?.find((group) => group.id === exercise.group_id)?.name ??
                        "Chưa phân nhóm"}
                    </span>
                  )}
                </td>
                <td className="py-2 pr-2">
                  <Link
                    to={`/library/exercises?q=${encodeURIComponent(exercise.code)}`}
                    className="font-bold text-mint-600 hover:underline"
                  >
                    Xem trong ngân hàng
                  </Link>
                </td>
                {editable ? (
                  <td className="py-2 pr-2">
                    <HvButton
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() => remove(exercise.id)}
                    >
                      Gỡ
                    </HvButton>
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </table>
      )}

      {editable ? (
        <ExerciseDialog
          open={creating}
          onOpenChange={(open) => {
            if (!open) setCreating(false);
          }}
          mode="create"
          onCreated={addCreated}
        />
      ) : null}
      {editable ? (
        <BankPickerModal
          open={pickerOpen}
          onOpenChange={setPickerOpen}
          title="Chọn từ ngân hàng bài tập"
          noun="bài tập"
          searchLabel="Tìm bài tập"
          search={search}
          items={pickerItems}
          isPending={catalog.isPending}
          isError={catalog.isError}
          getLabel={(exercise) => exercise.title}
          renderMeta={(exercise) => <CopyCodeButton code={exercise.code} />}
          onConfirm={addFromBank}
          confirmPending={save.isPending}
        />
      ) : null}
    </section>
  );
}
