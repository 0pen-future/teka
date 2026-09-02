import * as React from "react";

import { HvIcon, HvModal } from "@/components/hv";
import {
  ClassSearchEmptyNote,
  ClassSearchInput,
  formatScheduleSummary,
  useClassSearch,
  type Class,
} from "@/features/roster";
import { cn } from "@/lib/utils";

interface ClassSelectProps {
  classes: Class[];
  selected: Class | undefined;
  /** Active headcount of the selected class, for the trigger's sub-line. */
  headcount: number;
  today: string;
  onSelect: (classId: string) => void;
}

/**
 * The toolbar's class picker: one button naming the open class, and a
 * searchable list in a modal. The picker replaced a row of class tabs so a
 * center with twenty classes still has a one-line toolbar.
 */
export function ClassSelect({ classes, selected, headcount, today, onSelect }: ClassSelectProps) {
  const [open, setOpen] = React.useState(false);
  const search = useClassSearch(classes);

  function choose(classId: string) {
    setOpen(false);
    search.setQuery("");
    if (classId !== selected?.id) onSelect(classId);
  }

  return (
    <>
      <button
        type="button"
        aria-label={selected ? `Chọn lớp — đang xem ${selected.name}` : "Chọn lớp"}
        aria-haspopup="dialog"
        aria-expanded={open}
        onClick={() => setOpen(true)}
        className="flex min-h-11 items-center gap-2.5 rounded-[var(--radius-md)] bg-white px-3.5 py-1.5 text-left shadow-soft-sm transition-colors hover:bg-cream-100 focus-visible:ring-4 focus-visible:outline-none"
      >
        <span className="flex min-w-0 flex-col">
          <span className="truncate font-display text-[15px] font-extrabold text-ink-900">
            {selected?.name ?? "Chọn lớp"}
          </span>
          {selected ? (
            <span className="truncate text-[12px] text-ink-500">
              {headcount} HS · {formatScheduleSummary(selected.schedules, today) || "chưa có lịch"}
            </span>
          ) : null}
        </span>
        <HvIcon name="chevron-down" size={18} className="shrink-0 text-ink-400" aria-hidden />
      </button>

      <HvModal open={open} onOpenChange={setOpen} size="md" title="Chọn lớp">
        <div className="flex flex-col gap-3">
          {search.showSearch ? (
            <ClassSearchInput value={search.query} onChange={search.setQuery} />
          ) : null}
          {search.emptyNote ? <ClassSearchEmptyNote note={search.emptyNote} /> : null}
          <ul
            aria-label="Lớp"
            className="flex max-h-[min(60dvh,420px)] flex-col gap-1 overflow-y-auto"
          >
            {search.filtered.map((klass) => {
              const active = klass.id === selected?.id;
              return (
                <li key={klass.id}>
                  <button
                    type="button"
                    aria-current={active ? "true" : undefined}
                    onClick={() => choose(klass.id)}
                    className={cn(
                      "flex min-h-11 w-full items-center gap-3 rounded-[var(--radius-md)] px-3 py-2 text-left transition-colors focus-visible:ring-4 focus-visible:outline-none",
                      active ? "bg-mint-50" : "hover:bg-cream-100",
                    )}
                  >
                    <span className="flex min-w-0 flex-1 flex-col">
                      <span className="font-display text-[14.5px] font-extrabold text-ink-900">
                        {klass.name}
                      </span>
                      <span className="truncate text-[12px] text-ink-500">
                        {formatScheduleSummary(klass.schedules, today) || "chưa có lịch"}
                      </span>
                    </span>
                    {active ? (
                      <HvIcon name="check" size={18} className="text-mint-600" aria-hidden />
                    ) : null}
                  </button>
                </li>
              );
            })}
          </ul>
        </div>
      </HvModal>
    </>
  );
}
