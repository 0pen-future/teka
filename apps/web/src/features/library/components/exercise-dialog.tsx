import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { HvButton, HvModal, HvSelect } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateExercise, useUpdateExercise } from "../hooks/use-library";
import { formatDifficulty } from "../lib/library-labels";
import { textareaClassName } from "@/lib/forms/textarea-class";
import {
  exerciseFormSchema,
  optionalText,
  toExerciseForm,
  toExerciseInput,
  type Exercise,
  type ExerciseInput,
} from "../schemas/library-schemas";

/**
 * The shared schema does not carry code/skill/level yet (the exercise
 * dialog used to leave them unset on the wire); this dialog is the one
 * place that edits them, so it extends the base form schema locally
 * instead of widening it for every other caller. Limits mirror the API's
 * `ExerciseRequest` binding tags (`dto.go`): code max 20, skill/level max 50.
 */
const exerciseDialogSchema = exerciseFormSchema.extend({
  code: z
    .string()
    .trim()
    .max(20, "Tối đa 20 ký tự")
    .regex(/^[A-Za-z0-9-]*$/, "Chỉ dùng chữ, số và dấu gạch ngang"),
  skill: optionalText(50),
  level: optionalText(50),
});
type ExerciseDialogFormInput = z.input<typeof exerciseDialogSchema>;
type ExerciseDialogFormValues = z.output<typeof exerciseDialogSchema>;

const EMPTY_FORM: ExerciseDialogFormInput = {
  title: "",
  description: "",
  difficulty: "",
  code: "",
  skill: "",
  level: "",
  tags: "",
};

function toDialogForm(exercise: Exercise): ExerciseDialogFormInput {
  return {
    ...toExerciseForm(exercise),
    code: exercise.code,
    skill: exercise.skill ?? "",
    level: exercise.level ?? "",
  };
}

function blankToNull(text: string): string | null {
  return text === "" ? null : text;
}

function toDialogInput(values: ExerciseDialogFormValues): ExerciseInput {
  return {
    ...toExerciseInput(values),
    code: blankToNull(values.code),
    skill: blankToNull(values.skill),
    level: blankToNull(values.level),
  };
}

const FORM_ID = "exercise-dialog-form";

/** The select cannot carry `""` as a real option, so "not set" gets its own value. */
const NOT_SET = "none";

const difficultyOptions = [
  { value: NOT_SET, label: "Chưa đặt" },
  ...[1, 2, 3, 4, 5].map((level) => ({ value: String(level), label: formatDifficulty(level) })),
];

export type ExerciseDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (exercise: Exercise) => void }
  | { mode: "edit"; exercise: Exercise; onSaved: (exercise: Exercise) => void }
);

/** Create/edit form for one catalog exercise; difficulty is an optional 1–5 level. */
export function ExerciseDialog(props: ExerciseDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<ExerciseDialogFormInput, unknown, ExerciseDialogFormValues>({
    resolver: zodResolver(exerciseDialogSchema),
    defaultValues: editing ? toDialogForm(props.exercise) : EMPTY_FORM,
  });
  const createMutation = useCreateExercise();
  const updateMutation = useUpdateExercise(editing ? props.exercise.id : "");
  const handleApiError = useApiFormErrors(form);
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(props.mode === "edit" ? toDialogForm(props.exercise) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    const input: ExerciseInput = toDialogInput(values);
    if (props.mode === "edit") {
      updateMutation.mutate(input, {
        onSuccess: (exercise) => {
          onOpenChange(false);
          props.onSaved(exercise);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(input, {
      onSuccess: (exercise) => {
        onOpenChange(false);
        props.onCreated(exercise);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Sửa bài tập" : "Thêm bài tập"}
      description="Bài tập dùng chung cho cả trung tâm; gắn vào buổi học mẫu trong trang buổi học."
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
              <FieldLabel htmlFor="exercise-title">Tên bài tập</FieldLabel>
              <Input
                id="exercise-title"
                placeholder="VD: Bài 1: Tập hợp"
                aria-invalid={Boolean(errors.title)}
                {...form.register("title")}
              />
              <FieldError errors={[errors.title]} />
            </Field>
            <Field data-invalid={Boolean(errors.difficulty)}>
              <FieldLabel htmlFor="exercise-difficulty">Độ khó</FieldLabel>
              <Controller
                control={form.control}
                name="difficulty"
                render={({ field }) => (
                  <HvSelect
                    id="exercise-difficulty"
                    options={difficultyOptions}
                    value={field.value === "" ? NOT_SET : field.value}
                    onValueChange={(value) => field.onChange(value === NOT_SET ? "" : value)}
                    sheetTitle="Chọn độ khó"
                    searchThreshold={Infinity}
                    aria-invalid={Boolean(errors.difficulty)}
                    className="w-full"
                  />
                )}
              />
              <FieldError errors={[errors.difficulty]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field data-invalid={Boolean(errors.code)}>
              <FieldLabel htmlFor="exercise-code">Mã</FieldLabel>
              <Input
                id="exercise-code"
                placeholder="Tự sinh nếu bỏ trống"
                aria-invalid={Boolean(errors.code)}
                {...form.register("code")}
              />
              <FieldError errors={[errors.code]} />
            </Field>
            <Field data-invalid={Boolean(errors.skill)}>
              <FieldLabel htmlFor="exercise-skill">Kỹ năng</FieldLabel>
              <Input
                id="exercise-skill"
                placeholder="VD: Đọc hiểu"
                aria-invalid={Boolean(errors.skill)}
                {...form.register("skill")}
              />
              <FieldError errors={[errors.skill]} />
            </Field>
            <Field data-invalid={Boolean(errors.level)}>
              <FieldLabel htmlFor="exercise-level">Cấp độ</FieldLabel>
              <Input
                id="exercise-level"
                placeholder="VD: Cơ bản"
                aria-invalid={Boolean(errors.level)}
                {...form.register("level")}
              />
              <FieldError errors={[errors.level]} />
            </Field>
          </div>
          <Field data-invalid={Boolean(errors.description)}>
            <FieldLabel htmlFor="exercise-description">Mô tả</FieldLabel>
            <textarea
              id="exercise-description"
              rows={3}
              className={textareaClassName}
              aria-invalid={Boolean(errors.description)}
              {...form.register("description")}
            />
            <FieldError errors={[errors.description]} />
          </Field>
          <Field data-invalid={Boolean(errors.tags)}>
            <FieldLabel htmlFor="exercise-tags">Thẻ</FieldLabel>
            <Input
              id="exercise-tags"
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
