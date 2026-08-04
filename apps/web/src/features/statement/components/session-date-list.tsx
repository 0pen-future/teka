import { cn } from "@/lib/utils";

import { formatChipDate } from "../lib/format-chip-date";

export type SessionChipTone = "mint" | "coral" | "cream";

const toneClassNames: Record<SessionChipTone, string> = {
  mint: "bg-mint-50 text-mint-600",
  coral: "bg-coral-100 text-coral-600",
  cream: "bg-cream-200 text-ink-400",
};

export interface SessionDateListProps {
  label: string;
  dates: string[];
  suffix: string;
  tone: SessionChipTone;
}

/**
 * A label plus wrapping date chips — never a table. A table with a dozen
 * date columns is exactly what forces horizontal scrolling on a phone, which
 * the mobile layout must avoid at any width down to 320px. Omits itself
 * entirely when the group has no sessions, rather than rendering an empty
 * "Vắng: —" line.
 */
export function SessionDateList({ label, dates, suffix, tone }: SessionDateListProps) {
  if (dates.length === 0) {
    return null;
  }
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-[13px]">
      <span className="font-semibold text-ink-500">{label}:</span>
      {dates.map((date, index) => (
        <span
          key={`${date}-${index}`}
          className={cn(
            "rounded-[var(--radius-pill)] px-2 py-0.5 font-semibold",
            toneClassNames[tone],
          )}
        >
          {formatChipDate(date)} {suffix}
        </span>
      ))}
    </div>
  );
}
