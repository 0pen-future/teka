import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";

import { HvButton } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";

import { WeekdayChips } from "./weekday-chips";
import { useAddSchedule, useDeleteSchedule } from "../hooks/use-classes";
import { formatWeekday } from "../lib/roster-format";
import { scheduleInputSchema, type Schedule, type ScheduleInput } from "../schemas/roster-schemas";

export interface ScheduleEditorProps {
  classId: string;
  schedules: Schedule[];
}

const newScheduleDefaults: ScheduleInput = {
  weekday: 1,
  start_time: "",
  duration_min: 90,
  effective_from: "",
  effective_to: "",
};

/**
 * One row per weekly schedule entry, reusing the `WeekdayChips` picker from
 * `ClassDialog`. Adding or removing a row is one mutation each — the API
 * only regenerates sessions on or after `effective_from`, so an edit here
 * never rewrites sessions already attended or billed.
 */
export function ScheduleEditor({ classId, schedules }: ScheduleEditorProps) {
  const addMutation = useAddSchedule(classId);
  const deleteMutation = useDeleteSchedule(classId);
  const form = useForm<ScheduleInput>({
    resolver: zodResolver(scheduleInputSchema),
    defaultValues: newScheduleDefaults,
  });
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit((values) => {
    addMutation.mutate(values, {
      onSuccess: () => form.reset(newScheduleDefaults),
    });
  });

  return (
    <div>
      <ul className="space-y-2">
        {schedules.map((schedule) => (
          <li
            key={schedule.id}
            className="flex items-center justify-between rounded-[var(--radius-md)] border border-line-200 bg-white px-3 py-2"
          >
            <div>
              <p className="font-display text-[14px] font-bold text-ink-900">
                {formatWeekday(schedule.weekday)} · {schedule.start_time} · {schedule.duration_min}{" "}
                phút
              </p>
              <p className="text-[13px] text-ink-400">
                Áp dụng từ {schedule.effective_from}
                {schedule.effective_to ? ` đến ${schedule.effective_to}` : ""}
              </p>
            </div>
            <HvButton
              type="button"
              variant="ghost"
              size="sm"
              disabled={deleteMutation.isPending}
              onClick={() => deleteMutation.mutate(schedule.id)}
            >
              Xoá
            </HvButton>
          </li>
        ))}
        {schedules.length === 0 ? (
          <li className="rounded-[var(--radius-md)] border border-dashed border-line-200 px-3 py-4 text-center text-[13px] text-ink-400">
            Lớp chưa có lịch học nào.
          </li>
        ) : null}
      </ul>
      <form
        onSubmit={(event) => void onSubmit(event)}
        noValidate
        className="mt-4 rounded-[var(--radius-md)] border border-line-200 p-3"
      >
        <p className="mb-2 font-display text-[14px] font-bold text-ink-900">Thêm lịch học</p>
        <p className="mb-3 text-[13px] text-ink-400">
          Thay đổi lịch chỉ ảnh hưởng đến các buổi học được tạo trong tương lai.
        </p>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.weekday)}>
            <FieldLabel htmlFor="schedule-weekday">Ngày trong tuần</FieldLabel>
            <WeekdayChips
              id="schedule-weekday"
              value={form.watch("weekday")}
              onChange={(weekday) =>
                form.setValue("weekday", weekday, { shouldValidate: true, shouldDirty: true })
              }
            />
            <FieldError errors={[errors.weekday]} />
          </Field>
          <Field data-invalid={Boolean(errors.start_time)}>
            <FieldLabel htmlFor="schedule-start-time">Giờ học</FieldLabel>
            <Input
              id="schedule-start-time"
              type="time"
              aria-invalid={Boolean(errors.start_time)}
              {...form.register("start_time")}
            />
            <FieldError errors={[errors.start_time]} />
          </Field>
          <Field data-invalid={Boolean(errors.duration_min)}>
            <FieldLabel htmlFor="schedule-duration">Thời lượng (phút)</FieldLabel>
            <Input
              id="schedule-duration"
              type="number"
              min={1}
              aria-invalid={Boolean(errors.duration_min)}
              {...form.register("duration_min", { valueAsNumber: true })}
            />
            <FieldError errors={[errors.duration_min]} />
          </Field>
          <Field data-invalid={Boolean(errors.effective_from)}>
            <FieldLabel htmlFor="schedule-effective-from">Áp dụng từ ngày</FieldLabel>
            <Input
              id="schedule-effective-from"
              type="date"
              aria-invalid={Boolean(errors.effective_from)}
              {...form.register("effective_from")}
            />
            <FieldError errors={[errors.effective_from]} />
          </Field>
          <Field data-invalid={Boolean(errors.effective_to)}>
            <FieldLabel htmlFor="schedule-effective-to">Áp dụng đến ngày</FieldLabel>
            <Input
              id="schedule-effective-to"
              type="date"
              aria-invalid={Boolean(errors.effective_to)}
              {...form.register("effective_to")}
            />
            <FieldError errors={[errors.effective_to]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
        <HvButton type="submit" size="sm" className="mt-3" disabled={addMutation.isPending}>
          {addMutation.isPending ? "Đang thêm…" : "Thêm lịch học"}
        </HvButton>
      </form>
    </div>
  );
}
