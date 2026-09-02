import * as React from "react";

import { HvButton, HvModal, HvNotice } from "@/components/hv";

export interface UnsavedScoresGuardProps {
  open: boolean;
  /** Unsaved cell count shown in the title (includes invalid cells). */
  dirtyCount: number;
  /** Cells holding unreadable text; saving is blocked until they are fixed. */
  invalidCount?: number;
  /** Disables both actions while "Lưu và đóng" is saving. */
  pending?: boolean;
  /** Save first, then continue with whatever the user was doing. */
  onSave: () => void;
  /** Throw the unsaved cells away and continue. */
  onDiscard: () => void;
  /** Escape / overlay: stay on the scores with nothing changed. */
  onStay: () => void;
}

/**
 * "Còn n ô chưa lưu" prompt shown before an action that would unmount the
 * score draft (closing the panel, switching tab or session). Dismissing the
 * dialog keeps the draft; only the explicit "Bỏ thay đổi" discards, and the
 * save action holds initial focus so Enter can never discard by accident.
 */
export function UnsavedScoresGuard({
  open,
  dirtyCount,
  invalidCount = 0,
  pending = false,
  onSave,
  onDiscard,
  onStay,
}: UnsavedScoresGuardProps) {
  const saveRef = React.useRef<HTMLButtonElement>(null);
  const stayRef = React.useRef<HTMLButtonElement>(null);
  const canSave = invalidCount === 0;
  return (
    <HvModal
      open={open}
      onOpenChange={(next) => {
        if (!next && !pending) onStay();
      }}
      size="md"
      title={`Còn ${dirtyCount} ô chưa lưu`}
      description="Lưu điểm trước khi rời đi, hoặc bỏ các ô vừa sửa."
      // Initial focus goes to a safe action while the DOM keeps the visual
      // order, so Tab still moves left to right and Enter never discards.
      onOpenAutoFocus={(event) => {
        event.preventDefault();
        (canSave ? saveRef : stayRef).current?.focus();
      }}
      footer={
        <div className="flex gap-2">
          <HvButton
            ref={stayRef}
            type="button"
            variant="ghost"
            size="sm"
            disabled={pending}
            onClick={onStay}
          >
            Ở lại
          </HvButton>
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            className="text-coral-500"
            disabled={pending}
            onClick={onDiscard}
          >
            Bỏ thay đổi
          </HvButton>
          <HvButton
            ref={saveRef}
            type="button"
            variant="primary"
            size="sm"
            disabled={pending || !canSave}
            onClick={onSave}
          >
            {pending ? "Đang lưu…" : "Lưu và đóng"}
          </HvButton>
        </div>
      }
    >
      {canSave ? null : (
        <HvNotice tone="danger">
          Còn {invalidCount} ô không hợp lệ — sửa hoặc xóa nội dung ô trước khi lưu.
        </HvNotice>
      )}
    </HvModal>
  );
}
