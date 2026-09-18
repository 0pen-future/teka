import { zodResolver } from "@hookform/resolvers/zod";
import { CheckIcon, Trash2Icon } from "lucide-react";
import { Suspense, lazy, useEffect, useRef, useState } from "react";
import type { ChangeEvent } from "react";
import { Controller, useForm, type FieldErrors } from "react-hook-form";
import type { UseMutationResult } from "@tanstack/react-query";

import { HvButton, HvModal, HvSelect, hvToast } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { cn, formatDateTime } from "@/lib/utils";
import {
  asColumnId,
  type ColumnId,
  type KanbanColumn,
  type KanbanError,
  type TaskId,
} from "@/lib/kanban";

import type {
  AppTask,
  CreateTaskVariables,
  UpdateTaskVariables,
} from "../hooks/use-tasks-data-source";
import { quickDueOptions } from "../lib/due-state";
import { applyKanbanFormError, kanbanErrorToastMessage } from "../lib/map-api-error";
import { normalizeIncoming } from "../lib/rich-text";
import {
  TASK_PRIORITIES,
  taskFormSchema,
  type TaskFormValues,
  type TaskPriority,
  TITLE_MAX_LENGTH,
  TITLE_TOO_LONG_MESSAGE,
} from "../schemas/task-schemas";
import { PRIORITY_LABELS } from "./task-card-styles";
import { RichTextView } from "./rich-text-view";

// TipTap and ProseMirror are the heaviest part of the tasks feature and only
// matter once a task is opened for editing: keep them out of the board's chunk.
const TaskDescriptionEditor = lazy(() =>
  import("./task-description-editor").then((module) => ({
    default: module.TaskDescriptionEditor,
  })),
);

/** Same footprint as the editor (toolbar + surface + counter) so the modal does not jump when the chunk lands. */
function EditorSkeleton() {
  return (
    <div
      aria-busy
      aria-label="Đang tải trình soạn thảo"
      className="min-h-[164px] animate-pulse rounded-[14px] border-2 border-line-200 bg-cream-100"
    />
  );
}

/** Fallback copy for a move that failed for no reason the server spelled out (network, 5xx). */
const MOVE_FAILED_MESSAGE = "Không chuyển được việc, vui lòng thử lại.";

/**
 * Toast shown when a save is refused by the form's own validation. The
 * description field keeps its precise inline error ("tối đa 2000 ký tự");
 * the toast rephrases that one as an instruction, per the design's copy. Any
 * other description error (the raw-HTML cap) is passed through, so the toast
 * never contradicts the inline message.
 */
function validationToast(errors: FieldErrors<TaskFormValues>): string | undefined {
  if (errors.title?.message) return errors.title.message;
  if (errors.description?.message) {
    return errors.description.message === "Mô tả tối đa 2000 ký tự"
      ? "Mô tả vượt 2.000 ký tự, hãy rút gọn"
      : errors.description.message;
  }
  return errors.column_id?.message;
}

/** Radio-tile colors per priority: the idle dot plus the tinted checked state. */
const PRIORITY_TILE: Record<TaskPriority, { dot: string; checked: string }> = {
  none: { dot: "bg-ink-300", checked: "border-ink-900 bg-cream-100 text-ink-900" },
  low: { dot: "bg-sky-400", checked: "border-sky-300 bg-sky-50 text-sky-600" },
  medium: { dot: "bg-sun-400", checked: "border-sun-300 bg-sun-100 text-sun-600" },
  high: { dot: "bg-coral-400", checked: "border-coral-300 bg-coral-100 text-coral-600" },
};

type InlineConfirm = "delete" | "discard";

interface DirectoryOption {
  teacher_id: string;
  display_name: string;
}

