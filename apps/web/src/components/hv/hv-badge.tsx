import * as React from "react";
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

export type HvBadgeVariant =
  "math" | "viet" | "success" | "info" | "warning" | "danger" | "neutral";
export type HvBadgeSize = "sm" | "md";

const hvBadgeVariants = cva(
  "inline-flex items-center gap-[5px] whitespace-nowrap rounded-full font-body font-bold leading-none",
  {
    variants: {
      variant: {
        math: "bg-mint-50 text-mint-600",
        viet: "bg-sky-50 text-sky-500",
        success: "bg-mint-50 text-mint-600",
        info: "bg-sky-50 text-sky-500",
        warning: "bg-sun-100 text-sun-600",
        danger: "bg-coral-100 text-coral-600",
        neutral: "bg-cream-200 text-ink-500",
      },
      size: {
        sm: "px-[9px] py-[4px] text-[length:var(--text-2xs)]",
        md: "px-[11px] py-[6px] text-[length:var(--text-xs)]",
      },
    },
    defaultVariants: {
      variant: "math",
      size: "md",
    },
  },
);

/** Solid-fill overrides for subject badges only (`solid` prop). */
const solidVariantClasses: Partial<Record<HvBadgeVariant, string>> = {
  math: "bg-mint-400 text-white",
  viet: "bg-sky-300 text-white",
};

export interface HvBadgeProps extends React.HTMLAttributes<HTMLSpanElement> {
  /** Color/topic mapping. Defaults to "math". */
  variant?: HvBadgeVariant;
  /** Size scale. Defaults to "md". */
  size?: HvBadgeSize;
  /** Fills math/viet badges solid instead of the soft-tint background. */
  solid?: boolean;
  /** Renders a leading `currentColor` dot before the label. */
  dot?: boolean;
}

export function HvBadge({
  className,
  variant = "math",
  size = "md",
  solid = false,
  dot = false,
  children,
  ...rest
}: HvBadgeProps) {
  return (
    <span
      className={cn(
        hvBadgeVariants({ variant, size }),
        solid ? solidVariantClasses[variant] : null,
        className,
      )}
      {...rest}
    >
      {dot ? <span className="h-[7px] w-[7px] rounded-full bg-current" /> : null}
      {children}
    </span>
  );
}
