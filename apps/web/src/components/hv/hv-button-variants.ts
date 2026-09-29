import { cva } from "class-variance-authority";

import { cn } from "@/lib/utils";

export type HvButtonVariant = "primary" | "secondary" | "reward" | "danger" | "ghost";
export type HvButtonSize = "sm" | "md" | "lg";

/**
 * Class builder for the button look. Exported so a router `<Link>` can look
 * like a button without nesting `<button>` in `<a>` or copying the classes —
 * one source of truth for button colors.
 */
export const hvButtonVariants = cva(
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
    // Brand fills are pastel, so their label is always --text-on-brand
    // (ink-900); white text on them fails WCAG AA.
    variants: {
      variant: {
        primary: "bg-mint-400 text-on-brand shadow-press-mint active:shadow-none",
        secondary: "bg-sky-300 text-on-brand shadow-press-sky active:shadow-none",
        reward: "bg-sun-400 text-on-brand shadow-press-sun active:shadow-none",
        danger: "bg-coral-400 text-on-brand shadow-press-coral active:shadow-none",
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
