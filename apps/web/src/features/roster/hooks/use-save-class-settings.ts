import {
  useAddSchedule,
  useDeleteSchedule,
  useUpdateClass,
  useUpdateSchedule,
} from "./use-classes";
import { diffSchedules } from "../lib/schedule-diff";
import type { Class, ClassSettingsInput } from "../schemas/roster-schemas";

export type SaveClassSettingsResult =
  { ok: true } | { ok: false; partial: true } | { ok: false; partial: false; error: unknown };

/** Keep the class schedulable even if a later request in the save fails. */
export function useSaveClassSettings(klass: Class | undefined) {
  const id = klass?.id ?? "";
  const update = useUpdateClass(id);
  const add = useAddSchedule(id);
  const close = useUpdateSchedule(id);
  const remove = useDeleteSchedule(id);
  const isPending = update.isPending || add.isPending || close.isPending || remove.isPending;

  async function save(
    values: ClassSettingsInput,
    applyFrom: string,
  ): Promise<SaveClassSettingsResult> {
    if (!klass) return { ok: false, partial: false, error: new Error("Không tìm thấy lớp") };
    const diff = diffSchedules(klass.schedules, values.slots, applyFrom);
    let applied = false;
    try {
      if (values.name !== klass.name || values.default_unit_price !== klass.default_unit_price) {
        await update.mutateAsync({
          name: values.name,
          start_date: klass.start_date,
          end_date: klass.end_date ?? "",
          default_unit_price: values.default_unit_price,
        });
        applied = true;
      }
      // Adds precede closes/deletes: interruption may leave an extra row, never an empty timetable.
      for (const input of diff.toAdd) {
        await add.mutateAsync(input);
        applied = true;
      }
      for (const item of diff.toClose) {
        await close.mutateAsync({ scheduleId: item.id, input: item.input });
        applied = true;
      }
      for (const scheduleId of diff.toDelete) {
        await remove.mutateAsync(scheduleId);
        applied = true;
      }
      return { ok: true };
    } catch (error) {
      return applied ? { ok: false, partial: true } : { ok: false, partial: false, error };
    }
  }

  return { save, isPending };
}
