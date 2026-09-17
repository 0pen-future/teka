import { zodResolver } from "@hookform/resolvers/zod";
import { Suspense, lazy, useEffect, useState } from "react";
import { Controller, useForm } from "react-hook-form";
import type { UseMutationResult } from "@tanstack/react-query";

import {
  HvButton,
  HvConfirmDialog,
  HvModal,
  HvSegmented,
  HvSelect,
  hvToast,
} from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import type { ColumnId, KanbanColumn, KanbanError, TaskId } from "@/lib/kanban";

import type {
  AppTask,
  CreateTaskVariables,
  UpdateTaskVariables,
} from "../hooks/use-tasks-data-source";
import { applyKanbanFormError, kanbanErrorToastMessage } from "../lib/map-api-error";
import { normalizeIncoming } from "../lib/rich-text";
import { taskFormSchema, type TaskFormValues, type TaskPriority } from "../schemas/task-schemas";
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

const PRIORITY_OPTIONS: { value: TaskPriority; label: string }[] = [
  { value: "none", label: "Không" },
  { value: "low", label: "Thấp" },
  { value: "medium", label: "Trung bình" },
  { value: "high", label: "Cao" },
];

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
  /** Present in edit mode; absent when creating a new task. */
  task?: AppTask;
  /** The column a new task should land in (the column whose "+" was clicked). */
  defaultColumnId?: ColumnId;
  createTaskMutation: UseMutationResult<AppTask, KanbanError, CreateTaskVariables>;
  updateTaskMutation: UseMutationResult<AppTask, KanbanError, UpdateTaskVariables>;
  deleteTaskMutation: UseMutationResult<void, KanbanError, TaskId>;
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
 * an assignee's only lever on someone else's task is the move menu.
 */
export function TaskFormModal({
  open,
  onOpenChange,
  columns,
  members,
  currentUserId,
  canDelete,
  task,
  defaultColumnId,
  createTaskMutation,
  updateTaskMutation,
  deleteTaskMutation,
  onAnnounce,
}: TaskFormModalProps) {
  const mode = task ? "edit" : "create";
  const readOnly =
    mode === "edit" && currentUserId !== undefined && task !== undefined
      ? task.createdBy !== currentUserId
      : false;
  const canDeleteThisTask = mode === "edit" && !readOnly && canDelete;
  const [deleteOpen, setDeleteOpen] = useState(false);

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

  const isSaving = createTaskMutation.isPending || updateTaskMutation.isPending;

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
      } else if (task) {
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
      }
      onOpenChange(false);
    } catch (error) {
      applyKanbanFormError(form, error);
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  });

  const handleDelete = async () => {
    if (!task) return;
    try {
      await deleteTaskMutation.mutateAsync(task.id);
      onAnnounce(`Đã xoá công việc ${task.title}.`);
      hvToast("Đã xoá công việc", { variant: "success" });
      setDeleteOpen(false);
      onOpenChange(false);
    } catch (error) {
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  };

  const memberOptions = [
    { value: "", label: "Không giao" },
    ...members.map((member) => ({ value: member.teacher_id, label: member.display_name })),
  ];

  return (
    <>
      <HvModal
        open={open}
        onOpenChange={onOpenChange}
        title={mode === "create" ? "Tạo công việc" : "Chi tiết công việc"}
        footer={
          readOnly ? (
            <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
              Đóng
            </HvButton>
          ) : (
            <>
              {canDeleteThisTask ? (
                <HvButton
                  type="button"
                  variant="danger"
                  onClick={() => setDeleteOpen(true)}
                  className="mr-auto"
                >
                  Xoá
                </HvButton>
              ) : null}
              <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
                Huỷ
              </HvButton>
              <HvButton type="button" onClick={() => void onSubmit()} disabled={isSaving}>
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
              Bạn chỉ có thể xem công việc này. Dùng menu "Chuyển" trên bảng để đổi cột.
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
          {mode === "create" ? (
            <Field data-invalid={Boolean(errors.column_id)}>
              <FieldLabel htmlFor="task-column">Cột</FieldLabel>
              <HvSelect
                id="task-column"
                sheetTitle="Chọn cột"
                value={form.watch("column_id")}
                onValueChange={(value) =>
                  form.setValue("column_id", value, { shouldValidate: true })
                }
                options={columns.map((column) => ({ value: column.id, label: column.name }))}
              />
              <FieldError errors={[errors.column_id]} />
            </Field>
          ) : null}
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
          <Field>
            <FieldLabel>Độ ưu tiên</FieldLabel>
            <HvSegmented
              aria-label="Độ ưu tiên"
              block
              value={form.watch("priority")}
              onValueChange={(value) => form.setValue("priority", value)}
              options={PRIORITY_OPTIONS}
            />
          </Field>
          <FieldError errors={[errors.root]} />
        </form>
      </HvModal>
      <HvConfirmDialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        title="Xoá công việc này?"
        description="Công việc sẽ bị xoá khỏi bảng. Không thể hoàn tác."
        confirmLabel="Xoá"
        tone="danger"
        pending={deleteTaskMutation.isPending}
        onConfirm={() => void handleDelete()}
      />
    </>
  );
}
