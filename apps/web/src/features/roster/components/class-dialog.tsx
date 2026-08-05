import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvBadge, HvButton, HvModal } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { MoneyInput } from "./money-input";
import { WeekdayChips } from "./weekday-chips";
import { useArchiveClass, useCreateClass, useUpdateClass } from "../hooks/use-classes";
import {
  classDialogInputSchema,
  classUpdateInputSchema,
  toClassCreateInput,
  type Class,
  type ClassDialogInput,
  type ClassUpdateInput,
} from "../schemas/roster-schemas";

export interface ClassDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Present in edit mode; absent when creating a new class. */
  klass?: Class;
  onSuccess?: (klass: Class) => void;
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

function toEditDefaults(klass: Class): ClassUpdateInput {
  return {
    name: klass.name,
    start_date: klass.start_date,
    end_date: klass.end_date ?? "",
    default_unit_price: klass.default_unit_price,
  };
}

/**
 * `ClassDialog` (prototype `modalClass`). Create mode gathers the class plus
 * its one initial weekly schedule in a single step — `POST /classes`
 * requires at least one schedule atomically. Edit mode only touches
 * name/dates/price; additional schedule rows are managed later on
 * `ClassDetailPage`'s `ScheduleEditor`, and status changes through the
 * separate archive action shown here as a badge plus button, not a form
 * field.
 */
export function ClassDialog({ open, onOpenChange, klass, onSuccess }: ClassDialogProps) {
  const isEdit = Boolean(klass);
  const createForm = useForm<ClassDialogInput>({
    resolver: zodResolver(classDialogInputSchema),
    defaultValues: toCreateDefaults(),
  });
  const editForm = useForm<ClassUpdateInput>({
    resolver: zodResolver(classUpdateInputSchema),
    defaultValues: klass ? toEditDefaults(klass) : undefined,
  });
  const createMutation = useCreateClass();
  const updateMutation = useUpdateClass(klass?.id ?? "");
  const archiveMutation = useArchiveClass();
  const handleCreateApiError = useApiFormErrors(createForm);
  const handleEditApiError = useApiFormErrors(editForm);

  useEffect(() => {
    if (!open) {
      return;
    }
    if (klass) {
      editForm.reset(toEditDefaults(klass));
    } else {
      createForm.reset(toCreateDefaults());
    }
    // forms are stable from react-hook-form and not meaningful dependencies here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, klass]);

  const onCreateSubmit = createForm.handleSubmit((values) => {
    createMutation.mutate(toClassCreateInput(values), {
      onSuccess: (result) => {
        onOpenChange(false);
        onSuccess?.(result);
      },
      onError: handleCreateApiError,
    });
  });

  const onEditSubmit = editForm.handleSubmit((values) => {
    updateMutation.mutate(values, {
      onSuccess: (result) => {
        onOpenChange(false);
        onSuccess?.(result);
      },
      onError: handleEditApiError,
    });
  });

  if (isEdit && klass) {
    const { errors } = editForm.formState;
    return (
      <HvModal
        open={open}
        onOpenChange={onOpenChange}
        title="Sửa lớp"
        footer={
          <>
            <HvButton
              type="button"
              variant="danger"
              disabled={archiveMutation.isPending || klass.status === "archived"}
              onClick={() => archiveMutation.mutate(klass.id)}
            >
              Lưu trữ lớp
            </HvButton>
            <HvButton
              type="submit"
              form="class-dialog-edit-form"
              disabled={updateMutation.isPending}
            >
              {updateMutation.isPending ? "Đang lưu…" : "Lưu"}
            </HvButton>
          </>
        }
      >
        <div className="mb-4">
          <HvBadge variant={klass.status === "active" ? "success" : "neutral"}>
            {klass.status === "active" ? "Đang hoạt động" : "Đã lưu trữ"}
          </HvBadge>
        </div>
        <form id="class-dialog-edit-form" onSubmit={(event) => void onEditSubmit(event)} noValidate>
          <FieldGroup>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="class-name">Tên lớp</FieldLabel>
              <Input
                id="class-name"
                aria-invalid={Boolean(errors.name)}
                {...editForm.register("name")}
              />
              <FieldError errors={[errors.name]} />
            </Field>
            <Field data-invalid={Boolean(errors.start_date)}>
              <FieldLabel htmlFor="class-start-date">Khai giảng</FieldLabel>
              <Input
                id="class-start-date"
                type="date"
                aria-invalid={Boolean(errors.start_date)}
                {...editForm.register("start_date")}
              />
              <FieldError errors={[errors.start_date]} />
            </Field>
            <Field data-invalid={Boolean(errors.end_date)}>
              <FieldLabel htmlFor="class-end-date">Ngày kết thúc</FieldLabel>
              <Input
                id="class-end-date"
                type="date"
                aria-invalid={Boolean(errors.end_date)}
                {...editForm.register("end_date")}
              />
              <FieldError errors={[errors.end_date]} />
            </Field>
            <Field data-invalid={Boolean(errors.default_unit_price)}>
              <FieldLabel htmlFor="class-unit-price">Đơn giá / buổi (đ)</FieldLabel>
              <MoneyInput
                id="class-unit-price"
                aria-invalid={Boolean(errors.default_unit_price)}
                value={editForm.watch("default_unit_price")}
                onChange={(value) =>
                  editForm.setValue("default_unit_price", value, {
                    shouldValidate: true,
                    shouldDirty: true,
                  })
                }
              />
              <FieldError errors={[errors.default_unit_price]} />
            </Field>
            <FieldError errors={[errors.root]} />
          </FieldGroup>
        </form>
      </HvModal>
    );
  }

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
