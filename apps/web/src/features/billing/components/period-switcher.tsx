import { useNavigate } from "react-router";

import { cn } from "@/lib/utils";

import { usePeriods } from "../hooks/use-billing";

export interface PeriodSwitcherProps {
  currentPeriodId: string;
}

/**
 * Current and previous period only (plan open question 3) — `usePeriods`
 * already fetches exactly that page (`per_page=2`, newest first).
 */
export function PeriodSwitcher({ currentPeriodId }: PeriodSwitcherProps) {
  const { data } = usePeriods();
  const navigate = useNavigate();

  if (!data || data.items.length < 2) {
    return null;
  }

  return (
    <div className="inline-flex rounded-[var(--radius-pill)] border border-line-200 bg-white p-1">
      {data.items.map((period) => {
        const active = period.id === currentPeriodId;
        return (
          <button
            key={period.id}
            type="button"
            onClick={() => void navigate(`/billing/${period.id}`)}
            className={cn(
              "rounded-[var(--radius-pill)] px-3 py-1.5 font-display text-[13px] font-bold transition-colors",
              active ? "bg-mint-400 text-white" : "text-ink-500 hover:bg-cream-100",
            )}
          >
            Tháng {period.month}/{period.year}
          </button>
        );
      })}
    </div>
  );
}
