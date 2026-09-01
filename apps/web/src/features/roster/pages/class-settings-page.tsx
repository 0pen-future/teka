import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef, useState } from "react";
import { useForm } from "react-hook-form";
import { Link, useNavigate, useParams } from "react-router";

import { HvButton, HvCard, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useSessionsList } from "@/features/attendance";
import { useCenter, type CenterMember } from "@/features/center";
import { useCenterContext } from "@/features/teaching";
import { ApiError } from "@/lib/api/errors";
import { formatMoney } from "@/lib/utils";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { ClassStaffSection } from "../components/class-staff-section";
import { MoneyInput } from "../components/money-input";
import { ScheduleSlotsEditor } from "../components/schedule-slots-editor";
import {
  useAddSchedule,
  useClass,
  useDeleteSchedule,
  useReassignTeacher,
  useUpdateClass,
  useUpdateSchedule,
} from "../hooks/use-classes";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import { canWriteClass } from "../lib/class-permissions";
import { currentMonth } from "../lib/current-month";
import {
  deriveScheduleSlots,
  diffSchedules,
  emptySlot,
  weeklySessionCount,
} from "../lib/schedule-diff";
import {
  classSettingsInputSchema,
  type Class,
  type ClassSettingsInput,
} from "../schemas/roster-schemas";

const today = () => new Date().toISOString().slice(0, 10);

function toDefaults(klass: Class): ClassSettingsInput {
  const slots = deriveScheduleSlots(klass.schedules, today());
  return {
    name: klass.name,
    slots: slots.length ? slots : [emptySlot()],
    default_unit_price: klass.default_unit_price,
  };
}

/**
 * "Cài đặt lớp" screen (prototype `classCfg`): the owner arrives from the
 * per-row ⚙ link on the classes tab of "Lớp & học sinh"; members arrive from
 * their teaching screens (classbook, records). One form for name, khung-giờ
 * slots and unit price.
 * Saving fans out into `PUT /classes/:id` for name/price plus a
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
  const { data: center } = useCenter();
  const { isOwner } = useCenterContext();
  const { data: enrollmentsPage } = useEnrollmentsList({ class_id: id, per_page: 100 });
  const month = currentMonth();
  const { data: sessions } = useSessionsList(id, { from: month.from, to: month.to });

  const form = useForm<ClassSettingsInput>({
    resolver: zodResolver(classSettingsInputSchema),
    defaultValues: { name: "", slots: [emptySlot()], default_unit_price: 0 },
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

  const canWrite = canWriteClass(isOwner, klass);
  // Members can read this page, but the roster screen behind the old back
  // link is owner-only now — send them to their per-class records instead.
  const backTo = isOwner
    ? `/students?tab=students&class_id=${klass.id}`
    : `/records?class_id=${klass.id}`;
  const backLabel = isOwner ? "← Lớp & học sinh" : "← Hồ sơ học sinh";
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
  const slots = form.watch("slots");

  const onSubmit = form.handleSubmit(async (values) => {
    const applyFrom = today();
    const diff = diffSchedules(klass.schedules, values.slots, applyFrom);
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
          {backLabel}
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
            <Field data-invalid={Boolean(errors.slots)}>
              <div className="flex items-baseline gap-2">
                <FieldLabel>Lịch học trong tuần</FieldLabel>
                {weeklySessionCount(slots) > 0 ? (
                  <span className="text-[12.5px] font-bold text-ink-400">
                    · {weeklySessionCount(slots)} buổi/tuần
                  </span>
                ) : null}
              </div>
              <p className="text-[12.5px] text-ink-400">
                Mỗi khung giờ chọn được nhiều ngày. Lớp học nhiều giờ khác nhau thì thêm khung giờ
                mới.
              </p>
              <ScheduleSlotsEditor
                idPrefix="class-settings"
                value={slots}
                onChange={(next) =>
                  form.setValue("slots", next, { shouldValidate: true, shouldDirty: true })
                }
                slotErrors={slots.map((_, index) => ({
                  time: errors.slots?.[index]?.start_time?.message,
                  days: errors.slots?.[index]?.days?.message,
                }))}
              />
              <FieldError errors={[errors.slots?.root]} />
            </Field>
            <Field className="max-w-[280px]" data-invalid={Boolean(errors.default_unit_price)}>
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
            {rateChanged ? (
              <p className="rounded-[var(--radius-md)] bg-sun-100 px-4 py-3 text-[13px] font-bold text-sun-600">
                Đơn giá mới chỉ áp cho lượt ghi danh từ nay về sau và buổi học kế tiếp. Học phí đã
                chốt và đã gửi không thay đổi.
              </p>
            ) : null}
            {!canWrite ? (
              <p className="text-[13px] text-ink-400">
                Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được cài đặt lớp.
              </p>
            ) : null}
            <FieldError errors={[errors.root]} />
          </FieldGroup>
          <div className="mt-5 flex items-center justify-end gap-2">
            <HvButton type="button" variant="ghost" onClick={() => void navigate(backTo)}>
              Hủy
            </HvButton>
            <HvButton type="submit" disabled={saving || !canWrite}>
              {saving ? "Đang lưu…" : "Lưu thay đổi"}
            </HvButton>
          </div>
        </form>
      </HvCard>

      <ClassStaffSection classId={klass.id} />

      {/* Owner-only: `GET /centers/me` carries `members` only in the owner body,
          so the narrowing doubles as the role gate. The API's own owner check
          is the real authorization. */}
      {center && "members" in center ? (
        <TeacherHandoffCard klass={klass} members={center.members} />
      ) : null}
    </div>
  );
}

