import { useEffect } from "react";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { EMPTY_LESSON_FORM, useLessonForm, type LessonForm } from "../hooks/use-lesson-form";
import { useCreateLesson } from "../hooks/use-library";
import { textareaClassName } from "@/lib/forms/textarea-class";
import { toLessonInput, type TemplateLesson } from "../schemas/library-schemas";

interface LessonFieldsProps {
  form: LessonForm;
  /** Prefix for the input ids so two forms on one page never collide. */
  idPrefix: string;
}

/** The four lesson inputs, shared by the add dialog and the lesson page. */
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
      <Field data-invalid={Boolean(errors.objectives)}>
        <FieldLabel htmlFor={`${idPrefix}-objectives`}>Mục tiêu</FieldLabel>
        <textarea
          id={`${idPrefix}-objectives`}
          rows={3}
          className={textareaClassName}
          aria-invalid={Boolean(errors.objectives)}
          {...form.register("objectives")}
        />
        <FieldError errors={[errors.objectives]} />
      </Field>
      <Field data-invalid={Boolean(errors.duration_min)} className="sm:max-w-[220px]">
        <FieldLabel htmlFor={`${idPrefix}-duration`}>Thời lượng (phút)</FieldLabel>
        <Input
          id={`${idPrefix}-duration`}
          inputMode="numeric"
          placeholder="VD: 90"
          aria-invalid={Boolean(errors.duration_min)}
          {...form.register("duration_min")}
        />
        <FieldError errors={[errors.duration_min]} />
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
