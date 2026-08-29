import { useState } from "react";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldLabel } from "@/components/ui/field";
import { ApiError } from "@/lib/api/errors";

import {
  useAssignScoreSet,
  useClassScoreComponents,
  useClearScoreSet,
} from "../hooks/use-score-sets";
import type { ScoreSet } from "../schemas/grading";

export interface AssignScoreSetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  classId: string;
  className: string;
  scoreSets: ScoreSet[];
}

const CONFLICT_MESSAGE =
  "Lớp đã có điểm được ghi nhận nên không thể đổi hoặc xóa bộ điểm. Xóa điểm đã nhập của lớp trước khi thực hiện lại.";

/**
 * Assign/clear a bộ điểm on one class (`POST`/`DELETE
 * /classes/:id/score-set`). Shows the class's currently assigned columns —
 * a snapshot, not a live `ScoreSet` reference, so there is no "current set"
 * selection to preselect. A 409 (class already has recorded scores) blocks
 * further attempts in this dialog with the plain-language reason instead of
 * silently re-enabling the controls.
 */
export function AssignScoreSetDialog({
  open,
  onOpenChange,
  classId,
  className,
  scoreSets,
}: AssignScoreSetDialogProps) {
  const { data: current, isPending } = useClassScoreComponents(open ? classId : undefined);
  const assignMutation = useAssignScoreSet();
  const clearMutation = useClearScoreSet();
  const [selectedSetId, setSelectedSetId] = useState("");
  const [conflict, setConflict] = useState<string | null>(null);

  const hasComponents = (current?.components.length ?? 0) > 0;
  const busy = assignMutation.isPending || clearMutation.isPending;
  const locked = Boolean(conflict) || busy;

  function handleClose(next: boolean) {
    if (!next) {
      setSelectedSetId("");
      setConflict(null);
    }
    onOpenChange(next);
  }

  function reportError(error: unknown) {
    if (error instanceof ApiError && error.code === "CONFLICT") {
      setConflict(CONFLICT_MESSAGE);
      return;
    }
    hvToast("Có lỗi xảy ra, thử lại sau", { variant: "danger" });
  }

  function handleAssign() {
    if (!selectedSetId) {
      return;
    }
    assignMutation.mutate(
      { classId, setId: selectedSetId },
      {
        onSuccess: () => {
          hvToast(`Đã gán bộ điểm cho ${className}`, { variant: "success" });
          setSelectedSetId("");
        },
        onError: reportError,
      },
    );
  }

  function handleClear() {
    clearMutation.mutate(classId, {
      onSuccess: () => {
        hvToast(`Đã xóa bộ điểm của ${className}`, { variant: "success" });
      },
      onError: reportError,
    });
  }

  return (
    <HvModal
      open={open}
      onOpenChange={handleClose}
      title={`Bộ điểm — ${className}`}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => handleClose(false)}>
            Đóng
          </HvButton>
          {hasComponents ? (
            <HvButton type="button" variant="danger" disabled={locked} onClick={handleClear}>
              {clearMutation.isPending ? "Đang xóa…" : "Xóa gán"}
            </HvButton>
          ) : null}
          <HvButton type="button" disabled={!selectedSetId || locked} onClick={handleAssign}>
            {assignMutation.isPending ? "Đang gán…" : "Gán"}
          </HvButton>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <div>
          <p className="text-[12px] font-extrabold tracking-[0.3px] text-ink-400">
            CỘT ĐIỂM HIỆN TẠI
          </p>
          {isPending ? (
            <p className="mt-1 text-[13px] text-ink-400">Đang tải…</p>
          ) : hasComponents ? (
            <p className="mt-1 text-[13.5px] text-ink-700">
              {current!.components
                .slice()
                .sort((a, b) => a.position - b.position)
                .map((component) => component.name)
                .join(", ")}
            </p>
          ) : (
            <p className="mt-1 text-[13px] text-ink-400">Chưa gán bộ điểm</p>
          )}
        </div>

        {conflict ? (
          <p
            role="alert"
            className="rounded-[var(--radius-md)] bg-coral-50 px-3 py-2 text-[13px] font-bold text-coral-600"
          >
            {conflict}
          </p>
        ) : null}

        <Field>
          <FieldLabel htmlFor="assign-score-set-select">Chọn bộ điểm để gán</FieldLabel>
          <select
            id="assign-score-set-select"
            value={selectedSetId}
            onChange={(event) => setSelectedSetId(event.target.value)}
            disabled={locked}
            className="min-h-11 rounded-[var(--radius-md)] border border-line-200 bg-white px-3 text-[14px] text-ink-900 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <option value="">— Chọn bộ điểm —</option>
            {scoreSets.map((set) => (
              <option key={set.id} value={set.id}>
                {set.name}
              </option>
            ))}
          </select>
        </Field>
      </div>
    </HvModal>
  );
}