/**
 * "Giáo viên phụ trách" — hands the class to another center member via
 * `PUT /classes/:id/teacher`. Handing a class to the owner themselves is legal,
 * so the target list excludes only the class's current teacher, never the
 * owner. The confirm is a two-click reveal (no `window.confirm`, matching the
 * rest of the app): the primary button arms, a second explicit button commits.
 */
function TeacherHandoffCard({ klass, members }: { klass: Class; members: CenterMember[] }) {
  const [targetId, setTargetId] = useState("");
  const [arming, setArming] = useState(false);
  const reassign = useReassignTeacher(klass.id);

  const currentTeacher = members.find((member) => member.id === klass.teacher_id);
  const targets = members.filter((member) => member.id !== klass.teacher_id);
  const errorMessage =
    reassign.error instanceof ApiError
      ? reassign.error.message
      : reassign.error
        ? "Không bàn giao được lớp. Thử lại sau."
        : null;

  function confirm() {
    if (!targetId) {
      return;
    }
    reassign.mutate(targetId, {
      onSuccess: (result) => {
        const name = members.find((member) => member.id === result.teacher_id)?.full_name;
        hvToast(name ? `Đã bàn giao lớp cho ${name}` : "Đã bàn giao lớp");
        setArming(false);
        setTargetId("");
      },
    });
  }

  return (
    <HvCard id="teacher-handoff" className="max-w-[640px]">
      <p className="font-display text-[16px] font-bold text-ink-900">Giáo viên phụ trách</p>
      <p className="mt-0.5 text-[13px] text-ink-400">
        Giáo viên hiện tại:{" "}
        <span className="font-bold text-ink-700">
          {currentTeacher ? currentTeacher.full_name : "Không rõ"}
        </span>
        . Bàn giao sẽ chuyển lớp, lịch học và các buổi <em>đã lên lịch</em> từ hôm nay trở đi sang
        giáo viên mới. Buổi đã dạy, đã hủy và học phí đã chốt vẫn giữ nguyên.
      </p>

      {targets.length === 0 ? (
        <p className="mt-3 text-[13px] text-ink-400">
          Chưa có giáo viên khác trong trung tâm để bàn giao.
        </p>
      ) : (
        <div className="mt-3 flex flex-col gap-3">
          <Field className="max-w-[320px]">
            <FieldLabel htmlFor="handoff-teacher">Bàn giao cho</FieldLabel>
            <select
              id="handoff-teacher"
              value={targetId}
              onChange={(event) => {
                setTargetId(event.target.value);
                // Changing the target un-arms so a confirm always reflects the
                // teacher currently shown in the select.
                setArming(false);
              }}
              disabled={reassign.isPending}
              className="min-h-11 rounded-[var(--radius-md)] border border-line-200 bg-white px-3 text-[14px] text-ink-900"
            >
              <option value="">— Chọn giáo viên —</option>
              {targets.map((member) => (
                <option key={member.id} value={member.id}>
                  {member.full_name}
                  {member.is_owner ? " (chủ trung tâm)" : ""}
                </option>
              ))}
            </select>
          </Field>

          {errorMessage ? <p className="text-[13px] text-coral-600">{errorMessage}</p> : null}

          {arming ? (
            <div className="flex flex-wrap items-center gap-2">
              <span className="text-[13px] font-bold text-ink-700">
                Bàn giao lớp cho{" "}
                {targets.find((member) => member.id === targetId)?.full_name ?? "giáo viên này"}?
              </span>
              <HvButton size="sm" onClick={confirm} disabled={reassign.isPending}>
                {reassign.isPending ? "Đang bàn giao…" : "Xác nhận bàn giao"}
              </HvButton>
              <HvButton
                size="sm"
                variant="ghost"
                onClick={() => setArming(false)}
                disabled={reassign.isPending}
              >
                Hủy
              </HvButton>
            </div>
          ) : (
            <div>
              <HvButton
                variant="secondary"
                size="sm"
                onClick={() => setArming(true)}
                disabled={!targetId || reassign.isPending}
              >
                Bàn giao lớp
              </HvButton>
            </div>
          )}
        </div>
      )}
    </HvCard>
  );
}
