import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateTemplate, useUpdateTemplate } from "../hooks/use-library";
import { textareaClassName } from "@/lib/forms/textarea-class";
import {
  templateFormSchema,
  toTemplateForm,
  toTemplateInput,
  type ProgramTemplate,
  type TemplateFormInput,
  type TemplateFormValues,
} from "../schemas/library-schemas";

const EMPTY_FORM: TemplateFormInput = {
  code: "",
  name: "",
  subject: "",
  level: "",
  description: "",
};

const FORM_ID = "template-dialog-form";

export type TemplateDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (template: ProgramTemplate) => void }
  | { mode: "edit"; template: ProgramTemplate; onSaved?: (template: ProgramTemplate) => void }
);

/**
 * Create/edit form for a program template's own fields (code, name, subject,
 * level, description). Versions and lessons have their own flows on the
 * detail page. A duplicate code comes back as CONFLICT and lands on the code
 * input rather than the form footer.
 */
export function TemplateDialog(props: TemplateDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<TemplateFormInput, unknown, TemplateFormValues>({
    resolver: zodResolver(templateFormSchema),
    defaultValues: editing ? toTemplateForm(props.template) : EMPTY_FORM,
  });
  const createMutation = useCreateTemplate();
  const updateMutation = useUpdateTemplate(editing ? props.template.id : "");
  const handleApiError = useApiFormErrors(form, { conflictField: "code" });
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(props.mode === "edit" ? toTemplateForm(props.template) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    const input = toTemplateInput(values);
    if (props.mode === "edit") {
      updateMutation.mutate(input, {
        onSuccess: (template) => {
          onOpenChange(false);
          props.onSaved?.(template);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(input, {
      onSuccess: (template) => {
        onOpenChange(false);
        props.onCreated(template);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Sửa chương trình mẫu" : "Tạo chương trình mẫu"}
      description="Mã dùng để nhận diện chương trình, nội dung buổi học soạn trong từng phiên bản."
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={FORM_ID} disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu" : "Tạo"}
          </HvButton>
        </>
      }
    >
      <form id={FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-[160px_1fr]">
            <Field data-invalid={Boolean(errors.code)}>
              <FieldLabel htmlFor="template-code">Mã chương trình</FieldLabel>
              <Input
                id="template-code"
                placeholder="VD: TOAN6"
                autoCapitalize="characters"
                aria-invalid={Boolean(errors.code)}
                {...form.register("code")}
              />
              <FieldError errors={[errors.code]} />
            </Field>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="template-name">Tên chương trình</FieldLabel>
              <Input
                id="template-name"
                placeholder="VD: Toán 6 cơ bản"
                aria-invalid={Boolean(errors.name)}
                {...form.register("name")}
              />
              <FieldError errors={[errors.name]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.subject)}>
              <FieldLabel htmlFor="template-subject">Môn học</FieldLabel>
              <Input
                id="template-subject"
                placeholder="VD: Toán"
                aria-invalid={Boolean(errors.subject)}
                {...form.register("subject")}
              />
              <FieldError errors={[errors.subject]} />
            </Field>
            <Field data-invalid={Boolean(errors.level)}>
              <FieldLabel htmlFor="template-level">Trình độ</FieldLabel>
              <Input
                id="template-level"
                placeholder="VD: Lớp 6"
                aria-invalid={Boolean(errors.level)}
                {...form.register("level")}
              />
              <FieldError errors={[errors.level]} />
            </Field>
          </div>
          <Field data-invalid={Boolean(errors.description)}>
            <FieldLabel htmlFor="template-description">Mô tả</FieldLabel>
            <textarea
              id="template-description"
              rows={3}
              className={textareaClassName}
              aria-invalid={Boolean(errors.description)}
              {...form.register("description")}
            />
            <FieldError errors={[errors.description]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
