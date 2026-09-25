import type { UseFormReturn } from "react-hook-form";

import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { textareaClassName } from "@/lib/forms/textarea-class";

import type { TemplateFormInput, TemplateFormValues } from "../schemas/library-schemas";

export type TemplateForm = UseFormReturn<TemplateFormInput, unknown, TemplateFormValues>;

/**
 * The template's own fields (code, name, subject, level, description), shared
 * by the create/edit dialog and the first step of the create wizard so both
 * validate and label them identically.
 */
export function TemplateFields({
  form,
  idPrefix = "template",
}: {
  form: TemplateForm;
  idPrefix?: string;
}) {
  const { errors } = form.formState;
  return (
    <FieldGroup>
      <div className="grid gap-3 sm:grid-cols-[160px_1fr]">
        <Field data-invalid={Boolean(errors.code)}>
          <FieldLabel htmlFor={`${idPrefix}-code`}>Mã chương trình</FieldLabel>
          <Input
            id={`${idPrefix}-code`}
            placeholder="VD: TOAN6"
            autoCapitalize="characters"
            aria-invalid={Boolean(errors.code)}
            {...form.register("code")}
          />
          <FieldError errors={[errors.code]} />
        </Field>
        <Field data-invalid={Boolean(errors.name)}>
          <FieldLabel htmlFor={`${idPrefix}-name`}>Tên chương trình</FieldLabel>
          <Input
            id={`${idPrefix}-name`}
            placeholder="VD: Toán 6 cơ bản"
            aria-invalid={Boolean(errors.name)}
            {...form.register("name")}
          />
          <FieldError errors={[errors.name]} />
        </Field>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <Field data-invalid={Boolean(errors.subject)}>
          <FieldLabel htmlFor={`${idPrefix}-subject`}>Môn học</FieldLabel>
          <Input
            id={`${idPrefix}-subject`}
            placeholder="VD: Toán"
            aria-invalid={Boolean(errors.subject)}
            {...form.register("subject")}
          />
          <FieldError errors={[errors.subject]} />
        </Field>
        <Field data-invalid={Boolean(errors.level)}>
          <FieldLabel htmlFor={`${idPrefix}-level`}>Trình độ</FieldLabel>
          <Input
            id={`${idPrefix}-level`}
            placeholder="VD: Lớp 6"
            aria-invalid={Boolean(errors.level)}
            {...form.register("level")}
          />
          <FieldError errors={[errors.level]} />
        </Field>
      </div>
      <Field data-invalid={Boolean(errors.description)}>
        <FieldLabel htmlFor={`${idPrefix}-description`}>Mô tả</FieldLabel>
        <textarea
          id={`${idPrefix}-description`}
          rows={3}
          className={textareaClassName}
          aria-invalid={Boolean(errors.description)}
          {...form.register("description")}
        />
        <FieldError errors={[errors.description]} />
      </Field>
      <FieldError errors={[errors.root]} />
    </FieldGroup>
  );
}
