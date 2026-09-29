import * as React from "react";

import { cn } from "@/lib/utils";

export interface HvTableScrollProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Accessible name for the scroll region so keyboard users can find and scroll it. */
  "aria-label": string;
}

/**
 * Horizontal scroll region for wide tables. `relative` makes this the
 * containing block for absolutely positioned descendants (e.g. `sr-only`
 * header labels), so they are clipped by the scroll box instead of widening
 * the document. Wrap every table that can be wider than a phone in it.
 */
export const HvTableScroll = React.forwardRef<HTMLDivElement, HvTableScrollProps>(
  ({ className, ...rest }, ref) => (
    <div
      ref={ref}
      role="region"
      // A scrollable region must be focusable so keyboard users can scroll it
      // (WCAG 2.1.1; axe `scrollable-region-focusable`).
      // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
      tabIndex={0}
      className={cn(
        "relative max-w-full overflow-x-auto overscroll-x-contain",
        "focus-visible:outline-none focus-visible:ring-4",
        className,
      )}
      {...rest}
    />
  ),
);
HvTableScroll.displayName = "HvTableScroll";
