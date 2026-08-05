import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { MoneyInput } from "./money-input";
import { WeekdayChips } from "./weekday-chips";
import { useCreateClass } from "../hooks/use-classes";
import {
  classDialogInputSchema,
  toClassCreateInput,
  type ClassDialogInput,
} from "../schemas/roster-schemas";

export interface ClassDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

const today = () => new Date().toISOString().slice(0, 10);

function toCreateDefaults(): ClassDialogInput {
  return {
    name: "",
    start_date: today(),
    end_date: "",
    default_unit_price: 0,
    weekday: 1,
    start_time: "",
    duration_min: 90,
  };
}

/**
 * `ClassDialog` (prototype `modalClass`) — create-only. It gathers the class
 * plus its one initial weekly schedule in a single step because
 * `POST /classes` requires at least one schedule atomically. Later changes
 * to name/timetable/price go through the "Cài đặt lớp" screen
 * (`ClassSettingsPage`).
 */
export function ClassDialog({ open, onOpenChange }: ClassDialogProps) {
  const createForm = useForm<ClassDialogInput>({
    resolver: zodResolver(classDialogInputSchema),
    defaultValues: toCreateDefaults(),
  });
  const createMutation = useCreateClass();
  const handleCreateApiError = useApiFormErrors(createForm);

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
          <Field data-invalid={Boolean(errors.weekday)}>
            <FieldLabel htmlFor="class-weekday">Lịch trong tuần</FieldLabel>
            <WeekdayChips
              id="class-weekday"
              value={createForm.watch("weekday")}
              onChange={(weekday) =>
                createForm.setValue("weekday", weekday, { shouldValidate: true, shouldDirty: true })
              }
            />
            <FieldError errors={[errors.weekday]} />
          </Field>
          <div className="grid gap-3 sm:grid-cols-3">
            <Field data-invalid={Boolean(errors.start_time)}>
              <FieldLabel htmlFor="class-start-time">Giờ học</FieldLabel>
              <Input
                id="class-start-time"
                type="time"
                aria-invalid={Boolean(errors.start_time)}
                {...createForm.register("start_time")}
              />
              <FieldError errors={[errors.start_time]} />
            </Field>
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
