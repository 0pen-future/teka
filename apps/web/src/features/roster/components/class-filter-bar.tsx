import { useEffect, useId, useRef, useState } from "react";

import { HvSelect } from "@/components/hv";
import { Input } from "@/components/ui/input";

import { shiftOptions, weekdayOptions } from "../lib/class-labels";
import type { ClassListUrlValues } from "../hooks/use-class-list-url-state";

interface ClassFilterBarProps {
  q: string;
  weekday: ClassListUrlValues["weekday"];
  shift: ClassListUrlValues["shift"];
  onChange: (partial: Partial<ClassListUrlValues>) => void;
}

/**
 * Weekday / shift selects plus the search box above the class table. The
 * selects write straight to the URL; the search box settles for 300 ms first
 * so a half-typed name never reaches the API.
 */
export function ClassFilterBar({ q, weekday, shift, onChange }: ClassFilterBarProps) {
  const [query, setQuery] = useState(q);
  // The last value this box wrote to the URL. A `q` that differs from it
  // came from elsewhere (the back button, a shared link) and the box follows
  // it instead of re-arming its own stale text over it.
  const committed = useRef(q);
  const weekdayLabelId = useId();
  const shiftLabelId = useId();

  useEffect(() => {
    if (q === committed.current) {
      return;
    }
    committed.current = q;
    setQuery(q);
  }, [q]);

  useEffect(() => {
    if (query === q) {
      return;
    }
    const timer = setTimeout(() => {
      committed.current = query;
      onChange({ q: query });
    }, 300);
    return () => clearTimeout(timer);
  }, [query, q, onChange]);

  return (
    <div className="flex flex-col gap-2.5 sm:flex-row sm:flex-wrap sm:items-center">
      <span id={weekdayLabelId} className="sr-only">
        Ngày học
      </span>
      <HvSelect
        options={weekdayOptions}
        value={weekday}
        onValueChange={(next) => onChange({ weekday: next as ClassListUrlValues["weekday"] })}
        sheetTitle="Ngày học"
        labelId={weekdayLabelId}
        className="sm:min-w-[170px]"
      />
      <span id={shiftLabelId} className="sr-only">
        Ca học
      </span>
      <HvSelect
        options={shiftOptions}
        value={shift}
        onValueChange={(next) => onChange({ shift: next as ClassListUrlValues["shift"] })}
        sheetTitle="Ca học"
        labelId={shiftLabelId}
        className="sm:min-w-[150px]"
      />
      <Input
        type="search"
        aria-label="Tìm lớp học"
        placeholder="Tìm kiếm theo tên, mã lớp học"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className="min-h-11 min-w-0 flex-1 px-3.5 text-[13.5px] sm:min-w-[220px]"
      />
    </div>
  );
}
