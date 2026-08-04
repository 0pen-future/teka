import { cn } from "@/lib/utils";

/**
 * Display order follows the prototype ("T2…CN", week starting Monday), but
 * the underlying value is still the `class_schedules.weekday` integer
 * (0 = Chủ nhật … 6 = Thứ 7, `docs/schema_design.sql:149`).
 */
const chipOrder: { weekday: number; label: string }[] = [
  { weekday: 1, label: "T2" },
  { weekday: 2, label: "T3" },
  { weekday: 3, label: "T4" },
  { weekday: 4, label: "T5" },
  { weekday: 5, label: "T6" },
  { weekday: 6, label: "T7" },
  { weekday: 0, label: "CN" },
];

export interface WeekdayChipsProps {
  value: number | null;
  onChange: (weekday: number) => void;
  /** Accessible group label; defaults to the modalClass recipe's copy. */
  label?: string;
  id?: string;
}

/**
 * The 7 toggle chips from prototype `modalClass`, reused as-is by
 * `ScheduleEditor` per the Design Spec ("reuses the 7-chip weekday row from
 * modalClass"). Selected: mint-400 fill, white text. Idle: white fill,
 * line-200 border. Each chip is at least 44px tall to stay a comfortable
 * touch target.
 */
export function WeekdayChips({
  value,
  onChange,
  label = "Ngày trong tuần",
  id,
}: WeekdayChipsProps) {
  return (
    <div role="group" aria-label={label} id={id} className="flex flex-wrap gap-2">
      {chipOrder.map((chip) => {
        const selected = value === chip.weekday;
        return (
          <button
            key={chip.weekday}
            type="button"
            aria-pressed={selected}
            onClick={() => onChange(chip.weekday)}
            className={cn(
              "min-h-11 min-w-11 rounded-[var(--radius-md)] border px-3 font-display text-[14px] font-bold transition-colors",
              selected
                ? "border-mint-400 bg-mint-400 text-white"
                : "border-line-200 bg-white text-ink-500 hover:bg-cream-100",
            )}
          >
            {chip.label}
          </button>
        );
      })}
    </div>
  );
}
