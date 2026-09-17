import { zodResolver } from "@hookform/resolvers/zod";
import { Suspense, lazy, useEffect, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import type { UseMutationResult } from "@tanstack/react-query";

import {
  HvBadge,
  HvButton,
  HvChip,
  HvConfirmDialog,
  HvModal,
  HvSelect,
  hvToast,
} from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { formatDateTime } from "@/lib/utils";
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
import { TASK_PRIORITIES, taskFormSchema, type TaskFormValues } from "../schemas/task-schemas";
import { PRIORITY_LABELS, PRIORITY_VARIANTS } from "./task-card-styles";
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
  const [deleteOpen, setDeleteOpen] = useState(false);
  const [discardOpen, setDiscardOpen] = useState(false);

  const form = useForm<TaskFormValues>({
    resolver: zodResolver(taskFormSchema),
    defaultValues: toFormValues(task, defaultColumnId),
  });
  const { errors } = form.formState;

  useEffect(() => {
    if (open) {
      form.reset(toFormValues(task, defaultColumnId));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, task?.id]);

  const isSaving = form.formState.isSubmitting;

  const requestClose = () => {
    if (form.formState.isDirty) {
      setDiscardOpen(true);
      return;
    }
    onOpenChange(false);
  };

  const columnNameFor = (columnId: string): string =>
    columns.find((column) => column.id === columnId)?.name ?? "";

  const onSubmit = form.handleSubmit(async (values) => {
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
        hvToast("Đã tạo công việc", { variant: "success" });
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
      hvToast("Đã cập nhật công việc", { variant: "success" });

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
  });

  const handleDelete = async () => {
    if (!task) return;
    const { id: taskId, title } = task;
    try {
      await deleteTaskMutation.mutateAsync(taskId);
      setDeleteOpen(false);
      onOpenChange(false);
      onAnnounce(`Đã xoá công việc ${title}. Nhấn Hoàn tác trong 6 giây.`);
      hvToast("Đã xoá công việc", {
        variant: "success",
        duration: 6000,
        action: {
          label: "Hoàn tác",
          onClick: () => {
            restoreTaskMutation.mutate(taskId, {
              onSuccess: () => onAnnounce(`Đã khôi phục công việc ${title}.`),
              onError: (error) => hvToast(kanbanErrorToastMessage(error), { variant: "danger" }),
            });
          },
        },
      });
    } catch (error) {
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  };

  const memberOptions = [
    { value: "", label: "Không giao" },
    ...members.map((member) => ({ value: member.teacher_id, label: member.display_name })),
  ];

  const currentColumnName = task ? columnNameFor(task.columnId) || "—" : undefined;
  const creatorName = task
    ? (members.find((member) => member.teacher_id === task.createdBy)?.display_name ?? "—")
    : undefined;
  const createdLabel = task?.createdAt ? formatDateTime(task.createdAt).slice(0, 5) : "—";

  const saveDisabled =
    readOnly && canEdit ? isSaving || !form.formState.dirtyFields.column_id : isSaving;

  return (
    <>
      <HvModal
        open={open}
        onOpenChange={(next) => {
          if (!next) requestClose();
        }}
        title={mode === "create" ? "Tạo công việc" : "Chi tiết công việc"}
        description={
          mode === "edit" && task
            ? `${currentColumnName} · ${creatorName} · Tạo ${createdLabel}`
            : undefined
        }
        stickyFooter
        footer={
          readOnlyNoEdit ? (
            <HvButton type="button" variant="ghost" onClick={requestClose}>
              Đóng
            </HvButton>
          ) : (
            <>
              <div className="mr-auto flex items-center gap-2 empty:hidden">
                {canDeleteThisTask ? (
                  <HvButton
                    type="button"
                    variant="ghost"
                    size="sm"
                    onClick={() => setDeleteOpen(true)}
                    className="text-coral-500 hover:bg-coral-100"
                  >
                    Xoá
                  </HvButton>
                ) : null}
                {form.formState.isDirty ? (
                  <HvBadge variant="warning" size="sm">
                    Chưa lưu
                  </HvBadge>
                ) : null}
              </div>
              <HvButton type="button" variant="ghost" onClick={requestClose}>
                Huỷ
              </HvButton>
              <HvButton type="button" onClick={() => void onSubmit()} disabled={saveDisabled}>
                {isSaving ? "Đang lưu…" : "Lưu"}
              </HvButton>
            </>
          )
        }
      >
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void onSubmit();
          }}
          noValidate
          className="flex flex-col gap-4"
        >
          {readOnly ? (
            <p className="rounded-[12px] bg-cream-100 p-2.5 text-[12.5px] text-ink-500">
              {canEdit ? "Bạn chỉ có thể đổi cột của việc này." : "Bạn chỉ có thể xem việc này."}
            </p>
          ) : null}
          <Field data-invalid={Boolean(errors.title)}>
            <FieldLabel htmlFor="task-title">Tiêu đề</FieldLabel>
            <Input
              id="task-title"
              disabled={readOnly}
              aria-invalid={Boolean(errors.title)}
              {...form.register("title")}
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
          <div className="grid grid-cols-2 gap-3">
            <Field>
              <FieldLabel htmlFor="task-assignee">Người phụ trách</FieldLabel>
              <HvSelect
                id="task-assignee"
                sheetTitle="Chọn người phụ trách"
                disabled={readOnly}
                value={form.watch("assignee_id") ?? ""}
                onValueChange={(value) => form.setValue("assignee_id", value === "" ? null : value)}
                options={memberOptions}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="task-due-on">Hạn</FieldLabel>
              <Input
                id="task-due-on"
                type="date"
                disabled={readOnly}
                {...form.register("due_on")}
              />
            </Field>
          </div>
          <div className="-mt-2 flex flex-wrap gap-1.5">
            {quickDueOptions(new Date()).map((option) => (
              <HvChip
                key={option.label}
                size="sm"
                disabled={readOnly}
                pressed={form.watch("due_on") === option.value}
                onClick={() =>
                  form.setValue("due_on", option.value, { shouldDirty: true, shouldValidate: true })
                }
              >
                {option.label}
              </HvChip>
            ))}
          </div>
          <Field>
            <FieldLabel>Độ ưu tiên</FieldLabel>
            <div
              role="radiogroup"
              aria-label="Độ ưu tiên"
              className="grid grid-cols-4 gap-2 max-[440px]:grid-cols-2"
            >
              {TASK_PRIORITIES.map((priority) => (
                <HvChip
                  key={priority}
                  role="radio"
                  dot={PRIORITY_VARIANTS[priority]}
                  disabled={readOnly}
                  pressed={form.watch("priority") === priority}
                  onClick={() => form.setValue("priority", priority, { shouldDirty: true })}
                >
                  {PRIORITY_LABELS[priority]}
                </HvChip>
              ))}
            </div>
          </Field>
          <FieldError errors={[errors.root]} />
        </form>
      </HvModal>
      <HvConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Xoá công việc này?"
        description="Có thể hoàn tác trong vài giây sau khi xoá."
        confirmLabel="Xoá"
        tone="danger"
        pending={deleteTaskMutation.isPending}
        onConfirm={() => void handleDelete()}
      />
      <HvConfirmDialog
        open={discardOpen}
        onOpenChange={setDiscardOpen}
        title="Bỏ thay đổi?"
        description="Các thay đổi chưa lưu sẽ mất."
        confirmLabel="Bỏ thay đổi"
        cancelLabel="Tiếp tục sửa"
        tone="danger"
        onConfirm={() => {
          form.reset(toFormValues(task, defaultColumnId));
          setDiscardOpen(false);
          onOpenChange(false);
        }}
      />
    </>
  );
}
