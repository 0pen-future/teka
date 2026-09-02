import { HvButton, HvIcon } from "@/components/hv";

import { shiftMonth } from "../lib/classbook-stats";

interface MonthStepperProps {
  /** "YYYY-MM". */
  month: string;
  onChange: (month: string) => void;
}

/** "‹ Tháng 9/2026 ›" — the toolbar's month control, mirrored in `?month=`. */
export function MonthStepper({ month, onChange }: MonthStepperProps) {
  const [year = "", monthNumber = ""] = month.split("-");
  return (
    <div className="flex items-center gap-1">
      <HvButton
        type="button"
        variant="ghost"
        size="sm"
        aria-label="Tháng trước"
        icon={<HvIcon name="chevron-left" />}
        onClick={() => onChange(shiftMonth(month, -1))}
        className="w-11 px-0"
      />
      <span className="min-w-[124px] text-center font-display text-[15px] font-extrabold text-ink-900 tabular-nums">
        Tháng {Number(monthNumber)}/{year}
      </span>
      <HvButton
        type="button"
        variant="ghost"
        size="sm"
        aria-label="Tháng sau"
        icon={<HvIcon name="chevron-right" />}
        onClick={() => onChange(shiftMonth(month, 1))}
        className="w-11 px-0"
      />
    </div>
  );
}
