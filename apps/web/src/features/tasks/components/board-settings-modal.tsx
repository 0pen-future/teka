import { GripVertical } from "lucide-react";
import { Switch } from "radix-ui";
import { useState } from "react";
import type { UseMutationResult } from "@tanstack/react-query";

import { HvButton, HvIcon, HvModal, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import type { AdjacentDirection } from "@/lib/kanban";
import { isKanbanError, type ColumnId, type KanbanColumn, type KanbanError } from "@/lib/kanban";
import { cn } from "@/lib/utils";

import type {
  AppTask,
  CreateColumnVariables,
  DeleteColumnVariables,
  UpdateColumnVariables,
} from "../hooks/use-tasks-data-source";
import { kanbanErrorToastMessage } from "../lib/map-api-error";
import { DeleteColumnDialog } from "./delete-column-dialog";

const MAX_COLUMNS = 8;

function DoneSwitch({
  checked,
  onCheckedChange,
  disabled,
  label,
}: {
  checked: boolean;
  onCheckedChange: (checked: boolean) => void;
  disabled?: boolean;
  label: string;
}) {
  return (
    <Switch.Root
      checked={checked}
      onCheckedChange={onCheckedChange}
      disabled={disabled}
      aria-label={label}
      className={cn(
        "relative inline-flex h-6 w-11 shrink-0 cursor-pointer items-center rounded-full transition-colors",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        "disabled:cursor-not-allowed disabled:opacity-50",
        checked ? "bg-mint-400" : "bg-line-200",
      )}
    >
      <Switch.Thumb
        className={cn(
          "block size-5 translate-x-0.5 rounded-full bg-white shadow-sm transition-transform",
          checked && "translate-x-[22px]",
        )}
      />
    </Switch.Root>
  );
}

interface ColumnRowProps {
  column: KanbanColumn;
  taskCount: number;
  index: number;
  count: number;
  updateColumnMutation: UseMutationResult<KanbanColumn, KanbanError, UpdateColumnVariables>;
  onReorder: (direction: AdjacentDirection) => void;
  onRequestDelete: () => void;
  disableDelete: boolean;
}

function ColumnRow({
  column,
  taskCount,
  index,
  count,
  updateColumnMutation,
  onReorder,
  onRequestDelete,
  disableDelete,
}: ColumnRowProps) {
  const [name, setName] = useState(column.name);
  const [nameError, setNameError] = useState<string | undefined>(undefined);
  // Renders can't call setState in an effect for this; the recommended
  // alternative (react.dev: "adjusting state based on a prop change") is to
  // detect the change during render and reset synchronously.
  const [syncedName, setSyncedName] = useState(column.name);
  if (column.name !== syncedName) {
    setSyncedName(column.name);
    setName(column.name);
  }

  const commitName = async () => {
    const trimmed = name.trim();
    if (trimmed === column.name || trimmed === "") {
      setName(column.name);
      return;
    }
    try {
      await updateColumnMutation.mutateAsync({ columnId: column.id, name: trimmed });
      setNameError(undefined);
    } catch (error) {
      setName(column.name);
      if (isKanbanError(error) && error.kind === "validation") {
        setNameError(error.fields.name ?? kanbanErrorToastMessage(error));
      } else {
        hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
      }
    }
  };

  return (
    <div className="flex flex-col gap-1 rounded-[14px] border border-line-200 bg-white p-2.5">
      <div className="flex items-center gap-2">
        <GripVertical aria-hidden className="size-4 shrink-0 text-ink-300" />
        <div className="min-w-0 flex-1">
          <Input
            aria-label={`Tên cột ${column.name}`}
            aria-invalid={Boolean(nameError)}
            value={name}
            onChange={(event) => setName(event.target.value)}
            onBlur={() => void commitName()}
          />
        </div>
        <DoneSwitch
          checked={column.isDone}
          label={`Đánh dấu ${column.name} là cột hoàn thành`}
          onCheckedChange={(checked) => {
            updateColumnMutation.mutate(
              { columnId: column.id, isDone: checked },
              {
                onError: (error) => hvToast(kanbanErrorToastMessage(error), { variant: "danger" }),
              },
            );
          }}
        />
        <button
          type="button"
          aria-label={`Chuyển ${column.name} lên trước`}
          disabled={index === 0}
          onClick={() => onReorder("previous")}
          className="inline-flex size-7 items-center justify-center rounded-md text-ink-500 hover:bg-cream-100 disabled:cursor-not-allowed disabled:opacity-30"
        >
          <HvIcon name="arrow-up" className="size-4" />
        </button>
        <button
          type="button"
          aria-label={`Chuyển ${column.name} xuống sau`}
          disabled={index === count - 1}
          onClick={() => onReorder("next")}
          className="inline-flex size-7 items-center justify-center rounded-md text-ink-500 hover:bg-cream-100 disabled:cursor-not-allowed disabled:opacity-30"
        >
          <HvIcon name="arrow-down" className="size-4" />
        </button>
        <button
          type="button"
          aria-label={`Xoá cột ${column.name}`}
          disabled={disableDelete}
          onClick={onRequestDelete}
          className="inline-flex size-7 items-center justify-center rounded-md text-coral-500 hover:bg-coral-100 disabled:cursor-not-allowed disabled:opacity-30"
        >
          <HvIcon name="trash" className="size-4" />
        </button>
      </div>
      {nameError ? <p className="pl-6 text-[12px] text-coral-600">{nameError}</p> : null}
      <p className="pl-6 text-[11.5px] text-ink-400">{taskCount} việc</p>
    </div>
  );
}

export interface BoardSettingsModalProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  columns: KanbanColumn[];
  tasksByColumn: Map<ColumnId, AppTask[]>;
  createColumnMutation: UseMutationResult<KanbanColumn, KanbanError, CreateColumnVariables>;
  updateColumnMutation: UseMutationResult<KanbanColumn, KanbanError, UpdateColumnVariables>;
  deleteColumnMutation: UseMutationResult<void, KanbanError, DeleteColumnVariables>;
  reorderColumn: (columnId: ColumnId, direction: AdjacentDirection) => Promise<void>;
  onAnnounce: (message: string) => void;
}

