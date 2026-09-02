import * as React from "react";

import { cn } from "@/lib/utils";
import { Skeleton } from "@/components/ui/skeleton";

export type HvStateBlockState = "loading" | "empty" | "error";

export interface HvStateBlockProps {
  state: HvStateBlockState;
  title: React.ReactNode;
  description?: React.ReactNode;
  /** Optional call to action (usually an `HvButton`), rendered under the copy. */
  action?: React.ReactNode;
  /** Tighter padding for inline use inside a panel or table cell. */
  compact?: boolean;
  className?: string;
}

/**
 * Shared loading / empty / error block so every list and panel announces the
 * same way: loading is a polite live region, error is an alert.
 */
export function HvStateBlock({
  state,
  title,
  description,
  action,
  compact = false,
  className,
}: HvStateBlockProps) {
  const role = state === "loading" ? "status" : state === "error" ? "alert" : undefined;

  return (
    <div
      role={role}
      aria-live={state === "loading" ? "polite" : undefined}
      data-state={state}
      className={cn(
        "flex flex-col items-center gap-[var(--space-2)] rounded-[var(--radius-md)] text-center",
        compact ? "p-[var(--space-3)]" : "p-[var(--space-6)]",
        state === "error" ? "bg-coral-100 text-coral-600" : "bg-cream-100 text-ink-500",
        className,
      )}
    >
      {state === "loading" ? (
        <div className="flex w-full max-w-xs flex-col gap-2" aria-hidden="true">
          <Skeleton className="h-3 w-full bg-line-200" />
          <Skeleton className="h-3 w-4/5 bg-line-200" />
          {compact ? null : <Skeleton className="h-3 w-3/5 bg-line-200" />}
        </div>
      ) : null}
      <p
        className={cn(
          "text-[length:var(--text-sm)] font-bold",
          state === "empty" && "text-ink-700",
          state === "loading" && "sr-only",
        )}
      >
        {title}
      </p>
      {description != null ? (
        <p className="text-[length:var(--text-sm)] text-ink-500">{description}</p>
      ) : null}
      {action != null ? <div className="mt-[var(--space-1)]">{action}</div> : null}
    </div>
  );
}
