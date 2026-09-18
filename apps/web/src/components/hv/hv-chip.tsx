import * as React from "react";

import { cn } from "@/lib/utils";
import type { HvBadgeVariant } from "./hv-badge";

export type HvChipSize = "sm" | "md";

/**
 * Dot color per variant, mirroring `HvBadge`'s variant → color mapping (the
 * `text-*` half of its tint classes) since that map lives inside `HvBadge`'s
 * `cva` config and isn't exported.
 */
const dotColorClassName: Record<HvBadgeVariant, string> = {
  math: "bg-mint-600",
  viet: "bg-sky-500",
  success: "bg-mint-600",
  info: "bg-sky-500",
  warning: "bg-sun-600",
  danger: "bg-coral-600",
  neutral: "bg-ink-500",
};

const baseClassName = cn(
  "inline-flex select-none items-center gap-1.5 whitespace-nowrap rounded-full border-2",
  "border-line-200 bg-white font-body font-bold text-ink-700",
  "cursor-pointer transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)]",
  "hover:bg-cream-100",
  "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
  "disabled:cursor-not-allowed disabled:opacity-50",
);

const sizeClassName: Record<HvChipSize, string> = {
  sm: "min-h-8 px-2.5 text-[12px]",
  md: "min-h-10 px-3 text-[13px]",
};

/** Pressed/checked fill — a light mint tone shared by every chip instance. */
const pressedClassName = "border-mint-100 bg-mint-50 text-mint-600 hover:bg-mint-50";

export interface HvChipProps extends Omit<React.ButtonHTMLAttributes<HTMLButtonElement>, "type"> {
  /** Toggled state. Defaults to `false`. */
  pressed?: boolean;
  /** Leading 7px color dot, using the same variant palette as `HvBadge`. */
  dot?: HvBadgeVariant;
  /** Optional trailing count, e.g. the number of items a filter matches. */
  count?: number;
  /** Size scale. Defaults to "md". */
  size?: HvChipSize;
}

/**
 * A toggleable pill button. Acts as an `aria-pressed` toggle by default, or
 * as a radio item (`aria-checked`) when the owner renders it inside a
 * `role="radiogroup"` and passes `role="radio"`.
 */
export const HvChip = React.forwardRef<HTMLButtonElement, HvChipProps>(
  ({ pressed = false, dot, count, size = "md", role, className, children, ...rest }, ref) => {
    const isRadio = role === "radio";
    return (
      <button
        ref={ref}
        type="button"
        role={role}
        data-state={pressed ? "on" : "off"}
        className={cn(baseClassName, sizeClassName[size], pressed && pressedClassName, className)}
        {...rest}
        aria-pressed={isRadio ? undefined : pressed}
        aria-checked={isRadio ? pressed : undefined}
      >
        {dot != null ? (
          <span
            aria-hidden="true"
            className={cn("h-[7px] w-[7px] shrink-0 rounded-full", dotColorClassName[dot])}
          />
        ) : null}
        {children}
        {count != null ? <span className="ml-0.5 tabular-nums opacity-70">{count}</span> : null}
      </button>
    );
  },
);
HvChip.displayName = "HvChip";
