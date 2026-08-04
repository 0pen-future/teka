import * as React from "react";

import { cn } from "@/lib/utils";

export type ProgressBarColor = "mint" | "viet" | "reward" | "missing";
export type ProgressBarSize = "sm" | "md" | "lg";

const trackSizeClasses: Record<ProgressBarSize, string> = {
  sm: "h-[10px]",
  md: "h-[14px]",
  lg: "h-[20px]",
};

const fillColorClasses: Record<ProgressBarColor, string> = {
  mint: "bg-mint-400",
  viet: "bg-sky-300",
  reward: "bg-sun-400",
  missing: "bg-coral-400",
};

export interface ProgressBarProps extends Omit<React.HTMLAttributes<HTMLDivElement>, "color"> {
  /** Percentage complete, clamped to 0..100. */
  value: number;
  /** Fill color. Defaults to "mint". */
  color?: ProgressBarColor;
  /** Track height. Defaults to "md". */
  size?: ProgressBarSize;
  /** Optional label shown above the track, on the left. */
  label?: React.ReactNode;
  /** Shows the rounded percentage above the track, on the right. */
  showValue?: boolean;
}

export function ProgressBar({
  value,
  color = "mint",
  size = "md",
  label,
  showValue = false,
  className,
  ...rest
}: ProgressBarProps) {
  const clamped = Math.min(100, Math.max(0, value));
  const showHead = label != null || showValue;

  return (
    <div className={className} {...rest}>
      {showHead ? (
        <div className="mb-[6px] flex items-baseline justify-between font-body font-bold">
          {label != null ? (
            <span className="text-[length:var(--text-sm)] text-ink-700">{label}</span>
          ) : null}
          {showValue ? (
            <span className="text-[length:var(--text-sm)] text-ink-500">
              {Math.round(clamped)}%
            </span>
          ) : null}
        </div>
      ) : null}
      <div
        role="progressbar"
        aria-valuenow={clamped}
        aria-valuemin={0}
        aria-valuemax={100}
        className={cn("overflow-hidden rounded-full bg-cream-200", trackSizeClasses[size])}
      >
        <div
          className={cn(
            "h-full rounded-full shadow-[inset_0_-3px_0_rgba(0,0,0,0.08)]",
            "transition-[width] duration-[var(--dur-slow)] ease-[var(--ease-out)]",
            fillColorClasses[color],
          )}
          style={{ width: `${clamped}%` }}
        />
      </div>
    </div>
  );
}