export interface TaskFormModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  columns: KanbanColumn[];
  members: DirectoryOption[];
  currentUserId: string | undefined;
  canDelete: boolean;
  /** Whether the caller holds `tasks.edit` — gates the D12 assignee move-only branch, not derived from `task`. */
  canEdit: boolean;
  /** Present in edit mode; absent when creating a new task. */
  task?: AppTask;
  /** The column a new task should land in (the column whose "+" was clicked). */
  defaultColumnId?: ColumnId;
  createTaskMutation: UseMutationResult<AppTask, KanbanError, CreateTaskVariables>;
  updateTaskMutation: UseMutationResult<AppTask, KanbanError, UpdateTaskVariables>;
  deleteTaskMutation: UseMutationResult<void, KanbanError, TaskId>;
  restoreTaskMutation: UseMutationResult<AppTask, KanbanError, TaskId>;
  onMoveTask: (taskId: TaskId, columnId: ColumnId) => Promise<unknown>;
  onAnnounce: (message: string) => void;
}

function toFormValues(
  task: AppTask | undefined,
  defaultColumnId: ColumnId | undefined,
): TaskFormValues {
  if (task) {
    return {
      title: task.title,
      description: normalizeIncoming(task.description),
      column_id: task.columnId,
      assignee_id: task.assigneeId,
      priority: task.priority,
      due_on: task.dueOn ?? "",
    };
  }
  return {
    title: "",
    description: "",
    column_id: defaultColumnId ?? "",
    assignee_id: null,
    priority: "none",
    due_on: "",
  };
}

interface ConfirmPanelProps {
  tone: "danger" | "default";
  title: string;
  description: string;
  cancelLabel: string;
  confirmLabel: string;
  pending?: boolean;
  onCancel: () => void;
  onConfirm: () => void;
}

/**
 * The design's in-modal confirmation: a sheet anchored to the bottom of the
 * open dialog rather than a second stacked dialog, so the task being
 * deleted (or the edits being discarded) stays visible behind it. Focus
 * lands on the safe action; the modal's own Escape/overlay handling closes
 * this panel first (see `requestClose`).
 */
function ConfirmPanel({
  tone,
  title,
  description,
  cancelLabel,
  confirmLabel,
  pending = false,
  onCancel,
  onConfirm,
}: ConfirmPanelProps) {
  const cancelRef = useRef<HTMLButtonElement>(null);
  const titleId = `task-confirm-title-${tone}`;
  const descriptionId = `task-confirm-desc-${tone}`;

  useEffect(() => {
    cancelRef.current?.focus();
  }, []);

  return (
    <div
      role="alertdialog"
      aria-labelledby={titleId}
      aria-describedby={descriptionId}
      className={cn(
        "absolute inset-x-0 bottom-0 z-10 flex flex-col gap-3 rounded-b-[var(--radius-xl)] border-t-2 bg-white px-5 py-4",
        "shadow-[0_-12px_30px_-18px_rgba(28,58,49,0.3)]",
        "animate-[slideUp_var(--dur-base)_var(--ease-soft)] motion-reduce:animate-none",
        tone === "danger" ? "border-coral-300" : "border-line-200",
      )}
    >
      <b id={titleId} className="font-display text-[16px] font-extrabold text-ink-900">
        {title}
      </b>
      <p id={descriptionId} className="text-[13.5px] text-ink-500">
        {description}
      </p>
      <div className="flex flex-wrap justify-end gap-2">
        <HvButton ref={cancelRef} variant="ghost" size="sm" onClick={onCancel}>
          {cancelLabel}
        </HvButton>
        <HvButton
          variant={tone === "danger" ? "danger" : "primary"}
          size="sm"
          disabled={pending}
          icon={tone === "danger" ? <Trash2Icon aria-hidden className="size-4" /> : undefined}
          onClick={onConfirm}
        >
          {confirmLabel}
        </HvButton>
      </div>
    </div>
  );
}

/**
 * Create/edit modal. Editing a task the caller only holds as assignee (not
 * creator) renders read-only — the server's D3 rule (owner/creator/assignee)
 * would technically allow an assignee edit, but this app narrows it further:
 * an assignee's only lever on someone else's task is moving it to another
 * column (D12), gated by `canEdit` (the same `tasks.edit` permission the
 * board's own move affordances use), not derived from `task`.
 */
