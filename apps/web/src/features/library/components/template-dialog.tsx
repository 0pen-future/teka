import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateTemplate, useUpdateTemplate } from "../hooks/use-library";
import { TemplateFields } from "./template-fields";
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
        <TemplateFields form={form} />
      </form>
    </HvModal>
  );
}
