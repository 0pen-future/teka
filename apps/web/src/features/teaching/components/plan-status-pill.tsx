import { cn } from "@/lib/utils";

import { planStatusLabels } from "../lib/plan-status";
import type { LessonPlanStatus } from "../lib/teaching-store";

const planStatusClasses: Record<LessonPlanStatus, string> = {
  none: "bg-cream-200 text-ink-400",
  draft: "bg-sky-50 text-sky-500",
  pending: "bg-sun-100 text-sun-600",
  approved: "bg-mint-50 text-mint-600",
  redo: "bg-coral-100 text-coral-600",
};

/** Prototype status chip for the giáo án state machine — shared by phases 3, 4, and 6. */
export function PlanStatusPill({
  status,
  className,
}: {
  status: LessonPlanStatus;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "whitespace-nowrap rounded-full px-[10px] py-1 text-[12px] font-extrabold",
        planStatusClasses[status],
        className,
      )}
    >
      {planStatusLabels[status]}
    </span>
  );
}
