import { CheckIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export interface QuickDoneCheckboxProps {
  checked: boolean;
  onCheckedChange: () => void;
  disabled?: boolean;
  label: string;
}

/**
 * A 40px checkbox at a card's leading edge, for moving a task to the first
 * "done" column in one click (see `handleQuickDone` in the board page).
 * `checked` reflects `completedAt !== null`; clicking it again does nothing
 * once a task is already done — the move is one-directional, undo happens
 * through the toast the page shows, not by unchecking.
 */
export function QuickDoneCheckbox({
  checked,
  onCheckedChange,
  disabled,
  label,
}: QuickDoneCheckboxProps) {
  return (
    <button
      type="button"
      role="checkbox"
      aria-checked={checked}
      aria-label={label}
      disabled={Boolean(disabled) || checked}
      onClick={onCheckedChange}
      className={cn(
        "mt-0.5 flex size-10 shrink-0 items-center justify-center rounded-full",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        "disabled:cursor-not-allowed",
      )}
    >
      <span
        className={cn(
          "flex size-5 shrink-0 items-center justify-center rounded-md border-2",
          checked
            ? "border-mint-500 bg-mint-500"
            : "border-line-200 bg-white hover:border-mint-400",
        )}
      >
        {checked ? <CheckIcon aria-hidden className="size-3.5 text-white" /> : null}
      </span>
    </button>
  );
}
