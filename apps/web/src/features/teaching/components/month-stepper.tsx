import { HvIcon } from "@/components/hv";

import { shiftMonth } from "../lib/classbook-stats";

interface MonthStepperProps {
  /** "YYYY-MM". */
  month: string;
  onChange: (month: string) => void;
}

const arrowClassName =
  "grid h-9 w-9 shrink-0 cursor-pointer place-items-center rounded-[var(--radius-sm)] text-ink-500 transition-colors duration-[var(--dur-fast)] ease-[var(--ease-out)] hover:bg-cream-100 hover:text-ink-900 focus-visible:ring-4 focus-visible:outline-none";

/**
 * "‹ Tháng 9/2026 ›" — the toolbar's month control, mirrored in `?month=`.
 * One white card with the two arrows inside it, so it sits beside the class
 * picker as a peer control rather than as two loose buttons.
 */
export function MonthStepper({ month, onChange }: MonthStepperProps) {
  const [year = "", monthNumber = ""] = month.split("-");
  return (
    <div className="inline-flex shrink-0 items-center gap-1 rounded-[var(--radius-md)] bg-white p-1 shadow-soft-sm">
      <button
        type="button"
        aria-label="Tháng trước"
        onClick={() => onChange(shiftMonth(month, -1))}
        className={arrowClassName}
      >
        <HvIcon name="chevron-left" size={18} />
      </button>
      <span className="min-w-[120px] text-center font-display text-[14px] font-extrabold text-ink-900 tabular-nums">
        Tháng {Number(monthNumber)}/{year}
      </span>
      <button
        type="button"
        aria-label="Tháng sau"
        onClick={() => onChange(shiftMonth(month, 1))}
        className={arrowClassName}
      >
        <HvIcon name="chevron-right" size={18} />
      </button>
    </div>
  );
}
