import { HvSelect } from "@/components/hv";
import { formatScheduleLabel, type Class } from "@/features/roster";

interface ClassSelectProps {
  classes: Class[];
  selected: Class | undefined;
  today: string;
  onSelect: (classId: string) => void;
}

/** "Toán 8 · Tối Thứ Ba" — the class as teachers say it, name and khung giờ in one breath. */
function classLabel(klass: Class, today: string): string {
  const schedule = formatScheduleLabel(klass.schedules, today);
  return schedule ? `${klass.name} · ${schedule}` : klass.name;
}

/**
 * The toolbar's class picker: a thin wrapper over `HvSelect` that turns each
 * class into a `{ label, meta }` pair so the trigger reads "Toán 8 · Tối Thứ
 * Ba" and the option shows name and khung giờ as separate spans. Re-picking
 * the current class is a no-op for the caller (`onSelect` fires only when
 * the id actually changes).
 */
export function ClassSelect({ classes, selected, today, onSelect }: ClassSelectProps) {
  return (
    <HvSelect
      aria-label={selected ? `Chọn lớp — đang xem ${classLabel(selected, today)}` : "Chọn lớp"}
      sheetTitle="Chọn lớp"
      searchNoun="lớp"
      placeholder="Chọn lớp"
      options={classes.map((klass) => ({
        value: klass.id,
        label: klass.name,
        meta: formatScheduleLabel(klass.schedules, today) || undefined,
      }))}
      value={selected?.id ?? ""}
      onValueChange={(classId) => {
        if (classId !== selected?.id) onSelect(classId);
      }}
      className="w-full min-w-0 sm:w-fit sm:max-w-[520px]"
    />
  );
}