export function TaskFormModal({
  open,
  onOpenChange,
  columns,
  members,
  currentUserId,
  canDelete,
  canEdit,
  task,
  defaultColumnId,
  createTaskMutation,
  updateTaskMutation,
  deleteTaskMutation,
  restoreTaskMutation,
  onMoveTask,
  onAnnounce,
}: TaskFormModalProps) {
  const mode = task ? "edit" : "create";
  const readOnly =
    mode === "edit" && currentUserId !== undefined && task !== undefined
      ? task.createdBy !== currentUserId
      : false;
  const readOnlyNoEdit = readOnly && !canEdit;
  const canChangeColumn = mode === "edit" && (!readOnly || canEdit);
  const canDeleteThisTask = mode === "edit" && !readOnly && canDelete;
  const [confirm, setConfirm] = useState<InlineConfirm | null>(null);
  // Whichever footer button opened the confirm panel gets focus back when it closes.
  const confirmOpenerRef = useRef<HTMLElement | null>(null);

  const form = useForm<TaskFormValues>({
    resolver: zodResolver(taskFormSchema),
    defaultValues: toFormValues(task, defaultColumnId),
  });
  const { errors } = form.formState;

  useEffect(() => {
    if (open) {
      form.reset(toFormValues(task, defaultColumnId));
      setConfirm(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, task?.id]);

  const isSaving = form.formState.isSubmitting;

  const openConfirm = (kind: InlineConfirm) => {
    confirmOpenerRef.current =
      document.activeElement instanceof HTMLElement ? document.activeElement : null;
    setConfirm(kind);
  };

  const closeConfirm = () => {
    setConfirm(null);
    confirmOpenerRef.current?.focus();
    confirmOpenerRef.current = null;
  };

  /** Escape / overlay / Huỷ: an open confirm panel absorbs the first dismissal; a dirty form asks before closing. */
  const requestClose = () => {
    if (confirm) {
      closeConfirm();
      return;
    }
    if (form.formState.isDirty) {
      openConfirm("discard");
      return;
    }
    onOpenChange(false);
  };

  const columnNameFor = (columnId: string): string =>
    columns.find((column) => column.id === columnId)?.name ?? "";

  const onSubmit = form.handleSubmit(
    async (values) => {
      try {
        if (mode === "create") {
          await createTaskMutation.mutateAsync({
            title: values.title,
            description: values.description,
            columnId: values.column_id,
            assigneeId: values.assignee_id,
            priority: values.priority,
            dueOn: values.due_on === "" ? null : values.due_on,
          });
          onAnnounce(`Đã tạo công việc ${values.title}.`);
          hvToast("Đã lưu công việc", { variant: "success" });
          onOpenChange(false);
          return;
        }
        if (!task) return;

        const columnChanged = values.column_id !== task.columnId;

        if (readOnly && canEdit) {
          // D12: an assignee-only editor never writes task content — the
          // server's `CanWriteTask` would 403 it — so the only lever here is
          // the move, gated the same way the board's own move affordances are.
          if (columnChanged) {
            try {
              await onMoveTask(task.id, asColumnId(values.column_id));
            } catch (error) {
              form.setError("column_id", {
                type: "server",
                message: kanbanErrorToastMessage(error, MOVE_FAILED_MESSAGE),
              });
              return;
            }
            onAnnounce(`Đã chuyển "${task.title}" sang cột ${columnNameFor(values.column_id)}.`);
          }
          form.reset(values);
          onOpenChange(false);
          return;
        }

        await updateTaskMutation.mutateAsync({
          taskId: task.id,
          title: values.title,
          description: values.description,
          assigneeId: values.assignee_id,
          priority: values.priority,
          dueOn: values.due_on === "" ? null : values.due_on,
        });
        onAnnounce(`Đã cập nhật công việc ${values.title}.`);
        hvToast("Đã lưu công việc", { variant: "success" });

        if (columnChanged) {
          try {
            await onMoveTask(task.id, asColumnId(values.column_id));
          } catch (error) {
            // The content is already saved server-side; keep the modal open
            // instead of losing that context behind a toast (see plan's risk
            // assessment for "Lưu + move hai request").
            form.setError("column_id", {
              type: "server",
              message: kanbanErrorToastMessage(error, MOVE_FAILED_MESSAGE),
            });
            return;
          }
          onAnnounce(`Đã chuyển "${task.title}" sang cột ${columnNameFor(values.column_id)}.`);
        }
        form.reset(values);
        onOpenChange(false);
      } catch (error) {
        applyKanbanFormError(form, error);
        hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
      }
    },
    (invalid) => {
      const message = validationToast(invalid);
      if (message) hvToast(message, { variant: "danger" });
    },
  );

  const handleDelete = async () => {
    if (!task) return;
    const { id: taskId, title } = task;
    try {
      await deleteTaskMutation.mutateAsync(taskId);
      setConfirm(null);
      onOpenChange(false);
      onAnnounce(`Đã xoá công việc ${title}. Nhấn Hoàn tác trong 6 giây.`);
      hvToast(`Đã xoá "${title}"`, {
        variant: "success",
        duration: 6000,
        action: {
          label: "Hoàn tác",
          onClick: () => {
            restoreTaskMutation.mutate(taskId, {
              onSuccess: () => {
                onAnnounce(`Đã khôi phục công việc ${title}.`);
                hvToast("Đã khôi phục công việc", { variant: "success" });
              },
              onError: (error) => hvToast(kanbanErrorToastMessage(error), { variant: "danger" }),
            });
          },
        },
      });
    } catch (error) {
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  };

  const handleDiscard = () => {
    form.reset(toFormValues(task, defaultColumnId));
    setConfirm(null);
    onOpenChange(false);
  };

  const memberOptions = [
    { value: "", label: "Không giao" },
    ...members.map((member) => ({ value: member.teacher_id, label: member.display_name })),
  ];

  const currentColumn = task ? columns.find((column) => column.id === task.columnId) : undefined;
  const creatorName = task
    ? (members.find((member) => member.teacher_id === task.createdBy)?.display_name ?? "—")
    : undefined;
  const createdLabel = task?.createdAt ? formatDateTime(task.createdAt).slice(0, 10) : "—";

  const saveDisabled =
    readOnly && canEdit ? isSaving || !form.formState.dirtyFields.column_id : isSaving;

  const priority = form.watch("priority");

  return (
    <HvModal
      open={open}
      onOpenChange={(next) => {
        if (!next) requestClose();
      }}
      title={mode === "create" ? "Tạo công việc" : "Chi tiết công việc"}
      description={
        mode === "edit" && task ? (
          <span className="inline-flex flex-wrap items-center gap-x-2.5 gap-y-1.5">
            <span
              className={cn(
                "inline-flex items-center gap-1 rounded-full px-[9px] py-0.5 font-display text-[12px] font-extrabold",
                currentColumn?.isDone ? "bg-mint-50 text-mint-600" : "bg-sky-50 text-sky-500",
              )}
            >
              {currentColumn?.isDone ? <CheckIcon aria-hidden className="size-3" /> : null}
              {currentColumn?.name ?? "—"}
            </span>
            <span>
              {creatorName} tạo {createdLabel}
            </span>
          </span>
        ) : undefined
      }
      stickyFooter
      className="sm:max-w-[520px]"
      footer={
        <div
          // The inline confirm panel covers this footer without a portal, so
          // it stays tabbable unless the covered controls are taken out of the
          // tab order — otherwise Tab from the panel lands on "Lưu".
          inert={confirm !== null || undefined}
          className="-mx-6 -mb-6 flex flex-1 items-center gap-2.5 border-t-[1.5px] border-line-100 pb-4 pl-3 pr-4 pt-3"
        >
          {readOnlyNoEdit ? (
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              className="ml-auto"
              onClick={requestClose}
            >
              Đóng
            </HvButton>
          ) : (
            <>
              {canDeleteThisTask ? (
                <button
                  type="button"
                  onClick={() => openConfirm("delete")}
                  className="inline-flex min-h-11 items-center gap-1.5 rounded-[var(--radius-md)] px-3 font-display text-[14px] font-bold text-coral-600 hover:bg-coral-100 focus-visible:outline-none focus-visible:ring-4"
                >
                  <Trash2Icon aria-hidden className="size-4" />
                  Xoá
                </button>
              ) : null}
              <span aria-hidden className="flex-1" />
              {form.formState.isDirty ? (
                <span className="rounded-full bg-sun-100 px-2.5 py-1 font-body text-[12.5px] font-bold text-sun-600">
                  Chưa lưu
                </span>
              ) : null}
              <HvButton type="button" variant="ghost" size="sm" onClick={requestClose}>
                Huỷ
              </HvButton>
              <HvButton
                type="button"
                size="sm"
                onClick={() => void onSubmit()}
                disabled={saveDisabled}
              >
                {isSaving ? "Đang lưu…" : "Lưu"}
              </HvButton>
            </>
          )}
        </div>
      }
    >
      <form
        inert={confirm !== null || undefined}
        onSubmit={(event) => {
          event.preventDefault();
          void onSubmit();
        }}
        noValidate
        className="flex flex-col gap-[18px]"
      >
        {readOnly ? (
          <p className="rounded-[12px] bg-cream-100 p-2.5 text-[12.5px] text-ink-500">
            {canEdit ? "Bạn chỉ có thể đổi cột của việc này." : "Bạn chỉ có thể xem việc này."}
          </p>
        ) : null}
        <Field data-invalid={Boolean(errors.title)}>
          <div className="flex items-baseline gap-1">
            <FieldLabel htmlFor="task-title">Tiêu đề</FieldLabel>
            <span aria-hidden className="text-coral-500">
              *
            </span>
          </div>
          <Input
            id="task-title"
            required
            disabled={readOnly}
            aria-invalid={Boolean(errors.title)}
            {...form.register("title", {
              // Trim over-long input (typed or pasted) at the limit and say so,
              // instead of a silent native `maxLength` cut.
              onChange: (event: ChangeEvent<HTMLInputElement>) => {
                const { value } = event.target;
                if (value.length <= TITLE_MAX_LENGTH) return;
                form.setValue("title", value.slice(0, TITLE_MAX_LENGTH), {
                  shouldDirty: true,
                  shouldValidate: true,
                });
                hvToast(TITLE_TOO_LONG_MESSAGE, { variant: "danger" });
              },
            })}
          />
          <FieldError errors={[errors.title]} />
        </Field>
        <Field data-invalid={Boolean(errors.description)}>
          <FieldLabel id="task-description-label">Mô tả</FieldLabel>
          {readOnly && task ? (
            <RichTextView
              html={task.description}
              emptyText="Không có mô tả"
              className="rounded-[14px] bg-cream-100 px-3 py-2.5"
            />
          ) : (
            <Controller
              control={form.control}
              name="description"
              render={({ field }) => (
                <Suspense fallback={<EditorSkeleton />}>
                  <TaskDescriptionEditor
                    id="task-description"
                    value={field.value}
                    onChange={field.onChange}
                    onBlur={field.onBlur}
                    labelId="task-description-label"
                    invalid={Boolean(errors.description)}
                    describedBy={errors.description ? "task-description-error" : undefined}
                  />
                </Suspense>
              )}
            />
          )}
          <FieldError id="task-description-error" errors={[errors.description]} />
        </Field>
        <div className="grid grid-cols-2 gap-3.5 max-[520px]:grid-cols-1">
          <Field data-invalid={Boolean(errors.column_id)}>
            <FieldLabel htmlFor="task-column">Cột</FieldLabel>
            <HvSelect
              id="task-column"
              sheetTitle="Chọn cột"
              disabled={mode === "edit" && !canChangeColumn}
              value={form.watch("column_id")}
              onValueChange={(value) =>
                form.setValue("column_id", value, { shouldDirty: true, shouldValidate: true })
              }
              options={columns.map((column) => ({ value: column.id, label: column.name }))}
            />
            <FieldError errors={[errors.column_id]} />
          </Field>
          <Field>
            <FieldLabel htmlFor="task-assignee">Người phụ trách</FieldLabel>
            <HvSelect
              id="task-assignee"
              sheetTitle="Chọn người phụ trách"
              disabled={readOnly}
              value={form.watch("assignee_id") ?? ""}
              onValueChange={(value) =>
                form.setValue("assignee_id", value === "" ? null : value, { shouldDirty: true })
              }
              options={memberOptions}
            />
          </Field>
        </div>
        <Field>
          <FieldLabel htmlFor="task-due-on">Hạn</FieldLabel>
          <Input id="task-due-on" type="date" disabled={readOnly} {...form.register("due_on")} />
          <div className="mt-2 flex flex-wrap gap-1.5">
            {quickDueOptions(new Date()).map((option) => (
              <button
                key={option.label}
                type="button"
                disabled={readOnly}
                onClick={() =>
                  form.setValue("due_on", option.value, { shouldDirty: true, shouldValidate: true })
                }
                className={cn(
                  "min-h-8 rounded-full border-[1.5px] border-line-200 bg-white px-2.5 font-body text-[12.5px] font-bold text-ink-500",
                  "hover:border-mint-300 hover:bg-mint-50 hover:text-mint-700",
                  "focus-visible:outline-none focus-visible:ring-4 disabled:cursor-not-allowed disabled:opacity-50",
                )}
              >
                {option.label}
              </button>
            ))}
          </div>
        </Field>
        <Field>
          <FieldLabel>Độ ưu tiên</FieldLabel>
          <div
            role="radiogroup"
            aria-label="Độ ưu tiên"
            className="grid grid-cols-4 gap-1.5 max-[440px]:grid-cols-2"
          >
            {TASK_PRIORITIES.map((option) => {
              const checked = priority === option;
              return (
                <button
                  key={option}
                  type="button"
                  role="radio"
                  aria-checked={checked}
                  disabled={readOnly}
                  onClick={() => form.setValue("priority", option, { shouldDirty: true })}
                  className={cn(
                    "flex min-h-11 items-center justify-center gap-[7px] whitespace-nowrap rounded-[14px] border-2 border-line-200 bg-white px-2",
                    "font-display text-[14px] font-bold text-ink-500 hover:border-line-300 hover:bg-cream-50",
                    "focus-visible:outline-none focus-visible:ring-4 disabled:cursor-not-allowed disabled:opacity-50",
                    checked && PRIORITY_TILE[option].checked,
                  )}
                >
                  <span
                    aria-hidden
                    className={cn("size-[9px] shrink-0 rounded-full", PRIORITY_TILE[option].dot)}
                  />
                  {PRIORITY_LABELS[option]}
                </button>
              );
            })}
          </div>
        </Field>
        <FieldError errors={[errors.root]} />
      </form>
      {confirm === "delete" ? (
        <ConfirmPanel
          tone="danger"
          title="Xoá công việc này?"
          description="Công việc sẽ rời khỏi bảng. Bạn có 6 giây để hoàn tác."
          cancelLabel="Giữ lại"
          confirmLabel="Xoá"
          pending={deleteTaskMutation.isPending}
          onCancel={closeConfirm}
          onConfirm={() => void handleDelete()}
        />
      ) : confirm === "discard" ? (
        <ConfirmPanel
          tone="default"
          title="Bỏ thay đổi chưa lưu?"
          description="Tiêu đề, mô tả hoặc thuộc tính bạn vừa sửa sẽ không được giữ."
          cancelLabel="Tiếp tục sửa"
          confirmLabel="Bỏ thay đổi"
          onCancel={closeConfirm}
          onConfirm={handleDiscard}
        />
      ) : null}
    </HvModal>
  );
}