/**
 * Column CRUD + manual reordering (buttons, no drag-and-drop) for
 * `tasks.manage_board` holders. `PUT /task-columns/order` treats both 409 and
 * 422 as "the board changed under you" (see `use-tasks-data-source.ts`), so
 * the up/down buttons surface that as a single toast rather than a field error.
 */
export function BoardSettingsModal({
  open,
  onOpenChange,
  columns,
  tasksByColumn,
  createColumnMutation,
  updateColumnMutation,
  deleteColumnMutation,
  reorderColumn,
  onAnnounce,
}: BoardSettingsModalProps) {
  const sorted = [...columns].sort((a, b) => a.order - b.order);
  const [newName, setNewName] = useState("");
  const [newNameError, setNewNameError] = useState<string | undefined>(undefined);
  const [deleteTarget, setDeleteTarget] = useState<KanbanColumn | undefined>(undefined);

  const handleReorder = async (columnId: ColumnId, direction: AdjacentDirection) => {
    try {
      await reorderColumn(columnId, direction);
    } catch (error) {
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  };

  const handleCreate = async () => {
    const trimmed = newName.trim();
    if (trimmed === "") return;
    try {
      const column = await createColumnMutation.mutateAsync({ name: trimmed, isDone: false });
      setNewName("");
      setNewNameError(undefined);
      onAnnounce(`Đã tạo cột ${column.name}.`);
    } catch (error) {
      if (isKanbanError(error) && error.kind === "validation") {
        setNewNameError(error.fields.name ?? kanbanErrorToastMessage(error));
      } else {
        hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
      }
    }
  };

  return (
    <>
      <HvModal
        open={open}
        onOpenChange={onOpenChange}
        title="Cấu hình cột bảng công việc"
        size="lg"
      >
        <div className="flex flex-col gap-3">
          <p className="text-[12.5px] text-ink-400">
            {sorted.length}/{MAX_COLUMNS} cột
          </p>
          {sorted.map((column, index) => (
            <ColumnRow
              key={column.id}
              column={column}
              taskCount={tasksByColumn.get(column.id)?.length ?? 0}
              index={index}
              count={sorted.length}
              updateColumnMutation={updateColumnMutation}
              onReorder={(direction) => void handleReorder(column.id, direction)}
              onRequestDelete={() => setDeleteTarget(column)}
              disableDelete={sorted.length <= 1}
            />
          ))}
          <div className="flex flex-col gap-1 border-t border-line-200 pt-3">
            <div className="flex gap-2">
              <Input
                aria-label="Tên cột mới"
                placeholder="Tên cột mới"
                value={newName}
                disabled={sorted.length >= MAX_COLUMNS}
                onChange={(event) => setNewName(event.target.value)}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    event.preventDefault();
                    void handleCreate();
                  }
                }}
              />
              <HvButton
                type="button"
                variant="secondary"
                size="sm"
                disabled={sorted.length >= MAX_COLUMNS || newName.trim() === ""}
                onClick={() => void handleCreate()}
              >
                Thêm cột
              </HvButton>
            </div>
            {newNameError ? <p className="text-[12px] text-coral-600">{newNameError}</p> : null}
          </div>
        </div>
      </HvModal>
      {deleteTarget ? (
        <DeleteColumnDialog
          open
          onOpenChange={(next) => {
            if (!next) setDeleteTarget(undefined);
          }}
          column={deleteTarget}
          taskCount={tasksByColumn.get(deleteTarget.id)?.length ?? 0}
          otherColumns={sorted.filter((column) => column.id !== deleteTarget.id)}
          deleteColumnMutation={deleteColumnMutation}
          onAnnounce={onAnnounce}
        />
      ) : null}
    </>
  );
}
