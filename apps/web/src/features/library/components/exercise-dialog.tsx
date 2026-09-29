import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import { z } from "zod";

import { HvButton, HvConfirmDialog, HvModal, HvSelect, hvToast } from "@/components/hv";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateExercise, useDeleteExercise, useUpdateExercise } from "../hooks/use-library";
import { optionalText, type Exercise, type ExerciseInput } from "../schemas/library-schemas";

/** The fixed skill list of the exercise bank; a new exercise starts as "Tổng hợp". */
const EXERCISE_SKILLS = [
  "Tổng hợp",
  "Vocabulary",
  "Grammar",
  "Listening",
  "Reading",
  "Writing",
  "Speaking",
] as const;
const DEFAULT_SKILL = EXERCISE_SKILLS[0];

/** The select cannot carry `""` as a real option, so a legacy "not set" skill gets its own value. */
const NOT_SET = "none";

/**
 * Limits mirror the API's `ExerciseRequest` binding tags (`dto.go`): title
 * max 200, code max 20, skill/level max 50. The code is normalized the way
 * the bank shows it — upper case, inner spaces as `_` — before the pattern
 * check, so "pp1 u3l1" becomes PP1_U3L1; blank means "let the API generate
 * a BT-0001-style code" on create and "keep the current code" on edit.
 */
const exerciseDialogSchema = z.object({
  code: z
    .string()
    .transform((text) => text.trim().toUpperCase().replace(/\s+/g, "_"))
    .pipe(
      z
        .string()
        .max(20, "Tối đa 20 ký tự")
        .regex(/^[A-Z0-9_-]*$/, "Chỉ dùng chữ, số, dấu gạch ngang và gạch dưới"),
    ),
  level: optionalText(50),
  title: z.string().trim().min(1, "Nhập tên bài tập").max(200, "Tối đa 200 ký tự"),
  skill: z.string().max(50, "Tối đa 50 ký tự"),
});
type ExerciseDialogFormInput = z.input<typeof exerciseDialogSchema>;
type ExerciseDialogFormValues = z.output<typeof exerciseDialogSchema>;

function blankToNull(text: string): string | null {
  return text === "" ? null : text;
}

function toDialogForm(exercise: Exercise): ExerciseDialogFormInput {
  return {
    code: exercise.code,
    level: exercise.level ?? "",
    title: exercise.title,
    skill: exercise.skill ?? "",
  };
}

function emptyForm(level: string | null | undefined): ExerciseDialogFormInput {
  return { code: "", level: level ?? "", title: "", skill: DEFAULT_SKILL };
}

/**
 * The dialog edits only the v5 fields; difficulty, description and tags stay
 * real API fields, so an edit sends back what the exercise already holds
 * (the request replaces every field) and a create leaves them unset.
 */
function toDialogInput(values: ExerciseDialogFormValues, current?: Exercise): ExerciseInput {
  return {
    title: values.title,
    code: blankToNull(values.code),
    skill: blankToNull(values.skill),
    level: blankToNull(values.level),
    description: current?.description ?? null,
    difficulty: current?.difficulty ?? null,
    tags: current?.tags ?? [],
  };
}

/**
 * The fixed list, plus the current value when an older exercise carries a
 * skill outside it (or none), so `HvSelect` still shows what is stored and
 * saving without touching the select does not rewrite it.
 */
function skillOptionsFor(current: string | null | undefined) {
  const options: { value: string; label: string }[] = EXERCISE_SKILLS.map((skill) => ({
    value: skill,
    label: skill,
  }));
  if (current === null) return [{ value: NOT_SET, label: "Chưa đặt" }, ...options];
  if (current !== undefined && !options.some((option) => option.value === current)) {
    return [...options, { value: current, label: current }];
  }
  return options;
}

const FORM_ID = "exercise-dialog-form";

export type ExerciseDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | {
      mode: "create";
      /**
       * Set when the dialog is opened from a template lesson: the title says
       * the new exercise joins that lesson, and "Cấp độ" starts from the
       * template's level.
       */
      lesson?: { level: string | null };
      onCreated: (exercise: Exercise) => void;
    }
  | {
      mode: "edit";
      exercise: Exercise;
      onSaved: (exercise: Exercise) => void;
      /** Offers "Xoá" in the footer when given; the bank is the only caller that deletes. */
      onDeleted?: (exercise: Exercise) => void;
    }
);

