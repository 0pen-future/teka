import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useState } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal, HvSelect } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { textareaClassName } from "@/lib/forms/textarea-class";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreatePath, useUpdatePath } from "../hooks/use-paths";
import { pathStatusLabel } from "../lib/path-labels";
import {
  pathFormSchema,
  pathToForm,
  toPathInput,
  type LearningPath,
  type PathFormInput,
  type PathFormValues,
  type PathStatus,
} from "../schemas/paths-schemas";
import { PathDeleteConfirm } from "./path-delete-confirm";

const EMPTY_FORM: PathFormInput = { code: "", name: "", description: "", status: "draft" };

const FORM_ID = "path-dialog-form";

export type PathDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (path: LearningPath) => void }
  | {
      mode: "edit";
      path: LearningPath;
      onSaved?: (path: LearningPath) => void;
      onDeleted: (path: LearningPath) => void;
    }
);

/**
 * The v5 "Tạo / Sửa lộ trình học" form for a path's own fields; stages are
 * managed in the paths table. Editing also offers "Xoá", whose confirmation
 * stacks on top of the form so cancelling it returns to the edit. A duplicate
 * code comes back as CODE_TAKEN and lands on the code input rather than the
 * form footer.
 */
export function PathDialog(props: PathDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<PathFormInput, unknown, PathFormValues>({
    resolver: zodResolver(pathFormSchema),
    defaultValues: editing ? pathToForm(props.path) : EMPTY_FORM,
  });
  const createMutation = useCreatePath();
  const updateMutation = useUpdatePath(editing ? props.path.id : "");
  const handleApiError = useApiFormErrors(form, { conflictField: "code" });
  const pending = createMutation.isPending || updateMutation.isPending;
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (open) {
      setDeleting(false);
      form.reset(props.mode === "edit" ? pathToForm(props.path) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    if (props.mode === "edit") {
      updateMutation.mutate(toPathInput(values), {
        onSuccess: (path) => {
          onOpenChange(false);
          props.onSaved?.(path);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(toPathInput(values), {
      onSuccess: (path) => {
        onOpenChange(false);
        props.onCreated(path);
      },
      onError: handleApiError,
    });
  });

  // A retired path is reopened through the same form; a new one starts as
  // draft or active only.
  const statuses: PathStatus[] = editing ? ["draft", "active", "archived"] : ["draft", "active"];
  const status = form.watch("status");
  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Sửa lộ trình học" : "Tạo lộ trình học"}
      description="Tên và trạng thái. Chặng quản lý ngay trong bảng lộ trình."
      footer={
        <>
          {editing ? (
            <HvButton
              type="button"
              variant="ghost"
              className="mr-auto text-coral-600"
              disabled={pending}
              onClick={() => setDeleting(true)}
            >
              Xoá
            </HvButton>
          ) : null}
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={FORM_ID} disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu thay đổi" : "Tạo mới"}
          </HvButton>
        </>
      }
    >
      <form id={FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="path-name">Tên lộ trình</FieldLabel>
            <Input
              id="path-name"
              placeholder="VD: MOVERS"
              aria-invalid={Boolean(errors.name)}
              {...form.register("name")}
            />
            <FieldError errors={[errors.name]} />
          </Field>
          <Field data-invalid={Boolean(errors.status)}>
            <FieldLabel htmlFor="path-status">Trạng thái</FieldLabel>
            <HvSelect
              id="path-status"
              aria-label="Trạng thái"
              value={status}
              onValueChange={(next) =>
                form.setValue("status", next as PathStatus, {
                  shouldDirty: true,
                  shouldValidate: true,
                })
              }
              options={statuses.map((value) => ({ value, label: pathStatusLabel[value] }))}
              sheetTitle="Trạng thái lộ trình"
              searchThreshold={Infinity}
            />
            <FieldError errors={[errors.status]} />
          </Field>
          <Field data-invalid={Boolean(errors.code)}>
            <FieldLabel htmlFor="path-code">Mã lộ trình</FieldLabel>
            <Input
              id="path-code"
              placeholder="VD: LT-MOVERS"
              autoCapitalize="characters"
              aria-invalid={Boolean(errors.code)}
              {...form.register("code")}
            />
            <FieldError errors={[errors.code]} />
          </Field>
          <Field data-invalid={Boolean(errors.description)}>
            <FieldLabel htmlFor="path-description">Mô tả</FieldLabel>
            <textarea
              id="path-description"
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
      {props.mode === "edit" ? (
        <PathDeleteConfirm
          path={props.path}
          open={deleting}
          onOpenChange={setDeleting}
          onDeleted={(path) => {
            onOpenChange(false);
            props.onDeleted(path);
          }}
        />
      ) : null}
    </HvModal>
  );
}
