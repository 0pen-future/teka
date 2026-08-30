import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateScoreSet, useUpdateScoreSet } from "../hooks/use-score-sets";
import { scoreSetInputSchema, type ScoreSet, type ScoreSetInput } from "../schemas/grading";

export interface ScoreSetEditorModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Present when editing an existing bộ điểm; absent for create. */
  scoreSet?: ScoreSet;
}

function toDefaults(scoreSet?: ScoreSet): ScoreSetInput {
  return {
    name: scoreSet?.name ?? "",
    components: scoreSet && scoreSet.components.length > 0 ? [...scoreSet.components] : [""],
  };
}

/**
 * Create/edit bộ điểm: a name plus an ordered list of column names. Row
 * order maps directly to the server's `position` index, so add/remove/move
 * here are the whole editing surface — no separate reorder step. Duplicate
 * (case-insensitive) and blank names are caught client-side by
 * `scoreSetInputSchema` before the request round-trips.
 */
export function ScoreSetEditorModal({ open, onOpenChange, scoreSet }: ScoreSetEditorModalProps) {
  const isEdit = Boolean(scoreSet);
  const form = useForm<ScoreSetInput>({
    resolver: zodResolver(scoreSetInputSchema),
    defaultValues: toDefaults(scoreSet),
  });
  const createMutation = useCreateScoreSet();
  const updateMutation = useUpdateScoreSet();
  const mutation = isEdit ? updateMutation : createMutation;
  const handleApiError = useApiFormErrors(form, { conflictField: "name" });

  // Re-seed on every open: the mounted form would otherwise keep the values
  // from whichever bộ điểm was last opened (or a discarded draft).
  useEffect(() => {
    if (open) {
      form.reset(toDefaults(scoreSet));
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, scoreSet]);

  const { errors } = form.formState;
  const components = form.watch("components");

  function setComponents(next: string[]) {
    form.setValue("components", next, { shouldValidate: true, shouldDirty: true });
  }

  function updateComponent(index: number, value: string) {
    setComponents(components.map((component, i) => (i === index ? value : component)));
  }

  function removeComponent(index: number) {
    setComponents(components.filter((_, i) => i !== index));
  }

  function addComponent() {
    setComponents([...components, ""]);
  }

  function moveComponent(index: number, direction: -1 | 1) {
    const target = index + direction;
    if (target < 0 || target >= components.length) {
      return;
    }
    const next = [...components];
    const [moved] = next.splice(index, 1);
    next.splice(target, 0, moved!);
    setComponents(next);
  }

  const onSubmit = form.handleSubmit((values) => {
    const input: ScoreSetInput = {
      name: values.name,
      components: values.components.map((component) => component.trim()),
    };
    if (isEdit && scoreSet) {
      updateMutation.mutate(
        { id: scoreSet.id, input },
        {
          onSuccess: () => {
            hvToast(`Đã lưu bộ điểm ${input.name}`, { variant: "success" });
            onOpenChange(false);
          },
          onError: handleApiError,
        },
      );
      return;
    }
    createMutation.mutate(input, {
      onSuccess: () => {
        hvToast(`Đã tạo bộ điểm ${input.name}`, { variant: "success" });
        onOpenChange(false);
      },
      onError: handleApiError,
    });
  });

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? "Sửa bộ điểm" : "Tạo bộ điểm mới"}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form="score-set-editor-form" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </>
      }
    >
      <form id="score-set-editor-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="score-set-name">Tên bộ điểm</FieldLabel>
            <Input
              id="score-set-name"
              placeholder="VD: Giữa kỳ"
              aria-invalid={Boolean(errors.name)}
              {...form.register("name")}
            />
            <FieldError errors={[errors.name]} />
          </Field>

          <Field data-invalid={Boolean(errors.components)}>
            <FieldLabel>Cột điểm</FieldLabel>
            <div className="flex flex-col gap-2">
              {components.map((component, index) => (
                <div key={index} className="flex items-center gap-1.5">
                  <span className="w-5 shrink-0 text-[12px] font-bold text-ink-400">
                    {index + 1}
                  </span>
                  <Input
                    aria-label={`Tên cột điểm ${index + 1}`}
                    value={component}
                    aria-invalid={Boolean(errors.components?.[index])}
                    onChange={(event) => updateComponent(index, event.target.value)}
                  />
                  <button
                    type="button"
                    aria-label={`Di chuyển cột điểm ${index + 1} lên`}
                    onClick={() => moveComponent(index, -1)}
                    disabled={index === 0}
                    className="px-1 py-1 text-ink-400 hover:text-mint-600 disabled:cursor-not-allowed disabled:opacity-30"
                  >
                    ↑
                  </button>
                  <button
                    type="button"
                    aria-label={`Di chuyển cột điểm ${index + 1} xuống`}
                    onClick={() => moveComponent(index, 1)}
                    disabled={index === components.length - 1}
                    className="px-1 py-1 text-ink-400 hover:text-mint-600 disabled:cursor-not-allowed disabled:opacity-30"
                  >
                    ↓
                  </button>
                  {components.length > 1 ? (
                    <button
                      type="button"
                      aria-label={`Xóa cột điểm ${index + 1}`}
                      onClick={() => removeComponent(index)}
                      className="px-1.5 py-1 text-[12.5px] font-extrabold text-coral-400 hover:text-coral-600"
                    >
                      Xóa
                    </button>
                  ) : null}
                </div>
              ))}
            </div>
            <FieldError
              errors={[
                ...components.map((_, index) => errors.components?.[index]),
                errors.components?.root,
              ]}
            />
            <button
              type="button"
              onClick={addComponent}
              disabled={components.length >= 10}
              className="mt-1 w-full rounded-[14px] border-2 border-dashed border-line-300 px-3.5 py-2 text-[12.5px] font-extrabold text-mint-600 hover:border-mint-400 hover:bg-mint-50 disabled:cursor-not-allowed disabled:opacity-40"
            >
              + Thêm cột điểm
            </button>
          </Field>

          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
