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

export interface WeekdayChipsMultiProps {
  value: number[];
  onChange: (weekdays: number[]) => void;
  /** Accessible group label; defaults to the slot recipe's copy. */
  label?: string;
  id?: string;
}

/**
 * The 7 pill toggle chips a khung-giờ slot picks its weekdays with
 * (prototype `sl.dayChips`). Selected: mint-400 fill, white text. Idle:
 * white fill, line-200 border. The prototype's pills are ~36px tall; each
 * chip keeps a 44px minimum here to stay a comfortable touch target.
 */
export function WeekdayChipsMulti({
  value,
  onChange,
  label = "Ngày trong tuần",
  id,
}: WeekdayChipsMultiProps) {
  function toggle(weekday: number) {
    onChange(
      value.includes(weekday) ? value.filter((day) => day !== weekday) : [...value, weekday],
    );
  }

  return (
    <div role="group" aria-label={label} id={id} className="flex flex-wrap gap-1.5">
      {chipOrder.map((chip) => {
        const selected = value.includes(chip.weekday);
        return (
          <button
            key={chip.weekday}
            type="button"
            aria-pressed={selected}
            onClick={() => toggle(chip.weekday)}
            className={cn(
              "min-h-11 min-w-11 rounded-full border-2 px-3 font-display text-[13px] font-bold transition-colors",
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
