import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { textareaClassName } from "@/lib/forms/textarea-class";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateStage, useUpdateStage } from "../hooks/use-paths";
import {
  stageFormSchema,
  stageToForm,
  toStageInput,
  type Stage,
  type StageFormInput,
  type StageFormValues,
} from "../schemas/paths-schemas";

const EMPTY_FORM: StageFormInput = { name: "", goal: "" };

const FORM_ID = "stage-dialog-form";

export type StageDialogProps = {
  pathId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & ({ mode: "create" } | { mode: "edit"; stage: Stage });

/**
 * Name and goal of one stage. A new stage lands at the end of the path; its
 * courses are picked on the timeline afterwards.
 */
export function StageDialog(props: StageDialogProps) {
  const { pathId, open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<StageFormInput, unknown, StageFormValues>({
    resolver: zodResolver(stageFormSchema),
    defaultValues: editing ? stageToForm(props.stage) : EMPTY_FORM,
  });
  const createMutation = useCreateStage(pathId);
  const updateMutation = useUpdateStage(pathId);
  const handleApiError = useApiFormErrors(form);
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(props.mode === "edit" ? stageToForm(props.stage) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    const input = toStageInput(values);
    if (props.mode === "edit") {
      updateMutation.mutate(
        { stageId: props.stage.id, input },
        { onSuccess: () => onOpenChange(false), onError: handleApiError },
      );
      return;
    }
    createMutation.mutate(input, {
      onSuccess: () => onOpenChange(false),
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Đổi tên chặng" : "Thêm chặng"}
      description="Mỗi chặng chứa các khóa học học sinh nên đi qua trước khi sang chặng sau."
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
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="stage-name">Tên chặng</FieldLabel>
            <Input
              id="stage-name"
              placeholder="VD: Nền tảng"
              aria-invalid={Boolean(errors.name)}
              {...form.register("name")}
            />
            <FieldError errors={[errors.name]} />
          </Field>
          <Field data-invalid={Boolean(errors.goal)}>
            <FieldLabel htmlFor="stage-goal">Mục tiêu</FieldLabel>
            <textarea
              id="stage-goal"
              rows={3}
              placeholder="Học sinh đạt được gì sau chặng này"
              className={textareaClassName}
              aria-invalid={Boolean(errors.goal)}
              {...form.register("goal")}
            />
            <FieldError errors={[errors.goal]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
