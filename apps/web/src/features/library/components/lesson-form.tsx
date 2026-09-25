import { useEffect } from "react";
import { Controller } from "react-hook-form";

import { HvButton, HvModal, HvSegmented, type HvSegmentedOption } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { EMPTY_LESSON_FORM, useLessonForm, type LessonForm } from "../hooks/use-lesson-form";
import { useCreateLesson } from "../hooks/use-library";
import { textareaClassName } from "@/lib/forms/textarea-class";
import {
  lessonModeSchema,
  toLessonInput,
  type LessonMode,
  type TemplateLesson,
} from "../schemas/library-schemas";
import { lessonModeLabel } from "../lib/library-labels";

const modeOptions: HvSegmentedOption<LessonMode>[] = lessonModeSchema.options.map((mode) => ({
  value: mode,
  label: lessonModeLabel[mode],
}));

interface LessonFieldsProps {
  form: LessonForm;
  /** Prefix for the input ids so two forms on one page never collide. */
  idPrefix: string;
}

/** The lesson inputs, shared by the add dialog and the lesson info card's edit form. */
export function LessonFields({ form, idPrefix }: LessonFieldsProps) {
  const { errors } = form.formState;
  return (
    <FieldGroup>
      <Field data-invalid={Boolean(errors.title)}>
        <FieldLabel htmlFor={`${idPrefix}-title`}>Tên buổi</FieldLabel>
        <Input
          id={`${idPrefix}-title`}
          placeholder="VD: Số tự nhiên"
          aria-invalid={Boolean(errors.title)}
          {...form.register("title")}
        />
        <FieldError errors={[errors.title]} />
      </Field>
      <Field data-invalid={Boolean(errors.mode)}>
        <FieldLabel htmlFor={`${idPrefix}-mode`}>Hình thức</FieldLabel>
        <Controller
          control={form.control}
          name="mode"
          render={({ field }) => (
            <HvSegmented
              aria-label="Hình thức buổi học"
              idBase={`${idPrefix}-mode`}
              options={modeOptions}
              value={field.value}
              onValueChange={field.onChange}
            />
          )}
        />
        <p className="text-[12px] text-ink-500">
          Buổi không lịch không xuất hiện trên thời khoá biểu; học viên tự học theo tiến độ riêng.
        </p>
        <FieldError errors={[errors.mode]} />
      </Field>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field data-invalid={Boolean(errors.duration_min)}>
          <FieldLabel htmlFor={`${idPrefix}-duration`}>Thời lượng (phút)</FieldLabel>
          <Input
            id={`${idPrefix}-duration`}
            type="number"
            inputMode="numeric"
            step={15}
            min={15}
            max={1440}
            placeholder="VD: 90"
            aria-invalid={Boolean(errors.duration_min)}
            {...form.register("duration_min")}
          />
          <FieldError errors={[errors.duration_min]} />
        </Field>
        <Field data-invalid={Boolean(errors.unit)}>
          <FieldLabel htmlFor={`${idPrefix}-unit`}>Đơn vị</FieldLabel>
          <Input
            id={`${idPrefix}-unit`}
            placeholder="VD: Tổ Toán"
            aria-invalid={Boolean(errors.unit)}
            {...form.register("unit")}
          />
          <FieldError errors={[errors.unit]} />
        </Field>
      </div>
      <Field data-invalid={Boolean(errors.objectives)}>
        <FieldLabel htmlFor={`${idPrefix}-objectives`}>Mô tả ngắn</FieldLabel>
        <textarea
          id={`${idPrefix}-objectives`}
          rows={3}
          placeholder="Mục tiêu buổi học, lưu ý cho giáo viên…"
          className={textareaClassName}
          aria-invalid={Boolean(errors.objectives)}
          {...form.register("objectives")}
        />
        <FieldError errors={[errors.objectives]} />
      </Field>
      <Field data-invalid={Boolean(errors.homework_note)}>
        <FieldLabel htmlFor={`${idPrefix}-homework`}>Bài tập về nhà</FieldLabel>
        <textarea
          id={`${idPrefix}-homework`}
          rows={3}
          className={textareaClassName}
          aria-invalid={Boolean(errors.homework_note)}
          {...form.register("homework_note")}
        />
        <FieldError errors={[errors.homework_note]} />
      </Field>
      <FieldError errors={[errors.root]} />
    </FieldGroup>
  );
}

const ADD_FORM_ID = "lesson-add-form";

interface LessonDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  versionId: string;
  templateId: string;
  onCreated?: (lesson: TemplateLesson) => void;
}

/** Appends a lesson to the end of a draft version. */
export function LessonDialog({
  open,
  onOpenChange,
  versionId,
  templateId,
  onCreated,
}: LessonDialogProps) {
  const form = useLessonForm();
  const mutation = useCreateLesson(versionId, templateId);
  const handleApiError = useApiFormErrors(form);

  useEffect(() => {
    if (open) {
      form.reset(EMPTY_LESSON_FORM);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(toLessonInput(values), {
      onSuccess: (lesson) => {
        onOpenChange(false);
        onCreated?.(lesson);
      },
      onError: handleApiError,
    });
  });

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Thêm buổi học"
      description="Buổi mới được xếp cuối danh sách; kéo lên xuống sau bằng nút mũi tên."
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={ADD_FORM_ID} disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Thêm"}
          </HvButton>
        </>
      }
    >
      <form id={ADD_FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <LessonFields form={form} idPrefix="lesson-add" />
      </form>
    </HvModal>
  );
}
