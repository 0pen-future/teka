import * as React from "react";

import { cn } from "@/lib/utils";

import { HvIcon, type HvIconName } from "./hv-icon";

export type HvNoticeTone = "info" | "warning" | "danger" | "success";
export type HvNoticeRole = "note" | "alert" | "status";

const toneClassName: Record<HvNoticeTone, string> = {
  info: "border-sky-200 bg-sky-50 text-sky-600",
  warning: "border-sun-200 bg-sun-100 text-sun-600",
  danger: "border-coral-300 bg-coral-100 text-coral-600",
  success: "border-mint-200 bg-mint-50 text-mint-700",
};

const toneIcon: Record<HvNoticeTone, HvIconName> = {
  info: "info",
  warning: "alert",
  danger: "alert",
  success: "check",
};

export interface HvNoticeProps {
  tone?: HvNoticeTone;
  /** Defaults to "alert" for danger and "note" otherwise. */
  role?: HvNoticeRole;
  title?: React.ReactNode;
  /** Override the tone's default icon; pass `null` to render none. */
  icon?: React.ReactNode;
  children?: React.ReactNode;
  className?: string;
}

/** Inline callout for contextual hints, warnings and error summaries. */
export function HvNotice({ tone = "info", role, title, icon, children, className }: HvNoticeProps) {
  const resolvedRole = role ?? (tone === "danger" ? "alert" : "note");
  const resolvedIcon = icon === undefined ? <HvIcon name={toneIcon[tone]} size={18} /> : icon;

  return (
    <div
      role={resolvedRole}
      className={cn(
        "flex gap-[var(--space-2)] rounded-[var(--radius-md)] border p-[var(--space-3)]",
        "text-[length:var(--text-sm)] leading-snug",
        toneClassName[tone],
        className,
      )}
    >
      {resolvedIcon != null ? (
        <span aria-hidden="true" className="mt-px inline-flex shrink-0">
          {resolvedIcon}
        </span>
      ) : null}
      <div className="min-w-0 flex-1">
        {title != null ? <p className="font-bold">{title}</p> : null}
        {children != null ? <div className={cn(title != null && "mt-0.5")}>{children}</div> : null}
      </div>
    </div>
  );
}
