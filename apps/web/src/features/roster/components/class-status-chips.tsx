import { HvChip, type HvBadgeVariant } from "@/components/hv";

import { classPhases, type ClassStats } from "../schemas/roster-schemas";
import { phaseLabel, phaseVariant } from "../lib/class-labels";
import type { ClassListView } from "../hooks/use-class-list-url-state";

interface ClassStatusChipsProps {
  stats: ClassStats | undefined;
  value: ClassListView;
  onChange: (view: ClassListView) => void;
  /** False on a list already narrowed to recruiting classes, where the chip would repeat "all". */
  includeRecruiting?: boolean;
}

/**
 * The status strip: one radio chip per phase plus "all" and "recruiting",
 * each carrying the count from `GET /classes/stats` over the same base set
 * as the table. Counts are
 * omitted (not zeroed) until the stats query resolves so a slow network
 * never flashes "0" over a populated table.
 */
export function ClassStatusChips({
  stats,
  value,
  onChange,
  includeRecruiting = true,
}: ClassStatusChipsProps) {
  const chips: { view: ClassListView; label: string; count?: number; dot?: HvBadgeVariant }[] = [
    { view: "all", label: "Tất cả", count: stats?.all },
    ...classPhases.map((phase) => ({
      view: phase,
      label: phaseLabel[phase],
      count: stats?.[phase],
      dot: phaseVariant[phase],
    })),
    ...(includeRecruiting
      ? [{ view: "recruiting" as const, label: "Cần tuyển sinh", count: stats?.recruiting }]
      : []),
  ];

  return (
    <div
      role="radiogroup"
      aria-label="Lọc theo trạng thái"
      className="flex flex-wrap items-center gap-1.5"
    >
      {chips.map((chip) => (
        <HvChip
          key={chip.view}
          role="radio"
          size="sm"
          pressed={chip.view === value}
          dot={chip.dot}
          count={chip.count}
          onClick={() => onChange(chip.view)}
        >
          {chip.label}
        </HvChip>
      ))}
    </div>
  );
}
