import * as React from "react";

import { cn } from "@/lib/utils";

import { statusPillLabels } from "./status-pill-labels";
import type { StatusPillStatus } from "./status-pill-labels";

export type { StatusPillStatus } from "./status-pill-labels";

const statusPillClasses: Record<StatusPillStatus, string> = {
  paid: "bg-mint-50 text-mint-600",
  partial: "bg-sun-100 text-sun-600",
  unpaid: "bg-coral-100 text-coral-600",
};

export interface StatusPillProps extends React.HTMLAttributes<HTMLSpanElement> {
  status: StatusPillStatus;
}

export function StatusPill({ status, className, children, ...rest }: StatusPillProps) {
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-full px-3 py-1 font-display text-[13px] font-bold",
        statusPillClasses[status],
        className,
      )}
      {...rest}
    >
      {children ?? statusPillLabels[status]}
    </span>
  );
}
