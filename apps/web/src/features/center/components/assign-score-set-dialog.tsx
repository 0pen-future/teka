import { RadioGroup } from "radix-ui";
import { useState } from "react";

import { HvBadge, HvButton, HvModal, HvNotice, HvStateBlock, hvToast } from "@/components/hv";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";

import {
  useAssignScoreSet,
  useClassScoreComponents,
  useClearScoreSet,
} from "../hooks/use-score-sets";
import type { ScoreSet } from "../schemas/grading";
import { ScoreSetPreviewStrip } from "./score-set-preview-strip";

export interface AssignScoreSetDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  classId: string;
  className: string;
  scoreSets: ScoreSet[];
  /** True when a 409 already locked this class earlier in the page session. */
  locked?: boolean;
  /** Called on a 409 so the page can remember the class as locked. */
  onLocked?: () => void;
}

const CONFLICT_MESSAGE =
  "Lớp đã có điểm được ghi nhận nên không thể đổi hoặc xóa bộ điểm. Xóa điểm đã nhập của lớp trước khi thực hiện lại.";

function sameColumns(a: readonly string[], b: readonly string[]): boolean {
  return a.length === b.length && a.every((name, index) => name === b[index]);
}

/**
 * Assign/clear a bộ điểm on one class (`POST`/`DELETE
 * /classes/:id/score-set`). The class keeps a snapshot of column names, not a
 * reference to a set, so the "Đang dùng" hint is a guess: the single set
 * whose columns match the snapshot exactly (none when several match). A 409
 * (class already has recorded scores) locks the dialog with the plain-language
 * reason and reports up so reopening for the same class stays locked.
 */
export function AssignScoreSetDialog({
  open,
  onOpenChange,
  classId,
  className,
  scoreSets,
  locked: lockedByPage = false,
  onLocked,
}: AssignScoreSetDialogProps) {
  const { data: current, isPending } = useClassScoreComponents(open ? classId : undefined);
  const assignMutation = useAssignScoreSet();
  const clearMutation = useClearScoreSet();
  const [selectedSetId, setSelectedSetId] = useState("");
  const [conflict, setConflict] = useState(false);

  const currentNames =
    current?.components
      .slice()
      .sort((a, b) => a.position - b.position)
      .map((component) => component.name) ?? [];
  const hasComponents = currentNames.length > 0;
  const matching = hasComponents
    ? scoreSets.filter((set) => sameColumns(set.components, currentNames))
    : [];
  const currentSetGuess = matching.length === 1 ? matching[0] : undefined;

  const busy = assignMutation.isPending || clearMutation.isPending;
  const showConflict = conflict || lockedByPage;
  const locked = showConflict || busy;

  function handleClose(next: boolean) {
    if (!next) {
      setSelectedSetId("");
      setConflict(false);
    }
    onOpenChange(next);
  }

  function reportError(error: unknown) {
    if (error instanceof ApiError && error.code === "CONFLICT") {
      setConflict(true);
      onLocked?.();
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
      size="lg"
      title={`Bộ điểm — ${className}`}
      footer={
        <>
          <HvButton type="button" variant="ghost" size="sm" onClick={() => handleClose(false)}>
            Đóng
          </HvButton>
          {hasComponents ? (
            <HvButton
              type="button"
              variant="danger"
              size="sm"
              disabled={locked}
              onClick={handleClear}
            >
              {clearMutation.isPending ? "Đang xóa…" : "Xóa gán"}
            </HvButton>
          ) : null}
          <HvButton
            type="button"
            variant="primary"
            size="sm"
            disabled={!selectedSetId || locked}
            onClick={handleAssign}
          >
            {assignMutation.isPending ? "Đang gán…" : "Gán"}
          </HvButton>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        {showConflict ? (
          <HvNotice tone="warning" role="alert" title="Không thể thay đổi bộ điểm">
            {CONFLICT_MESSAGE}
          </HvNotice>
        ) : (
          <HvNotice tone="info">Lớp đã ghi nhận điểm sẽ không đổi hoặc xóa được bộ điểm.</HvNotice>
        )}

        <div>
          <p className="text-[12px] font-extrabold tracking-[0.3px] text-ink-400">
            CỘT ĐIỂM HIỆN TẠI
          </p>
          {isPending ? (
            <HvStateBlock state="loading" compact className="mt-1" title="Đang tải cột điểm" />
          ) : hasComponents ? (
            <div className="mt-1.5">
              <ScoreSetPreviewStrip names={currentNames} />
            </div>
          ) : (
            <p className="mt-1 text-[13px] text-ink-400">Chưa gán bộ điểm</p>
          )}
        </div>

        <div className="flex flex-col gap-2">
          <p id="assign-score-set-label" className="text-[13px] font-bold text-ink-700">
            Chọn bộ điểm để gán
          </p>
          <RadioGroup.Root
            aria-labelledby="assign-score-set-label"
            value={selectedSetId}
            onValueChange={setSelectedSetId}
            disabled={locked}
            className="flex max-h-[50dvh] flex-col gap-2 overflow-y-auto"
          >
            {scoreSets.map((set) => {
              const isCurrent = currentSetGuess?.id === set.id;
              return (
                <RadioGroup.Item
                  key={set.id}
                  value={set.id}
                  aria-label={[
                    `${set.name}, ${set.components.length} cột: ${set.components.join(", ")}`,
                    isCurrent ? "đang dùng" : null,
                  ]
                    .filter(Boolean)
                    .join(" — ")}
                  className={cn(
                    "flex min-h-11 w-full flex-col gap-2 rounded-[var(--radius-md)] border-2 border-line-200 bg-white p-3 text-left transition-colors outline-none",
                    "hover:border-mint-300 focus-visible:border-mint-400",
                    "data-[state=checked]:border-mint-400 data-[state=checked]:bg-mint-50",
                    "disabled:cursor-not-allowed disabled:opacity-60",
                  )}
                >
                  <span className="flex flex-wrap items-center gap-2">
                    <span className="text-[14px] font-bold text-ink-900">{set.name}</span>
                    <HvBadge variant="neutral">{set.components.length} cột</HvBadge>
                    {isCurrent ? <HvBadge variant="info">Đang dùng</HvBadge> : null}
                  </span>
                  <ScoreSetPreviewStrip names={set.components} />
                </RadioGroup.Item>
              );
            })}
          </RadioGroup.Root>
        </div>
      </div>
    </HvModal>
  );
}
