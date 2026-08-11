import { Input } from "@/components/ui/input";

import { WeekdayChipsMulti } from "./weekday-chips";
import { emptySlot } from "../lib/schedule-diff";
import type { ScheduleSlotInput } from "../schemas/roster-schemas";

export interface SlotErrors {
  time?: string;
  days?: string;
}

export interface ScheduleSlotsEditorProps {
  value: ScheduleSlotInput[];
  onChange: (slots: ScheduleSlotInput[]) => void;
  /** Per-slot validation messages, index-aligned with `value`. */
  slotErrors?: (SlotErrors | undefined)[];
  /** Prefixes each slot's time-input id so two editors can coexist. */
  idPrefix: string;
}

/**
 * The khung-giờ list shared by `ClassDialog` and `ClassSettingsPage`
 * (prototype `modalClass.slots` / `classCfg.slots`): each card pairs one
 * start time with the weekdays it repeats on; "+ Thêm khung giờ khác"
 * appends the starter slot and "Xóa" only renders while another slot
 * remains, so a class can never edit its way to zero khung giờ.
 */
export function ScheduleSlotsEditor({
  value,
  onChange,
  slotErrors,
  idPrefix,
}: ScheduleSlotsEditorProps) {
  function updateSlot(index: number, patch: Partial<ScheduleSlotInput>) {
    onChange(value.map((slot, i) => (i === index ? { ...slot, ...patch } : slot)));
  }

  return (
    <div className="flex flex-col gap-2">
      {value.map((slot, index) => (
        <div
          key={index}
          className="rounded-[var(--radius-lg)] border border-line-200 bg-cream-100 px-3 py-2.5"
        >
          <div className="flex flex-wrap items-center gap-2.5">
            <span className="text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400">
              KHUNG GIỜ {index + 1}
            </span>
            <Input
              id={`${idPrefix}-slot-time-${index + 1}`}
              aria-label={`Giờ học khung ${index + 1}`}
              type="time"
              className="w-auto py-1.5 text-[13.5px]"
              aria-invalid={Boolean(slotErrors?.[index]?.time)}
              value={slot.start_time}
              onChange={(event) => updateSlot(index, { start_time: event.target.value })}
            />
            <span
              className={
                slot.days.length
                  ? "text-[12px] font-extrabold text-mint-600"
                  : "text-[12px] font-bold text-ink-400"
              }
            >
              {slot.days.length ? `${slot.days.length} buổi/tuần` : "Chưa chọn ngày"}
            </span>
            {value.length > 1 ? (
              <button
                type="button"
                onClick={() => onChange(value.filter((_, i) => i !== index))}
                className="ml-auto px-1.5 py-1 text-[12.5px] font-extrabold text-coral-400 hover:text-coral-600"
              >
                Xóa
              </button>
            ) : null}
          </div>
          <div className="mt-2">
            <WeekdayChipsMulti
              label={`Ngày học khung ${index + 1}`}
              value={slot.days}
              onChange={(days) => updateSlot(index, { days })}
            />
          </div>
          {slotErrors?.[index]?.time ?? slotErrors?.[index]?.days ? (
            <p role="alert" className="mt-1.5 text-sm text-destructive">
              {slotErrors?.[index]?.time ?? slotErrors?.[index]?.days}
            </p>
          ) : null}
        </div>
      ))}
      <button
        type="button"
        onClick={() => onChange([...value, emptySlot()])}
        className="w-full rounded-[14px] border-2 border-dashed border-line-300 px-3.5 py-2 text-[12.5px] font-extrabold text-mint-600 hover:border-mint-400 hover:bg-mint-50"
      >
        + Thêm khung giờ khác
      </button>
    </div>
  );
}
