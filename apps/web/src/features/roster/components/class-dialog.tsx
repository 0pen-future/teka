import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useRef } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal, HvSelect, HvStateBlock, hvToast } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { ApiError } from "@/lib/api/errors";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { MoneyInput } from "./money-input";
import { ScheduleSlotsEditor } from "./schedule-slots-editor";
import { useClass, useCreateClass, useCourseOptions } from "../hooks/use-classes";
import { useSaveClassSettings } from "../hooks/use-save-class-settings";
import { canWriteClass } from "../lib/class-permissions";
import { deriveScheduleSlots, emptySlot, weeklySessionCount } from "../lib/schedule-diff";
import {
  classDialogInputSchema,
  classSettingsInputSchema,
  toClassCreateInput,
  type Class,
  type ClassDialogInput,
  type ClassSettingsInput,
} from "../schemas/roster-schemas";

export type ClassDialogProps =
  | { mode?: "create"; open: boolean; onOpenChange: (open: boolean) => void }
  | { mode: "edit"; classId: string; open: boolean; onOpenChange: (open: boolean) => void };

const today = () => new Date().toISOString().slice(0, 10);

function toCreateDefaults(): ClassDialogInput {
  return {
    name: "",
    start_date: today(),
    end_date: "",
    default_unit_price: 0,
    slots: [emptySlot()],
    duration_min: 90,
    course_id: "",
  };
}

const NO_COURSE_OPTION = { value: "", label: "Không gắn khóa học" };

/**
 * `ClassDialog` (prototype `modalClass`) supports creation and in-place editing.
 * Create posts one atomic class; edit saves name/price and timetable separately.
 */
export function ClassDialog(props: ClassDialogProps) {
  if (props.mode === "edit") {
    return <EditClassDialog {...props} />;
  }
  return <CreateClassForm open={props.open} onOpenChange={props.onOpenChange} />;
}

function CreateClassForm({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const createForm = useForm<ClassDialogInput>({
    resolver: zodResolver(classDialogInputSchema),
    defaultValues: toCreateDefaults(),
  });
  const createMutation = useCreateClass();
  const handleCreateApiError = useApiFormErrors(createForm);
  const courseOptions = useCourseOptions(open);

  useEffect(() => {
    if (open) {
      createForm.reset(toCreateDefaults());
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onCreateSubmit = createForm.handleSubmit((values) => {
    createMutation.mutate(toClassCreateInput(values), {
      onSuccess: () => {
        onOpenChange(false);
      },
      onError: handleCreateApiError,
    });
  });

  const { errors } = createForm.formState;
  const slots = createForm.watch("slots");
  const courseId = createForm.watch("course_id");
  const courses = courseOptions.data ?? [];
  /** The price the dialog itself last put in the field; null until a course prefilled one. */
  const lastPrefill = useRef<number | null>(null);

  function handleCoursePick(nextId: string) {
    createForm.setValue("course_id", nextId, { shouldDirty: true, shouldValidate: true });
    const course = courses.find((option) => option.id === nextId);
    // Prefill the price from the course while the field still holds nothing
    // but a prefill (blank, or the previous course's price), so a price the
    // teacher typed on purpose survives a course change.
    const current = createForm.getValues("default_unit_price");
    if (course && (current === 0 || current === lastPrefill.current)) {
      lastPrefill.current = course.default_unit_price;
      createForm.setValue("default_unit_price", course.default_unit_price, {
        shouldDirty: true,
        shouldValidate: true,
      });
    }
  }
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Tạo lớp mới"
      description="Ngày khai giảng · lịch cố định trong tuần · đơn giá mỗi buổi."
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton
            type="submit"
            form="class-dialog-create-form"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? "Đang lưu…" : "Tạo lớp"}
          </HvButton>
        </>
      }
    >
      <form
        id="class-dialog-create-form"
        onSubmit={(event) => void onCreateSubmit(event)}
        noValidate
      >
        <FieldGroup>
          <Field data-invalid={Boolean(errors.name)}>
            <FieldLabel htmlFor="class-name">Tên lớp</FieldLabel>
            <Input
              id="class-name"
              placeholder="VD: Toán 9C"
              aria-invalid={Boolean(errors.name)}
              {...createForm.register("name")}
            />
            <FieldError errors={[errors.name]} />
          </Field>
          <Field data-invalid={Boolean(errors.course_id)}>
            <FieldLabel htmlFor="class-course">Khóa học</FieldLabel>
            <HvSelect
              id="class-course"
              aria-label="Khóa học"
              value={courseId}
              onValueChange={handleCoursePick}
              options={[
                NO_COURSE_OPTION,
                ...courses.map((course) => ({
                  value: course.id,
                  label: course.name,
                  meta: course.code,
                })),
              ]}
              sheetTitle="Chọn khóa học"
              placeholder="Không gắn khóa học"
              searchNoun="khóa học"
              aria-invalid={Boolean(errors.course_id)}
              disabled={courseOptions.isPending}
            />
            <FieldError errors={[errors.course_id]} />
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
            <ScheduleSlotsEditor
              idPrefix="class-dialog"
              value={slots}
              onChange={(next) =>
                createForm.setValue("slots", next, { shouldValidate: true, shouldDirty: true })
              }
              slotErrors={slots.map((_, index) => ({
                time: errors.slots?.[index]?.start_time?.message,
                days: errors.slots?.[index]?.days?.message,
              }))}
            />
            <FieldError errors={[errors.slots?.root]} />
          </Field>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.default_unit_price)}>
              <FieldLabel htmlFor="class-unit-price">Đơn giá / buổi (đ)</FieldLabel>
              <MoneyInput
                id="class-unit-price"
                aria-invalid={Boolean(errors.default_unit_price)}
                value={createForm.watch("default_unit_price")}
                onChange={(value) =>
                  createForm.setValue("default_unit_price", value, {
                    shouldValidate: true,
                    shouldDirty: true,
                  })
                }
              />
              <FieldError errors={[errors.default_unit_price]} />
            </Field>
            <Field data-invalid={Boolean(errors.start_date)}>
              <FieldLabel htmlFor="class-start-date">Khai giảng</FieldLabel>
              <Input
                id="class-start-date"
                type="date"
                aria-invalid={Boolean(errors.start_date)}
                {...createForm.register("start_date")}
              />
              <FieldError errors={[errors.start_date]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.duration_min)}>
              <FieldLabel htmlFor="class-duration">Thời lượng (phút)</FieldLabel>
              <Input
                id="class-duration"
                type="number"
                min={1}
                aria-invalid={Boolean(errors.duration_min)}
                {...createForm.register("duration_min", { valueAsNumber: true })}
              />
              <FieldError errors={[errors.duration_min]} />
            </Field>
            <Field data-invalid={Boolean(errors.end_date)}>
              <FieldLabel htmlFor="class-end-date">Ngày kết thúc</FieldLabel>
              <Input
                id="class-end-date"
                type="date"
                aria-invalid={Boolean(errors.end_date)}
                {...createForm.register("end_date")}
              />
              <FieldError errors={[errors.end_date]} />
            </Field>
          </div>
          <p className="text-[12px] text-ink-400">
            Đơn giá lưu ở từng lượt ghi danh (mặc định kế thừa đơn giá lớp).
          </p>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}

