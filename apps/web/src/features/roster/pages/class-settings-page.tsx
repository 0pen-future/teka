import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useSessionsList } from "@/features/attendance";
import { formatMoney } from "@/lib/utils";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { MoneyInput } from "../components/money-input";
import { WeekdayChipsMulti } from "../components/weekday-chips";
import {
  useAddSchedule,
  useClass,
  useDeleteSchedule,
  useUpdateClass,
  useUpdateSchedule,
} from "../hooks/use-classes";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import { deriveScheduleForm, diffSchedules } from "../lib/schedule-diff";
import {
  classSettingsInputSchema,
  type Class,
  type ClassSettingsInput,
} from "../schemas/roster-schemas";

const today = () => new Date().toISOString().slice(0, 10);

/**
 * Month-start → today, plus the month's "07" label. The stat range must stop
 * at today: `GET /classes/:id/sessions` materializes every session in the
 * requested range, and rows written for future dates would freeze the old
 * timetable before the save can change it.
 */
function currentMonth() {
  const now = new Date();
  const first = new Date(now.getFullYear(), now.getMonth(), 1);
  const iso = (date: Date) =>
    `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(
      date.getDate(),
    ).padStart(2, "0")}`;
  return { from: iso(first), to: iso(now), label: String(now.getMonth() + 1).padStart(2, "0") };
}

function toDefaults(klass: Class): ClassSettingsInput {
  const { days, start_time } = deriveScheduleForm(klass.schedules, today());
  return {
    name: klass.name,
    days,
    start_time,
    default_unit_price: klass.default_unit_price,
  };
}

/**
 * "Cài đặt lớp" screen (prototype `classCfg`, reached from the tab row on
 * "Lớp & học sinh"): one form for name, weekly days, shared start time and
 * unit price. Saving fans out into `PUT /classes/:id` for name/price plus a
 * schedule diff — new rows are added first (`effective_from` = today), then
 * replaced rows are closed with `effective_to` = yesterday per the API's
 * close-and-replace contract — so changes apply from the next session, past
 * sessions stay explicable, and a mid-sequence failure can never leave the
 * class without a timetable.
 */
