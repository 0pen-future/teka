import * as React from "react";

import { HvButton } from "./hv-button";
import { HvModal } from "./hv-modal";

export type HvConfirmDialogTone = "default" | "danger";

export interface HvConfirmDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: React.ReactNode;
  description?: React.ReactNode;
  confirmLabel: string;
  /** Defaults to "Hủy". */
  cancelLabel?: string;
  /** "danger" renders the confirm action with the destructive button style. */
  tone?: HvConfirmDialogTone;
  /** Disables both actions while the confirmed request is in flight. */
  pending?: boolean;
  onConfirm: () => void;
}

/**
 * Two-action confirmation modal. Cancel is the first tabbable element so an
 * accidental Enter never confirms a destructive action.
 */
export function HvConfirmDialog({
  open,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel = "Hủy",
  tone = "default",
  pending = false,
  onConfirm,
}: HvConfirmDialogProps) {
  return (
    <HvModal
      open={open}
      onOpenChange={(next) => {
        if (pending && !next) return;
        onOpenChange(next);
      }}
      size="md"
      title={title}
      description={description}
      footer={
        <>
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            disabled={pending}
            onClick={() => onOpenChange(false)}
          >
            {cancelLabel}
          </HvButton>
          <HvButton
            type="button"
            variant={tone === "danger" ? "danger" : "primary"}
            size="sm"
            disabled={pending}
            onClick={onConfirm}
          >
            {confirmLabel}
          </HvButton>
        </>
      }
    >
      {null}
    </HvModal>
  );
}
