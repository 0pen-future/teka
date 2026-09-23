import { SearchIcon } from "lucide-react";
import { useEffect, useState } from "react";

import { HvBadge, HvButton, HvStateBlock, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";

import {
  useExercisesList,
  useMaterialsList,
  useSetLessonExercises,
  useSetLessonMaterials,
} from "../hooks/use-library";
import { formatDifficulty, materialKindLabel } from "../lib/library-labels";
import type {
  LessonExercise,
  LessonMaterial,
  LessonMaterialInput,
  TemplateLessonDetail,
} from "../schemas/library-schemas";

interface LessonAttachmentsProps {
  lesson: TemplateLessonDetail;
  templateId: string;
  /** True only on a draft the viewer may author; otherwise the lists render read-only. */
  editable: boolean;
}

/**
 * The materials and exercises attached to one template lesson. Each block
 * keeps its own picker state and saves separately, keyed on what the
 * server currently holds so a save (or a concurrent change) reinitialises
 * only that block.
 */
export function LessonAttachments({ lesson, templateId, editable }: LessonAttachmentsProps) {
  const materialsKey = lesson.materials.map((m) => `${m.id}:${m.shared_with_students}`).join(",");
  const exercisesKey = lesson.exercises.map((e) => e.id).join(",");
  return (
    <>
      <section
        aria-label="Học liệu"
        className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
      >
        <h2 className="font-display text-[16px] font-extrabold text-ink-900">Học liệu</h2>
        {editable ? (
          <MaterialsPicker key={materialsKey} lesson={lesson} templateId={templateId} />
        ) : (
          <MaterialsReadOnly materials={lesson.materials} />
        )}
      </section>
      <section
        aria-label="Bài tập"
        className="flex flex-col gap-3 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
      >
        <h2 className="font-display text-[16px] font-extrabold text-ink-900">Bài tập</h2>
        {editable ? (
          <ExercisesPicker key={exercisesKey} lesson={lesson} templateId={templateId} />
        ) : (
          <ExercisesReadOnly exercises={lesson.exercises} />
        )}
      </section>
    </>
  );
}

function apiMessage(error: unknown, fallback: string): string {
  return error instanceof ApiError ? error.message : fallback;
}

function useSearch() {
  const [query, setQuery] = useState("");
  const [q, setQ] = useState("");
  useEffect(() => {
    const timer = setTimeout(() => setQ(query.trim()), 300);
    return () => clearTimeout(timer);
  }, [query]);
  return { query, setQuery, q };
}

function SearchBox({ label, search }: { label: string; search: ReturnType<typeof useSearch> }) {
  return (
    <div className="relative">
      <SearchIcon
        aria-hidden="true"
        className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-ink-400"
      />
      <Input
        type="search"
        aria-label={label}
        placeholder="Tìm theo tên…"
        value={search.query}
        onChange={(event) => search.setQuery(event.target.value)}
        className="pl-9"
      />
    </div>
  );
}

function PickerState({
  list,
  count,
  noun,
  q,
}: {
  list: { isPending: boolean; isError: boolean };
  count: number;
  noun: string;
  q: string;
}) {
  if (list.isPending) return <HvStateBlock state="loading" title={`Đang tải ${noun}`} />;
  if (list.isError) return <HvStateBlock state="error" title={`Không tải được ${noun}`} />;
  if (count === 0) {
    return (
      <HvStateBlock
        state="empty"
        title={q ? `Không có ${noun} nào khớp từ khoá.` : `Kho chưa có ${noun} nào.`}
        description={
          q
            ? undefined
            : `Thêm ${noun} trong tab ${noun === "học liệu" ? "Học liệu" : "Bài tập"} của kho.`
        }
      />
    );
  }
  return null;
}

const rowClassName = "flex flex-wrap items-center gap-x-3 gap-y-1 border-t border-line-100 py-2";
const checkboxClassName = "size-4 accent-mint-600";

/**
 * Rows the picker shows: the catalog page, plus (when no search narrows
 * it) every attached item the page does not carry, so it can be unticked.
 */
function pickerRows<T extends { id: string }>(
  catalog: T[] | undefined,
  attached: T[],
  q: string,
): T[] {
  if (!catalog) return [];
  if (q) return catalog;
  return [...catalog, ...attached.filter((item) => !catalog.some((row) => row.id === item.id))];
}

/**
 * Checklist over the catalog. Selection lives outside the filtered list so
 * an attached material hidden by the search stays attached; the saved
 * order is the current order first, then newly ticked items in catalog order.
 */
function MaterialsPicker({
  lesson,
  templateId,
}: {
  lesson: TemplateLessonDetail;
  templateId: string;
}) {
  const search = useSearch();
  const catalog = useMaterialsList({ q: search.q || undefined, per_page: 100, sort: "title" });
  const initial = lesson.materials.map((m) => ({
    material_id: m.id,
    shared_with_students: m.shared_with_students,
  }));
  const [selected, setSelected] = useState<LessonMaterialInput[]>(initial);
  const save = useSetLessonMaterials(lesson.id, lesson.version_id, templateId);
  const dirty = JSON.stringify(selected) !== JSON.stringify(initial);
  const rows = pickerRows(catalog.data?.items, lesson.materials, search.q);

  function toggle(id: string) {
    setSelected((current) =>
      current.some((item) => item.material_id === id)
        ? current.filter((item) => item.material_id !== id)
        : [...current, { material_id: id, shared_with_students: false }],
    );
  }

  function toggleShared(id: string) {
    setSelected((current) =>
      current.map((item) =>
        item.material_id === id
          ? { ...item, shared_with_students: !item.shared_with_students }
          : item,
      ),
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <SearchBox label="Tìm học liệu" search={search} />
      <PickerState list={catalog} count={rows.length} noun="học liệu" q={search.q} />
      {rows.length > 0 ? (
        <ul className="flex flex-col">
          {rows.map((material) => {
            const picked = selected.find((item) => item.material_id === material.id);
            return (
              <li key={material.id} className={rowClassName}>
                <label className="flex min-w-0 flex-1 items-center gap-2 text-[14px] text-ink-900">
                  <input
                    type="checkbox"
                    className={checkboxClassName}
                    aria-label={material.title}
                    checked={Boolean(picked)}
                    onChange={() => toggle(material.id)}
                  />
                  <span className="truncate font-bold">{material.title}</span>
                  <HvBadge variant="neutral" size="sm">
                    {materialKindLabel[material.kind]}
                  </HvBadge>
                </label>
                {picked ? (
                  <label className="flex items-center gap-2 text-[13px] text-ink-700">
                    <input
                      type="checkbox"
                      className={checkboxClassName}
                      aria-label={`Chia sẻ ${material.title} với học viên`}
                      checked={picked.shared_with_students}
                      onChange={() => toggleShared(material.id)}
                    />
                    Chia sẻ với học viên
                  </label>
                ) : null}
              </li>
            );
          })}
        </ul>
      ) : null}
      <div className="flex items-center justify-between gap-3">
        <span className="text-[13px] text-ink-500">Đã chọn {selected.length} học liệu</span>
        <HvButton
          type="button"
          size="sm"
          disabled={!dirty || save.isPending}
          onClick={() =>
            save.mutate(selected, {
              onSuccess: () => hvToast("Đã lưu học liệu của buổi"),
              onError: (error) =>
                hvToast(apiMessage(error, "Không lưu được học liệu của buổi."), {
                  variant: "danger",
                }),
            })
          }
        >
          {save.isPending ? "Đang lưu…" : "Lưu học liệu"}
        </HvButton>
      </div>
    </div>
  );
}

function ExercisesPicker({
  lesson,
  templateId,
}: {
  lesson: TemplateLessonDetail;
  templateId: string;
}) {
  const search = useSearch();
  const catalog = useExercisesList({ q: search.q || undefined, per_page: 100, sort: "title" });
  const initial = lesson.exercises.map((e) => e.id);
  const [selected, setSelected] = useState<string[]>(initial);
  const save = useSetLessonExercises(lesson.id, lesson.version_id, templateId);
  const dirty = JSON.stringify(selected) !== JSON.stringify(initial);
  const rows = pickerRows(catalog.data?.items, lesson.exercises, search.q);

  function toggle(id: string) {
    setSelected((current) =>
      current.includes(id) ? current.filter((item) => item !== id) : [...current, id],
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <SearchBox label="Tìm bài tập" search={search} />
      <PickerState list={catalog} count={rows.length} noun="bài tập" q={search.q} />
      {rows.length > 0 ? (
        <ul className="flex flex-col">
          {rows.map((exercise) => (
            <li key={exercise.id} className={rowClassName}>
              <label className="flex min-w-0 flex-1 items-center gap-2 text-[14px] text-ink-900">
                <input
                  type="checkbox"
                  className={checkboxClassName}
                  aria-label={exercise.title}
                  checked={selected.includes(exercise.id)}
                  onChange={() => toggle(exercise.id)}
                />
                <span className="truncate font-bold">{exercise.title}</span>
                {exercise.difficulty !== null ? (
                  <span className="text-[13px] text-ink-500">
                    {formatDifficulty(exercise.difficulty)}
                  </span>
                ) : null}
              </label>
            </li>
          ))}
        </ul>
      ) : null}
      <div className="flex items-center justify-between gap-3">
        <span className="text-[13px] text-ink-500">Đã chọn {selected.length} bài tập</span>
        <HvButton
          type="button"
          size="sm"
          disabled={!dirty || save.isPending}
          onClick={() =>
            save.mutate(
              selected.map((exercise_id) => ({ exercise_id })),
              {
                onSuccess: () => hvToast("Đã lưu bài tập của buổi"),
                onError: (error) =>
                  hvToast(apiMessage(error, "Không lưu được bài tập của buổi."), {
                    variant: "danger",
                  }),
              },
            )
          }
        >
          {save.isPending ? "Đang lưu…" : "Lưu bài tập"}
        </HvButton>
      </div>
    </div>
  );
}

function MaterialsReadOnly({ materials }: { materials: LessonMaterial[] }) {
  if (materials.length === 0) {
    return <p className="text-[14px] text-ink-400">Chưa gắn học liệu.</p>;
  }
  return (
    <ul className="flex flex-col">
      {materials.map((material) => (
        <li key={material.id} className={rowClassName}>
          <span className="font-mono text-[13px] text-ink-500">{material.position}</span>
          <span className="min-w-0 flex-1 truncate text-[14px] font-bold text-ink-900">
            {material.url ? (
              <a
                href={material.url}
                target="_blank"
                rel="noreferrer"
                className="hover:text-mint-600 hover:underline"
              >
                {material.title}
              </a>
            ) : (
              material.title
            )}
          </span>
          <HvBadge variant="neutral" size="sm">
            {materialKindLabel[material.kind]}
          </HvBadge>
          {material.shared_with_students ? (
            <HvBadge variant="success" size="sm">
              Chia sẻ HV
            </HvBadge>
          ) : null}
        </li>
      ))}
    </ul>
  );
}

function ExercisesReadOnly({ exercises }: { exercises: LessonExercise[] }) {
  if (exercises.length === 0) {
    return <p className="text-[14px] text-ink-400">Chưa gắn bài tập.</p>;
  }
  return (
    <ul className="flex flex-col">
      {exercises.map((exercise) => (
        <li key={exercise.id} className={rowClassName}>
          <span className="font-mono text-[13px] text-ink-500">{exercise.position}</span>
          <span className="min-w-0 flex-1 truncate text-[14px] font-bold text-ink-900">
            {exercise.title}
          </span>
          {exercise.difficulty !== null ? (
            <span className="text-[13px] text-ink-500">
              {formatDifficulty(exercise.difficulty)}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  );
}
