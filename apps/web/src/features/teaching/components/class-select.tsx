import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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

const triggerLabelClassName = "font-display text-[15px] font-extrabold";
const itemLabelClassName = "font-display text-[15px]";

/**
 * The toolbar's class picker: a plain dropdown anchored under the trigger.
 * Trigger and options both carry the class label with its timetable, so two
 * same-named classes stay tellable apart. The trigger sits at content width
 * but may shrink and truncate, so a long label never pushes the rest of the
 * toolbar onto a second line. Colours are pinned to the DS palette rather
 * than the shadcn popover tokens, so the list stays a white card even when
 * something outside the app toggles the `.dark` class. The hover colours
 * are set by overriding the accent variables on the item: the base item rule
 * paints every descendant `accent-foreground` on focus, and a variable
 * override is the one thing that beats it regardless of CSS order. Hover
 * is solid mint with white text in every theme, and the checked item's
 * pale tint only shows while it is not hovered, so the highlighted row never
 * ends up white-on-pale.
 */
export function ClassSelect({ classes, selected, today, onSelect }: ClassSelectProps) {
  return (
    <Select
      value={selected?.id ?? ""}
      onValueChange={(classId) => {
        if (classId !== selected?.id) onSelect(classId);
      }}
    >
      <SelectTrigger
        aria-label={selected ? `Chọn lớp — đang xem ${classLabel(selected, today)}` : "Chọn lớp"}
        className="w-full min-w-0 gap-2.5 border-0 pr-3 pl-4 text-ink-900 shadow-soft-sm focus-visible:ring-4 sm:w-fit sm:max-w-[520px] *:data-[slot=select-value]:min-w-0"
      >
        <SelectValue placeholder="Chọn lớp">
          {selected ? (
            <span className={`${triggerLabelClassName} truncate`}>
              {classLabel(selected, today)}
            </span>
          ) : null}
        </SelectValue>
      </SelectTrigger>
      <SelectContent
        position="popper"
        align="start"
        sideOffset={6}
        className="rounded-[var(--radius-md)] bg-white p-1.5 text-ink-900 shadow-soft-lg ring-line-100"
      >
        {classes.map((klass) => {
          const label = classLabel(klass, today);
          return (
            <SelectItem
              key={klass.id}
              value={klass.id}
              textValue={label}
              className="min-h-11 rounded-[var(--radius-sm)] py-2 pl-3 text-ink-900 focus:[--color-accent:var(--color-mint-600)] focus:[--color-accent-foreground:var(--color-white)] data-[state=checked]:not-focus:bg-mint-50 data-[state=checked]:not-focus:text-mint-700 [&_svg]:text-mint-600"
            >
              <span className={itemLabelClassName}>{label}</span>
            </SelectItem>
          );
        })}
      </SelectContent>
    </Select>
  );
}
