import * as React from "react";
import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

export type HvCardVariant = "raised" | "flat" | "sunken";
export type HvCardPadding = "sm" | "md" | "lg";

const hvCardVariants = cva("rounded-[var(--radius-xl)] bg-white", {
  variants: {
    variant: {
      raised: "border border-line-100 shadow-soft-md",
      flat: "border border-line-200",
      sunken: "border-0 bg-cream-200",
    },
    padding: {
      sm: "p-[var(--space-3)]",
      md: "p-[var(--pad-card)]",
      lg: "p-[var(--space-6)]",
    },
    interactive: {
      true: cn(
        "cursor-pointer transition-[transform,box-shadow] duration-[var(--dur-fast)] ease-[var(--ease-out)]",
        "hover:-translate-y-0.5 hover:shadow-soft-lg active:translate-y-0 active:shadow-soft-sm",
      ),
      false: "",
    },
  },
  defaultVariants: {
    variant: "raised",
    padding: "md",
    interactive: false,
  },
});

export interface HvCardProps extends React.HTMLAttributes<HTMLDivElement> {
  /** Surface treatment. Defaults to "raised". */
  variant?: HvCardVariant;
  /** Inner padding scale. Defaults to "md" (20px). */
  padding?: HvCardPadding;
  /** Adds hover/active affordances for clickable cards. */
  interactive?: boolean;
}

export const HvCard = React.forwardRef<HTMLDivElement, HvCardProps>(
  ({ className, variant = "raised", padding = "md", interactive = false, ...rest }, ref) => (
    <div
      ref={ref}
      className={cn(hvCardVariants({ variant, padding, interactive }), className)}
      {...rest}
    />
  ),
);
HvCard.displayName = "HvCard";
