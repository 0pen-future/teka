import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { Controller, useForm } from "react-hook-form";

import { HvButton, HvModal, HvSelect } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateMaterial, useUpdateMaterial } from "../hooks/use-library";
import { materialKindLabel } from "../lib/library-labels";
import { textareaClassName } from "../lib/textarea-class";
import {
  materialFormSchema,
  materialKindSchema,
  toMaterialForm,
  toMaterialInput,
  type Material,
  type MaterialFormInput,
  type MaterialFormValues,
} from "../schemas/library-schemas";

const EMPTY_FORM: MaterialFormInput = {
  title: "",
  kind: "link",
  url: "",
  description: "",
  tags: "",
};

const FORM_ID = "material-dialog-form";

const kindOptions = materialKindSchema.options.map((kind) => ({
  value: kind,
  label: materialKindLabel[kind],
}));

export type MaterialDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (material: Material) => void }
  | { mode: "edit"; material: Material; onSaved: (material: Material) => void }
);

/**
 * Create/edit form for one catalog material. Tags travel as one
 * comma-separated input and are split on submit, mirroring how the API
 * trims and drops blanks.
 */
export function MaterialDialog(props: MaterialDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<MaterialFormInput, unknown, MaterialFormValues>({
    resolver: zodResolver(materialFormSchema),
    defaultValues: editing ? toMaterialForm(props.material) : EMPTY_FORM,
  });
  const createMutation = useCreateMaterial();
  const updateMutation = useUpdateMaterial(editing ? props.material.id : "");
  const handleApiError = useApiFormErrors(form);
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(props.mode === "edit" ? toMaterialForm(props.material) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    const input = toMaterialInput(values);
    if (props.mode === "edit") {
      updateMutation.mutate(input, {
        onSuccess: (material) => {
          onOpenChange(false);
          props.onSaved(material);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(input, {
      onSuccess: (material) => {
        onOpenChange(false);
        props.onCreated(material);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Sửa học liệu" : "Thêm học liệu"}
      description="Học liệu dùng chung cho cả trung tâm; gắn vào buổi học mẫu trong trang buổi học."
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={FORM_ID} disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu" : "Thêm"}
          </HvButton>
        </>
      }
    >
      <form id={FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-[1fr_160px]">
            <Field data-invalid={Boolean(errors.title)}>
              <FieldLabel htmlFor="material-title">Tên học liệu</FieldLabel>
              <Input
                id="material-title"
                placeholder="VD: Slide chương 1"
                aria-invalid={Boolean(errors.title)}
                {...form.register("title")}
              />
              <FieldError errors={[errors.title]} />
            </Field>
            <Field data-invalid={Boolean(errors.kind)}>
              <FieldLabel htmlFor="material-kind">Loại</FieldLabel>
              <Controller
                control={form.control}
                name="kind"
                render={({ field }) => (
                  <HvSelect
                    id="material-kind"
                    options={kindOptions}
                    value={field.value}
                    onValueChange={field.onChange}
                    sheetTitle="Chọn loại học liệu"
                    searchThreshold={Infinity}
                    aria-invalid={Boolean(errors.kind)}
                    className="w-full"
                  />
                )}
              />
              <FieldError errors={[errors.kind]} />
            </Field>
          </div>
          <Field data-invalid={Boolean(errors.url)}>
            <FieldLabel htmlFor="material-url">Đường dẫn</FieldLabel>
            <Input
              id="material-url"
              inputMode="url"
              placeholder="https://…"
              aria-invalid={Boolean(errors.url)}
              {...form.register("url")}
            />
            <FieldError errors={[errors.url]} />
          </Field>
          <Field data-invalid={Boolean(errors.description)}>
            <FieldLabel htmlFor="material-description">Mô tả</FieldLabel>
            <textarea
              id="material-description"
              rows={3}
              className={textareaClassName}
              aria-invalid={Boolean(errors.description)}
              {...form.register("description")}
            />
            <FieldError errors={[errors.description]} />
          </Field>
          <Field data-invalid={Boolean(errors.tags)}>
            <FieldLabel htmlFor="material-tags">Thẻ</FieldLabel>
            <Input
              id="material-tags"
              placeholder="Cách nhau bằng dấu phẩy, VD: chương 1, ôn tập"
              aria-invalid={Boolean(errors.tags)}
              {...form.register("tags")}
            />
            <FieldError errors={[errors.tags]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