export function ClassSettingsPage() {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const { data: klass, isPending } = useClass(id);
  const { data: enrollmentsPage } = useEnrollmentsList({ class_id: id, per_page: 100 });
  const month = currentMonth();
  const { data: sessions } = useSessionsList(id, { from: month.from, to: month.to });

  const form = useForm<ClassSettingsInput>({
    resolver: zodResolver(classSettingsInputSchema),
    defaultValues: { name: "", days: [], start_time: "", default_unit_price: 0 },
  });
  const updateMutation = useUpdateClass(id ?? "");
  const addMutation = useAddSchedule(id ?? "");
  const closeMutation = useUpdateSchedule(id ?? "");
  const deleteMutation = useDeleteSchedule(id ?? "");
  const handleApiError = useApiFormErrors(form);

  // Reset once per class id, not per fetch: schedule mutations invalidate the
  // class detail, and a refetch-driven reset would wipe in-progress edits and
  // the partial-save error below.
  const resetForClassId = useRef<string | null>(null);
  useEffect(() => {
    if (klass && resetForClassId.current !== klass.id) {
      resetForClassId.current = klass.id;
      form.reset(toDefaults(klass));
    }
    // form is stable from react-hook-form and not a meaningful dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [klass]);

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }

  if (!klass) {
    return <p className="text-[13px] text-ink-400">Không tìm thấy lớp.</p>;
  }

  const backTo = `/students?class_id=${klass.id}`;
  const activeStudents = (enrollmentsPage?.items ?? []).filter(
    (enrollment) => !enrollment.ended_on,
  ).length;
  const monthSessions = (sessions ?? []).filter((session) => session.status !== "cancelled");
  const heldSessions = monthSessions.filter((session) => session.status === "held");
  const stats = [
    { label: "HỌC SINH", value: String(activeStudents) },
    { label: `BUỔI KỲ ${month.label}`, value: `${heldSessions.length}/${monthSessions.length}` },
    { label: "ĐƠN GIÁ HIỆN TẠI", value: formatMoney(klass.default_unit_price) },
  ];
  const rateChanged = form.watch("default_unit_price") !== klass.default_unit_price;
  const saving =
    updateMutation.isPending ||
    addMutation.isPending ||
    closeMutation.isPending ||
    deleteMutation.isPending;
  const { errors } = form.formState;

  const onSubmit = form.handleSubmit(async (values) => {
    const applyFrom = today();
    const diff = diffSchedules(klass.schedules, values.days, values.start_time, applyFrom);
    let applied = false;
    try {
      if (values.name !== klass.name || values.default_unit_price !== klass.default_unit_price) {
        await updateMutation.mutateAsync({
          name: values.name,
          start_date: klass.start_date,
          end_date: klass.end_date ?? "",
          default_unit_price: values.default_unit_price,
        });
        applied = true;
      }
      // Adds land before closes/deletes so an interrupted save can only leave
      // an extra row behind, never a class with no timetable at all.
      for (const input of diff.toAdd) {
        await addMutation.mutateAsync(input);
        applied = true;
      }
      for (const close of diff.toClose) {
        await closeMutation.mutateAsync({ scheduleId: close.id, input: close.input });
        applied = true;
      }
      for (const scheduleId of diff.toDelete) {
        await deleteMutation.mutateAsync(scheduleId);
        applied = true;
      }
      hvToast(`Đã lưu ${values.name.trim()} — áp dụng từ buổi kế tiếp`);
      void navigate(backTo);
    } catch (error) {
      if (applied) {
        form.setError("root", {
          message:
            "Chỉ lưu được một phần thay đổi — kiểm tra lại lịch của lớp rồi lưu lại lần nữa.",
        });
      } else {
        handleApiError(error);
      }
    }
  });

  return (
    <div className="flex flex-col gap-4">
      <div>
        <Link
          to={backTo}
          className="font-display text-[13px] font-bold text-ink-500 hover:text-mint-600"
        >
          ← Lớp &amp; học sinh
        </Link>
        <h1 className="mt-2 font-display text-[22px] font-bold text-ink-900">
          Cài đặt lớp — {klass.name}
        </h1>
        <p className="mt-1 text-[14px] text-ink-500">
          Thay đổi áp dụng từ buổi kế tiếp — các kỳ đã chốt không đổi.
        </p>
      </div>

      <div className="grid max-w-[640px] grid-cols-1 gap-3 sm:grid-cols-3">
        {stats.map((stat) => (
          <HvCard key={stat.label} padding="sm">
            <p className="text-[12px] font-bold tracking-[0.3px] text-ink-400">{stat.label}</p>
            <p className="mt-0.5 font-display text-[22px] font-bold text-ink-900">{stat.value}</p>
          </HvCard>
        ))}
      </div>

      <HvCard className="max-w-[640px]">
        <form onSubmit={(event) => void onSubmit(event)} noValidate>
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="class-settings-name">Tên lớp</FieldLabel>
              <Input
                id="class-settings-name"
                aria-invalid={Boolean(errors.name)}
                {...form.register("name")}
              />
              <FieldError errors={[errors.name]} />
            </Field>
            <Field data-invalid={Boolean(errors.days)}>
              <FieldLabel htmlFor="class-settings-days">Lịch trong tuần</FieldLabel>
              <WeekdayChipsMulti
                id="class-settings-days"
                value={form.watch("days")}
                onChange={(days) =>
                  form.setValue("days", days, { shouldValidate: true, shouldDirty: true })
                }
              />
              <FieldError errors={[errors.days]} />
            </Field>
            <div className="grid gap-3 sm:grid-cols-2">
              <Field data-invalid={Boolean(errors.start_time)}>
                <FieldLabel htmlFor="class-settings-start-time">Giờ học</FieldLabel>
                <Input
                  id="class-settings-start-time"
                  type="time"
                  aria-invalid={Boolean(errors.start_time)}
                  {...form.register("start_time")}
                />
                <FieldError errors={[errors.start_time]} />
              </Field>
              <Field data-invalid={Boolean(errors.default_unit_price)}>
                <FieldLabel htmlFor="class-settings-unit-price">Đơn giá / buổi (đ)</FieldLabel>
                <MoneyInput
                  id="class-settings-unit-price"
                  aria-invalid={Boolean(errors.default_unit_price)}
                  value={form.watch("default_unit_price")}
                  onChange={(value) =>
                    form.setValue("default_unit_price", value, {
                      shouldValidate: true,
                      shouldDirty: true,
                    })
                  }
                />
                <FieldError errors={[errors.default_unit_price]} />
              </Field>
            </div>
            {rateChanged ? (
              <p className="rounded-[var(--radius-md)] bg-sun-100 px-4 py-3 text-[13px] font-bold text-sun-600">
                Đơn giá mới chỉ áp cho lượt ghi danh từ nay về sau và buổi học kế tiếp. Học phí đã
                chốt và đã gửi không thay đổi.
              </p>
            ) : null}
            <FieldError errors={[errors.root]} />
          </FieldGroup>
          <div className="mt-5 flex items-center justify-end gap-2">
            <HvButton type="button" variant="ghost" onClick={() => void navigate(backTo)}>
              Hủy
            </HvButton>
            <HvButton type="submit" disabled={saving}>
              {saving ? "Đang lưu…" : "Lưu thay đổi"}
            </HvButton>
          </div>
        </form>
      </HvCard>
    </div>
  );
}
