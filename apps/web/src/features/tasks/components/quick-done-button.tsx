import { CheckIcon } from "lucide-react";

import { cn } from "@/lib/utils";

export interface QuickDoneButtonProps {
  onDone: () => void;
  disabled?: boolean;
}

/**
 * The 26px check button hanging over a card's leading edge, for moving a
 * task to the first "done" column in one click (see `handleQuickDone` in
 * the board page). Only rendered while the task is still open — the move is
 * one-directional, undo happens through the toast the page shows. Hidden
 * until the card is hovered or focused, except on touch devices, which have
 * no hover to reveal it with.
 */
export function QuickDoneButton({ onDone, disabled }: QuickDoneButtonProps) {
  return (
    <button
      type="button"
      aria-label="Đánh dấu hoàn thành"
      title="Đánh dấu hoàn thành"
      disabled={disabled}
      onClick={onDone}
      className={cn(
        "absolute left-[-2px] top-2.5 z-10 flex size-[26px] -translate-x-1/2 items-center justify-center",
        "rounded-full border-2 border-line-300 bg-white text-mint-600 shadow-xs",
        "opacity-0 transition-[opacity,background-color,border-color] group-hover:opacity-100",
        "group-focus-within:opacity-100 focus-visible:opacity-100 [@media(hover:none)]:opacity-100",
        "hover:border-mint-400 hover:bg-mint-400 hover:text-white",
        "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
        "disabled:cursor-not-allowed disabled:opacity-50",
      )}
    >
      <CheckIcon aria-hidden className="size-3.5" strokeWidth={3} />
    </button>
  );
}
