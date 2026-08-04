import * as React from "react";
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

export type HvButtonVariant = "primary" | "secondary" | "reward" | "danger" | "ghost";
export type HvButtonSize = "sm" | "md" | "lg";

const hvButtonVariants = cva(
  cn(
    "inline-flex select-none items-center justify-center gap-2 rounded-lg border-0",
    "cursor-pointer font-display font-bold tracking-[var(--tracking-wide)]",
    "transition-[transform,box-shadow,filter] duration-[var(--dur-fast)] ease-[var(--ease-out)]",
    "hover:brightness-[1.04] active:translate-y-[var(--press-depth)]",
    "focus-visible:outline-none focus-visible:ring-4",
    "disabled:translate-y-0 disabled:cursor-not-allowed disabled:bg-line-200",
    "disabled:text-ink-300 disabled:shadow-none disabled:brightness-100",
    "aria-disabled:translate-y-0 aria-disabled:cursor-not-allowed aria-disabled:bg-line-200",
    "aria-disabled:text-ink-300 aria-disabled:shadow-none aria-disabled:brightness-100",
  ),
  {
    variants: {
      variant: {
        primary: "bg-mint-400 text-white shadow-press-mint active:shadow-none",
        secondary: "bg-sky-300 text-white shadow-press-sky active:shadow-none",
        reward: "bg-sun-400 text-sun-600 shadow-press-sun active:shadow-none",
        danger: "bg-coral-400 text-white shadow-press-coral active:shadow-none",
        ghost: cn(
          "bg-white text-mint-600",
          "shadow-[0_var(--press-depth)_0_var(--line-300),inset_0_0_0_2px_var(--line-200)]",
          "active:shadow-[inset_0_0_0_2px_var(--line-200)]",
        ),
      },
      size: {
        sm: "min-h-[44px] rounded-[var(--radius-md)] px-[18px] text-[length:var(--text-sm)]",
        md: "min-h-[56px] px-6 text-[length:var(--text-md)]",
        lg: "min-h-[64px] px-8 text-[length:var(--text-lg)]",
      },
      block: {
        true: "flex w-full",
        false: "",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "md",
      block: false,
    },
  },
);

const iconSlotClassName = "inline-flex h-[1.15em] w-[1.15em] shrink-0 items-center justify-center";

export interface HvButtonProps extends Omit<
  React.ButtonHTMLAttributes<HTMLButtonElement>,
  "children"
> {
  /** Visual style. Defaults to "primary". */
  variant?: HvButtonVariant;
  /**
   * Button height/padding/type scale. Defaults to "md".
   *
   * "sm" (44px) is DS-sanctioned only for dense secondary actions — prefer
   * "md" (56px) or "lg" (64px) for primary, kid-facing touch targets.
   */
  size?: HvButtonSize;
  /** Expands the button to fill its container's width. */
  block?: boolean;
  /** Leading icon rendered before the label. */
  icon?: React.ReactNode;
  /** Trailing icon rendered after the label. */
  iconRight?: React.ReactNode;
  children?: React.ReactNode;
}

export const HvButton = React.forwardRef<HTMLButtonElement, HvButtonProps>(
  (
    {
      className,
      variant = "primary",
      size = "md",
      block = false,
      icon,
      iconRight,
      type = "button",
      children,
      ...rest
    },
    ref,
  ) => {
    return (
      <button
        ref={ref}
        type={type}
        className={cn(hvButtonVariants({ variant, size, block }), className)}
        {...rest}
      >
        {icon ? <span className={iconSlotClassName}>{icon}</span> : null}
        {children}
        {iconRight ? <span className={iconSlotClassName}>{iconRight}</span> : null}
      </button>
    );
  },
);
HvButton.displayName = "HvButton";