/** "Tạo bài tập" / "Sửa bài tập": one exercise of the center-wide bank. */
export function ExerciseDialog(props: ExerciseDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit" ? props.exercise : undefined;
  const lesson = props.mode === "create" ? props.lesson : undefined;
  const form = useForm<ExerciseDialogFormInput, unknown, ExerciseDialogFormValues>({
    resolver: zodResolver(exerciseDialogSchema),
    defaultValues: editing ? toDialogForm(editing) : emptyForm(lesson?.level),
  });
  const createMutation = useCreateExercise();
  const updateMutation = useUpdateExercise(editing?.id ?? "");
  const handleApiError = useApiFormErrors(form);
  const [deleting, setDeleting] = useState(false);
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(editing ? toDialogForm(editing) : emptyForm(lesson?.level));
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    if (props.mode === "edit") {
      updateMutation.mutate(toDialogInput(values, props.exercise), {
        onSuccess: (exercise) => {
          onOpenChange(false);
          props.onSaved(exercise);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(toDialogInput(values), {
      onSuccess: (exercise) => {
        onOpenChange(false);
        props.onCreated(exercise);
      },
      onError: handleApiError,
    });
  });

  function requestDelete(exercise: Exercise) {
    if (exercise.lesson_count > 0) {
      hvToast(`Còn ${exercise.lesson_count} buổi mẫu dùng ${exercise.code} — gỡ khỏi buổi trước`, {
        variant: "danger",
      });
      return;
    }
    setDeleting(true);
  }

  const title = editing
    ? "Sửa bài tập"
    : lesson
      ? "Tạo bài tập — thêm vào buổi này"
      : "Tạo bài tập";
  const onDeleted = props.mode === "edit" ? props.onDeleted : undefined;
  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={title}
      description="Mã dùng để tra cứu và sao chép nhanh."
      footer={
        <>
          {editing && onDeleted ? (
            <HvButton
              type="button"
              variant="ghost"
              className="mr-auto text-coral-600"
              disabled={pending}
              onClick={() => requestDelete(editing)}
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
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.code)}>
              <FieldLabel htmlFor="exercise-code">Mã bài tập</FieldLabel>
              <Input
                id="exercise-code"
                placeholder="VD: PP1_U3L1"
                aria-invalid={Boolean(errors.code)}
                {...form.register("code")}
              />
              {editing ? null : <FieldDescription>Để trống để tự sinh mã BT-…</FieldDescription>}
              <FieldError errors={[errors.code]} />
            </Field>
            <Field data-invalid={Boolean(errors.level)}>
              <FieldLabel htmlFor="exercise-level">Cấp độ</FieldLabel>
              <Input
                id="exercise-level"
                placeholder="VD: Prepare 1"
                aria-invalid={Boolean(errors.level)}
                {...form.register("level")}
              />
              <FieldError errors={[errors.level]} />
            </Field>
          </div>
          <Field data-invalid={Boolean(errors.title)}>
            <FieldLabel htmlFor="exercise-title">Tên bài tập</FieldLabel>
            <Input
              id="exercise-title"
              placeholder="VD: PP1, U3L1"
              aria-invalid={Boolean(errors.title)}
              {...form.register("title")}
            />
            <FieldError errors={[errors.title]} />
          </Field>
          <Field data-invalid={Boolean(errors.skill)}>
            <FieldLabel htmlFor="exercise-skill">Kỹ năng</FieldLabel>
            <Controller
              control={form.control}
              name="skill"
              render={({ field }) => (
                <HvSelect
                  id="exercise-skill"
                  options={skillOptionsFor(editing ? editing.skill : undefined)}
                  value={field.value === "" ? NOT_SET : field.value}
                  onValueChange={(value) => field.onChange(value === NOT_SET ? "" : value)}
                  sheetTitle="Chọn kỹ năng"
                  searchThreshold={Infinity}
                  aria-invalid={Boolean(errors.skill)}
                  className="w-full"
                />
              )}
            />
            <FieldError errors={[errors.skill]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
      {editing && onDeleted ? (
        <ExerciseDeleteConfirm
          exercise={editing}
          open={deleting}
          onOpenChange={setDeleting}
          onDeleted={(exercise) => {
            onOpenChange(false);
            onDeleted(exercise);
          }}
        />
      ) : null}
    </HvModal>
  );
}

/**
 * Stacked over the edit dialog. The API still refuses an exercise a lesson
 * uses (a link may appear after the bank row loaded); that refusal closes
 * the prompt and surfaces as a toast carrying the server's message.
 */
function ExerciseDeleteConfirm({
  exercise,
  open,
  onOpenChange,
  onDeleted,
}: {
  exercise: Exercise;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onDeleted: (exercise: Exercise) => void;
}) {
  const remove = useDeleteExercise();
  return (
    <HvConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title={`Xoá bài tập ${exercise.code}?`}
      description="Xoá vĩnh viễn khỏi ngân hàng bài tập."
      confirmLabel="Xoá"
      tone="danger"
      pending={remove.isPending}
      onConfirm={() =>
        remove.mutate(exercise.id, {
          onSuccess: () => {
            onOpenChange(false);
            onDeleted(exercise);
          },
          onError: (error) => {
            onOpenChange(false);
            hvToast(error instanceof ApiError ? error.message : "Không xoá được bài tập.", {
              variant: "danger",
            });
          },
        })
      }
    />
  );
}
