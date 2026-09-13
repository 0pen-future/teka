import { useState } from "react";
import type { UseMutationResult } from "@tanstack/react-query";

import { HvModal, HvButton, HvSelect, hvToast } from "@/components/hv";
import type { ColumnId, KanbanColumn, KanbanError } from "@/lib/kanban";

import type { DeleteColumnVariables } from "../hooks/use-tasks-data-source";
import { kanbanErrorToastMessage } from "../lib/map-api-error";

export interface DeleteColumnDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  column: KanbanColumn;
  taskCount: number;
  otherColumns: KanbanColumn[];
  deleteColumnMutation: UseMutationResult<void, KanbanError, DeleteColumnVariables>;
  onAnnounce: (message: string) => void;
}

/**
 * `move_to` is only asked for when the column has tasks (the API leaves it
 * optional otherwise); an empty column silently targets the first sibling so
 * the request always carries a value the port's signature requires.
 */
export function DeleteColumnDialog({
  open,
  onOpenChange,
  column,
  taskCount,
  otherColumns,
  deleteColumnMutation,
  onAnnounce,
}: DeleteColumnDialogProps) {
  const [moveTo, setMoveTo] = useState<ColumnId | "">("");
  const needsMoveTo = taskCount > 0;
  const resolvedMoveTo = needsMoveTo ? moveTo : (otherColumns[0]?.id ?? "");
  const canConfirm = resolvedMoveTo !== "";

  const handleConfirm = async () => {
    if (!canConfirm) return;
    try {
      await deleteColumnMutation.mutateAsync({
        columnId: column.id,
        moveTasksTo: resolvedMoveTo,
      });
      onAnnounce(`Đã xoá cột ${column.name}.`);
      hvToast("Đã xoá cột", { variant: "success" });
      setMoveTo("");
      onOpenChange(false);
    } catch (error) {
      hvToast(kanbanErrorToastMessage(error), { variant: "danger" });
    }
  };

  return (
    <HvModal
      open={open}
      onOpenChange={(next) => {
        if (!next) setMoveTo("");
        onOpenChange(next);
      }}
      title={`Xoá cột "${column.name}"?`}
      description={
        taskCount > 0
          ? `Cột này còn ${taskCount} việc. Chọn cột để chuyển các việc đó sang trước khi xoá.`
          : "Cột này không còn việc nào."
      }
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="button"
            variant="danger"
            disabled={!canConfirm || deleteColumnMutation.isPending}
            onClick={() => void handleConfirm()}
          >
            {deleteColumnMutation.isPending ? "Đang xoá…" : "Xoá cột"}
          </HvButton>
        </>
      }
    >
      {needsMoveTo ? (
        <HvSelect
          sheetTitle="Chuyển việc sang cột"
          aria-label="Chuyển việc sang cột"
          value={moveTo}
          onValueChange={(value) => setMoveTo(value as ColumnId)}
          options={otherColumns.map((candidate) => ({
            value: candidate.id,
            label: candidate.name,
          }))}
          placeholder="Chọn cột"
        />
      ) : null}
    </HvModal>
  );
}
