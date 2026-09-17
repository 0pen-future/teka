import type { MouseEvent, ReactNode, TouchEvent } from "react";

import { cn } from "@/lib/utils";

export interface CardControlBarrierProps {
  children: ReactNode;
  className?: string;
}

/**
 * Bubbling barrier for any control nested inside a draggable, clickable
 * card: stops `click` (would open the card), `mousedown`/`touchstart` (would
 * start a drag) from ever reaching the card's own handlers. Shared by every
 * in-card control — the actions menu here, the "Xong" checkbox later.
 */
export function CardControlBarrier({ children, className }: CardControlBarrierProps) {
  const stopPointerActivation = (event: MouseEvent | TouchEvent) => {
    event.stopPropagation();
  };

  return (
    <div
      role="presentation"
      className={cn(className)}
      onClick={(event) => event.stopPropagation()}
      onMouseDown={stopPointerActivation}
      onTouchStart={stopPointerActivation}
    >
      {children}
    </div>
  );
}
