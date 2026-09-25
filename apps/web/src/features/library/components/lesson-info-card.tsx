import { useState } from "react";

import { HvBadge, HvButton, hvToast } from "@/components/hv";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn } from "@/lib/utils";

import { useLessonForm } from "../hooks/use-lesson-form";
import { useUpdateLesson } from "../hooks/use-library";
import { formatDuration, lessonModeLabel } from "../lib/library-labels";
import { toLessonForm, toLessonInput, type TemplateLesson } from "../schemas/library-schemas";
import { LessonFields } from "./lesson-form";

interface LessonInfoCardProps {
  lesson: TemplateLesson;
  templateId: string;
  /** Draft version and `library.edit`: the "Sửa" button and the edit form appear. */
  editable: boolean;
}

/**
 * "Thông tin chung": the lesson's own fields (title, mode, duration, unit,
 * short description, homework) as a view/edit toggle card, separate from its
 * content and exercises.
 */
export function LessonInfoCard({ lesson, templateId, editable }: LessonInfoCardProps) {
  const [editing, setEditing] = useState(false);

  return (
    <section
      aria-label="Thông tin chung"
      className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h2 className="font-display text-[17px] font-extrabold text-ink-900">Thông tin chung</h2>
          <HvBadge variant="neutral" size="sm">
            {lessonModeLabel[lesson.mode]}
          </HvBadge>
        </div>
        {editable ? (
          <HvButton type="button" variant="ghost" size="sm" onClick={() => setEditing((v) => !v)}>
            {editing ? "Huỷ" : "Sửa"}
          </HvButton>
        ) : null}
      </div>
      {editable && editing ? (
        <LessonInfoEditor
          key={lesson.updated_at}
          lesson={lesson}
          templateId={templateId}
          onSaved={() => setEditing(false)}
        />
      ) : (
        <LessonInfoView lesson={lesson} />
      )}
    </section>
  );
}

function LessonInfoView({ lesson }: { lesson: TemplateLesson }) {
  return (
    <div className="grid gap-4 sm:grid-cols-2">
      <ViewField label="Hình thức" value={lessonModeLabel[lesson.mode]} />
      <ViewField label="Thời lượng" value={formatDuration(lesson.duration_min)} />
      <ViewField label="Đơn vị" value={lesson.unit} />
      <ViewField label="Mô tả ngắn" value={lesson.objectives} className="sm:col-span-2" />
      <ViewField label="Bài tập về nhà" value={lesson.homework_note} className="sm:col-span-2" />
    </div>
  );
}

function ViewField({
  label,
  value,
  className,
}: {
  label: string;
  value: string | null;
  className?: string;
}) {
  return (
    <div role="group" aria-label={label} className={cn("flex flex-col gap-1", className)}>
      <span className="text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
        {label}
      </span>
      {value ? (
        <p className="whitespace-pre-wrap text-[14px] text-ink-900">{value}</p>
      ) : (
        <p className="text-[14px] text-ink-400">Chưa có</p>
      )}
    </div>
  );
}

function LessonInfoEditor({
  lesson,
  templateId,
  onSaved,
}: {
  lesson: TemplateLesson;
  templateId: string;
  onSaved: () => void;
}) {
  const form = useLessonForm(toLessonForm(lesson));
  const mutation = useUpdateLesson(lesson.id, lesson.version_id, templateId);
  const handleApiError = useApiFormErrors(form);

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(toLessonInput(values), {
      onSuccess: (saved) => {
        form.reset(toLessonForm(saved));
        hvToast("Đã lưu buổi học");
        onSaved();
      },
      onError: handleApiError,
    });
  });

  return (
    <form onSubmit={(event) => void onSubmit(event)} noValidate className="flex flex-col gap-4">
      <LessonFields form={form} idPrefix="lesson-info" />
      <div className="flex justify-end">
        <HvButton type="submit" disabled={mutation.isPending}>
          {mutation.isPending ? "Đang lưu…" : "Lưu"}
        </HvButton>
      </div>
    </form>
  );
}
