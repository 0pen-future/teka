import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { HvButton, HvModal, HvNotice, HvSelect } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateMaterial, useUpdateMaterial } from "../hooks/use-library";
import { materialKindLabel } from "../lib/library-labels";
import { textareaClassName } from "@/lib/forms/textarea-class";
import {
  materialFormSchema,
  materialKindSchema,
  toMaterialForm,
  toMaterialInput,
  type Material,
  type MaterialFormInput,
  type MaterialFormValues,
  type MaterialKind,
} from "../schemas/library-schemas";

const EMPTY_FORM: MaterialFormInput = {
  title: "",
  kind: "link",
  url: "",
  description: "",
  tags: "",
};

const FORM_ID = "material-dialog-form";

function createForm(kind: MaterialFormInput["kind"] | undefined): MaterialFormInput {
  return kind ? { ...EMPTY_FORM, kind } : EMPTY_FORM;
}

/**
 * `other` stays readable on legacy rows but is not offered as a new choice;
 * `kindOptionsFor` puts it back only while editing an item that already has
 * it, so `HvSelect` still finds the current value among its own options
 * (otherwise it falls back to showing the placeholder instead of "Khác").
 */
const SELECTABLE_KINDS = materialKindSchema.options.filter((kind) => kind !== "other");

function kindOptionsFor(currentKind: MaterialKind | undefined) {
  const kinds = currentKind === "other" ? materialKindSchema.options : SELECTABLE_KINDS;
  return kinds.map((kind) => ({ value: kind, label: materialKindLabel[kind] }));
}

const urlPlaceholderByKind: Record<MaterialKind, string> = {
  video: "https://youtube.com/watch?v=…",
  audio: "https://…/bai-giang.mp3",
  image: "https://…/anh-minh-hoa.png",
  doc: "https://…/tai-lieu.pdf",
  note: "https://…/ghi-chu",
  live: "https://meet.google.com/xxx-yyyy-zzz",
  link: "https://…",
  other: "https://…",
};

/**
 * The bank keeps every material's URL required (the v5 decision — `note`
 * uses `description` for the write-up, not a blank link), so the dialog's
 * own schema tightens the shared form schema's optional URL locally rather
 * than editing it for every other caller.
 */
const requiredUrl = z
  .string()
  .trim()
  .min(1, "Bắt buộc nhập đường dẫn")
  .max(2000, "Tối đa 2000 ký tự")
  .refine(
    (text) => /^https?:\/\/[^/\s]+/i.test(text),
    "Đường dẫn phải bắt đầu bằng http:// hoặc https://",
  );
const materialDialogSchema = materialFormSchema.extend({ url: requiredUrl });

export type MaterialDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | {
      mode: "create";
      /** Kind preselected when the dialog opens from a "create <kind>" menu entry. */
      defaultKind?: MaterialFormInput["kind"];
      onCreated: (material: Material) => void;
    }
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
    resolver: zodResolver(materialDialogSchema),
    defaultValues: editing ? toMaterialForm(props.material) : createForm(props.defaultKind),
  });
  const createMutation = useCreateMaterial();
  const updateMutation = useUpdateMaterial(editing ? props.material.id : "");
  const handleApiError = useApiFormErrors(form);
  const pending = createMutation.isPending || updateMutation.isPending;
  const kind = form.watch("kind");
  const kindOptions = kindOptionsFor(editing ? props.material.kind : undefined);

  useEffect(() => {
    if (open) {
      form.reset(
        props.mode === "edit" ? toMaterialForm(props.material) : createForm(props.defaultKind),
      );
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
          {editing ? (
            <HvNotice tone="warning">Sửa nội dung này sẽ ảnh hưởng mọi buổi đang dùng</HvNotice>
          ) : null}
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
              placeholder={urlPlaceholderByKind[kind]}
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
