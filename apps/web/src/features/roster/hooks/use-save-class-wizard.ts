import { useQueryClient } from "@tanstack/react-query";

import { classesKeys } from "./roster-keys";
import { useSendClassInvitation } from "./use-class-invitations";
import {
  useAddSchedule,
  useDeleteSchedule,
  useUpdateClass,
  useUpdateSchedule,
} from "./use-classes";
import { diffScheduleRows } from "../lib/schedule-diff";
import {
  toClassWizardUpdateInput,
  type Class,
  type ClassWizardInput,
} from "../schemas/roster-schemas";

/** Where a save stopped after some of its requests already landed. */
export type SaveClassWizardStage = "schedule" | "invitation";

export type SaveClassWizardResult =
  | { ok: true }
  | { ok: false; partial: true; stage: SaveClassWizardStage; error: unknown }
  | { ok: false; partial: false; error: unknown };

/**
 * Saves the "Sửa lớp học" wizard: class fields first (so a switch back to a
 * scheduled class is accepted before rows are added), then the timetable
 * diff, then the planned-teacher invitation. The class is kept schedulable
 * even if a later request fails.
 */
export function useSaveClassWizard(klass: Class | undefined) {
  const id = klass?.id ?? "";
  const queryClient = useQueryClient();
  const update = useUpdateClass(id);
  const add = useAddSchedule(id);
  const close = useUpdateSchedule(id);
  const remove = useDeleteSchedule(id);
  const invite = useSendClassInvitation(id);
  const isPending =
    update.isPending || add.isPending || close.isPending || remove.isPending || invite.isPending;

  async function save(values: ClassWizardInput, applyFrom: string): Promise<SaveClassWizardResult> {
    if (!klass) return { ok: false, partial: false, error: new Error("Không tìm thấy lớp") };
    const rows = values.study_mode === "scheduled" ? values.slots : [];
    const diff = diffScheduleRows(klass.schedules, rows, applyFrom);
    const input = toClassWizardUpdateInput(klass, values);
    let applied = false;
    let stage: SaveClassWizardStage = "schedule";
    try {
      if (input) {
        await update.mutateAsync(input);
        applied = true;
      }
      // Adds precede closes/deletes: interruption may leave an extra row, never an empty timetable.
      for (const row of diff.toAdd) {
        await add.mutateAsync(row);
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
      stage = "invitation";
      if (values.teacher_id !== "") {
        await invite.mutateAsync({ teacher_id: values.teacher_id, role_key: "giao_vien" });
      }
      return { ok: true };
    } catch (error) {
      // A retry diffs against the class it reads, so it must see what already landed.
      if (applied) await queryClient.invalidateQueries({ queryKey: classesKeys.detail(id) });
      return applied
        ? { ok: false, partial: true, stage, error }
        : { ok: false, partial: false, error };
    }
  }

  return { save, isPending };
}
