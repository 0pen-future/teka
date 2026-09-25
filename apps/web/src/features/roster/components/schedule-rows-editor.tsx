import { Trash2 } from "lucide-react";

import { HvSelect } from "@/components/hv";
import { Input } from "@/components/ui/input";

import { addMinutes, formatWeekday } from "../lib/roster-format";
import type { ScheduleRowInput } from "../schemas/roster-schemas";

/** The first problem on a row and the input it belongs to. */
export type ScheduleRowError = { field: keyof ScheduleRowInput; message: string } | undefined;

/** Monday first, Sunday last — the prototype's day picker order. */
const dayOptions = [1, 2, 3, 4, 5, 6, 0].map((weekday) => ({
  value: String(weekday),
  label: formatWeekday(weekday, { word: true }),
}));

/** "Thứ Ba 19:00–20:30"; just the weekday until time and duration are both set. */
function scheduleRowSummary(row: ScheduleRowInput): string {
  const day = formatWeekday(row.weekday, { word: true });
  if (!/^\d{2}:\d{2}$/.test(row.start_time) || !row.duration_min) return day;
  return `${day} ${row.start_time}–${addMinutes(row.start_time, row.duration_min)}`;
}

/**
 * The wizard's weekly timetable: one numbered row per session with its own
 * weekday, start time and duration (prototype `cls.wiz.slots`).
 */
export function ScheduleRowsEditor({
  value,
  onChange,
  rowErrors,
  idPrefix,
}: {
  value: ScheduleRowInput[];
  onChange: (next: ScheduleRowInput[]) => void;
  rowErrors: ScheduleRowError[];
  /** Prefix for the per-row error ids the inputs point at. */
  idPrefix: string;
}) {
  function patch(index: number, next: Partial<ScheduleRowInput>) {
    onChange(value.map((row, at) => (at === index ? { ...row, ...next } : row)));
  }

  return (
    <ol className="flex flex-col gap-2.5">
      {value.map((row, index) => {
        const number = index + 1;
        const error = rowErrors[index];
        const errorId = `${idPrefix}-row-${number}-error`;
        const describedBy = error ? errorId : undefined;
        return (
          <li
            key={index}
            className="rounded-[16px] border-2 border-line-100 bg-cream-50 p-3"
            data-invalid={error ? true : undefined}
          >
            <div className="flex flex-wrap items-center gap-2.5">
              <span
                aria-hidden
                className="inline-flex size-7 shrink-0 items-center justify-center rounded-full bg-sky-100 text-[12.5px] font-extrabold text-sky-600"
              >
                {number}
              </span>
              <HvSelect
                aria-label={`Ngày học lịch ${number}`}
                className="w-[140px]"
                value={String(row.weekday)}
                onValueChange={(next) => patch(index, { weekday: Number(next) })}
                options={dayOptions}
                sheetTitle="Chọn ngày học"
                searchThreshold={99}
                aria-invalid={error?.field === "weekday"}
                aria-describedby={describedBy}
              />
              <Input
                type="time"
                aria-label={`Giờ bắt đầu lịch ${number}`}
                aria-invalid={error?.field === "start_time"}
                aria-describedby={describedBy}
                className="w-[120px]"
                value={row.start_time}
                onChange={(event) => patch(index, { start_time: event.target.value })}
              />
              <div className="flex items-center gap-1.5">
                <Input
                  type="number"
                  min={1}
                  step={5}
                  aria-label={`Thời lượng lịch ${number} (phút)`}
                  aria-invalid={error?.field === "duration_min"}
                  aria-describedby={describedBy}
                  className="w-[88px]"
                  value={row.duration_min ?? ""}
                  onChange={(event) =>
                    patch(index, {
                      duration_min: event.target.value === "" ? null : Number(event.target.value),
                    })
                  }
                />
                <span className="text-[13px] font-bold text-ink-400">phút</span>
              </div>
              <span className="min-w-0 flex-1 text-[13px] font-bold text-ink-500">
                {scheduleRowSummary(row)}
              </span>
              <button
                type="button"
                aria-label={`Xóa lịch ${number}`}
                onClick={() => onChange(value.filter((_, at) => at !== index))}
                className="inline-flex size-9 items-center justify-center rounded-full text-ink-400 transition-colors hover:bg-coral-100 hover:text-coral-500"
              >
                <Trash2 className="size-4" strokeWidth={2.2} />
              </button>
            </div>
            {error ? (
              <p
                id={errorId}
                role="alert"
                className="mt-1.5 text-[12.5px] font-bold text-coral-500"
              >
                {error.message}
              </p>
            ) : null}
          </li>
        );
      })}
    </ol>
  );
}
