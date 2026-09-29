import * as React from "react";

import { cn } from "@/lib/utils";

import { hvButtonVariants, type HvButtonSize, type HvButtonVariant } from "./hv-button-variants";

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