function toEditDefaults(klass: Class): ClassSettingsInput {
  const slots = deriveScheduleSlots(klass.schedules, today());
  return {
    name: klass.name,
    slots: slots.length ? slots : [emptySlot()],
    default_unit_price: klass.default_unit_price,
  };
}

function EditClassDialog({
  classId,
  open,
  onOpenChange,
}: Extract<ClassDialogProps, { mode: "edit" }>) {
  const { data: klass, isPending, isError, error } = useClass(open ? classId : undefined);
  const { isOwner } = useCenterContext();
  const form = useForm<ClassSettingsInput>({
    resolver: zodResolver(classSettingsInputSchema),
    defaultValues: { name: "", slots: [emptySlot()], default_unit_price: 0 },
  });
  const handleApiError = useApiFormErrors(form);
  const { save, isPending: saving } = useSaveClassSettings(klass);
  // A detail invalidation during a multi-request save must not erase user edits or its error.
  const resetForClassId = useRef<string | null>(null);
  useEffect(() => {
    if (!open) {
      resetForClassId.current = null;
    } else if (klass && resetForClassId.current !== klass.id) {
      resetForClassId.current = klass.id;
      form.reset(toEditDefaults(klass));
    }
    // form is stable from react-hook-form.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, klass]);

  const canWrite = klass ? canWriteClass(isOwner, klass) : false;
  const slots = form.watch("slots");
  const rateChanged = klass && form.watch("default_unit_price") !== klass.default_unit_price;
  const { errors } = form.formState;
  const submit = form.handleSubmit(async (values) => {
    if (!canWrite) return;
    const result = await save(values, today());
    if (result.ok) {
      hvToast(`Đã lưu ${values.name.trim()} — áp dụng từ buổi kế tiếp`);
      onOpenChange(false);
    } else if (result.partial) {
      form.setError("root", {
        message: "Chỉ lưu được một phần thay đổi — kiểm tra lại lịch của lớp rồi lưu lại lần nữa.",
      });
    } else {
      handleApiError(result.error);
    }
  });

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Sửa lớp học"
      description="Thay đổi áp dụng từ buổi kế tiếp — các kỳ đã chốt không đổi."
      stickyFooter
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form="class-dialog-edit-form" disabled={saving || !canWrite}>
            {saving ? "Đang lưu…" : "Lưu thay đổi"}
          </HvButton>
        </>
      }
    >
      {isPending ? (
        <HvStateBlock state="loading" title="Đang tải lớp" />
      ) : isError || !klass ? (
        <HvStateBlock
          state="error"
          title={
            error instanceof ApiError && error.status === 404
              ? "Không tìm thấy lớp"
              : "Không tải được lớp"
          }
          action={
            <HvButton variant="ghost" size="sm" onClick={() => onOpenChange(false)}>
              Đóng
            </HvButton>
          }
        />
      ) : (
        <form id="class-dialog-edit-form" onSubmit={(event) => void submit(event)} noValidate>
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="class-edit-name">Tên lớp</FieldLabel>
              <Input
                id="class-edit-name"
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
                idPrefix="class-edit"
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
              <FieldLabel htmlFor="class-edit-unit-price">Đơn giá / buổi (đ)</FieldLabel>
              <MoneyInput
                id="class-edit-unit-price"
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
        </form>
      )}
    </HvModal>
  );
}
