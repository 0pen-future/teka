import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal, HvSelect } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { textareaClassName } from "@/lib/forms/textarea-class";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { useCreateCourse, useUpdateCourse } from "../hooks/use-courses";
import { courseStatusLabel } from "../lib/course-labels";
import {
  courseFormSchema,
  toCourseForm,
  toCourseInput,
  type Course,
  type CourseFormInput,
  type CourseFormValues,
  type CourseStatus,
} from "../schemas/courses-schemas";

const EMPTY_FORM: CourseFormInput = {
  code: "",
  name: "",
  subject: "",
  level: "",
  description: "",
  status: "draft",
  default_unit_price: "0",
  total_sessions: "",
  duration_min: "",
};

const FORM_ID = "course-dialog-form";

export type CourseDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (course: Course) => void }
  | { mode: "edit"; course: Course; onSaved?: (course: Course) => void }
);

/**
 * Create/edit form for a course's own fields. The default template is set
 * on the detail page's own tab, so an edit carries the stored version id
 * through untouched. A duplicate code comes back as CONFLICT and lands on
 * the code input rather than the form footer.
 */
export function CourseDialog(props: CourseDialogProps) {
  const { open, onOpenChange } = props;
  const editing = props.mode === "edit";
  const form = useForm<CourseFormInput, unknown, CourseFormValues>({
    resolver: zodResolver(courseFormSchema),
    defaultValues: editing ? toCourseForm(props.course) : EMPTY_FORM,
  });
  const createMutation = useCreateCourse();
  const updateMutation = useUpdateCourse(editing ? props.course.id : "");
  const handleApiError = useApiFormErrors(form, { conflictField: "code" });
  const pending = createMutation.isPending || updateMutation.isPending;

  useEffect(() => {
    if (open) {
      form.reset(props.mode === "edit" ? toCourseForm(props.course) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    if (props.mode === "edit") {
      updateMutation.mutate(toCourseInput(values, props.course.default_template_version_id), {
        onSuccess: (course) => {
          onOpenChange(false);
          props.onSaved?.(course);
        },
        onError: handleApiError,
      });
      return;
    }
    createMutation.mutate(toCourseInput(values, null), {
      onSuccess: (course) => {
        onOpenChange(false);
        props.onCreated(course);
      },
      onError: handleApiError,
    });
  });

  // An archived course is reopened through the same form; a new course
  // starts as draft or active only.
  const statuses: CourseStatus[] = editing ? ["draft", "active", "archived"] : ["draft", "active"];
  const status = form.watch("status");
  const { errors } = form.formState;
  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={editing ? "Sửa khóa học" : "Tạo khóa học"}
      description="Mã dùng để nhận diện khóa; đơn giá là giá mặc định cho lớp mở từ khóa này."
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={FORM_ID} disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu" : "Tạo"}
          </HvButton>
        </>
      }
    >
      <form id={FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-[160px_1fr]">
            <Field data-invalid={Boolean(errors.code)}>
              <FieldLabel htmlFor="course-code">Mã khóa học</FieldLabel>
              <Input
                id="course-code"
                placeholder="VD: TOAN-6"
                autoCapitalize="characters"
                aria-invalid={Boolean(errors.code)}
                {...form.register("code")}
              />
              <FieldError errors={[errors.code]} />
            </Field>
            <Field data-invalid={Boolean(errors.name)}>
              <FieldLabel htmlFor="course-name">Tên khóa học</FieldLabel>
              <Input
                id="course-name"
                placeholder="VD: Toán 6 nền tảng"
                aria-invalid={Boolean(errors.name)}
                {...form.register("name")}
              />
              <FieldError errors={[errors.name]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.subject)}>
              <FieldLabel htmlFor="course-subject">Môn học</FieldLabel>
              <Input
                id="course-subject"
                placeholder="VD: Toán"
                aria-invalid={Boolean(errors.subject)}
                {...form.register("subject")}
              />
              <FieldError errors={[errors.subject]} />
            </Field>
            <Field data-invalid={Boolean(errors.level)}>
              <FieldLabel htmlFor="course-level">Cấp / trình độ</FieldLabel>
              <Input
                id="course-level"
                placeholder="VD: Lớp 6"
                aria-invalid={Boolean(errors.level)}
                {...form.register("level")}
              />
              <FieldError errors={[errors.level]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.status)}>
              <FieldLabel htmlFor="course-status">Trạng thái</FieldLabel>
              <HvSelect
                id="course-status"
                aria-label="Trạng thái"
                value={status}
                onValueChange={(next) =>
                  form.setValue("status", next as CourseStatus, {
                    shouldDirty: true,
                    shouldValidate: true,
                  })
                }
                options={statuses.map((value) => ({ value, label: courseStatusLabel[value] }))}
                sheetTitle="Trạng thái khóa học"
                searchThreshold={Infinity}
              />
              <FieldError errors={[errors.status]} />
            </Field>
            <Field data-invalid={Boolean(errors.default_unit_price)}>
              <FieldLabel htmlFor="course-unit-price">Đơn giá / buổi (đ)</FieldLabel>
              <Input
                id="course-unit-price"
                inputMode="numeric"
                aria-invalid={Boolean(errors.default_unit_price)}
                {...form.register("default_unit_price")}
              />
              <FieldError errors={[errors.default_unit_price]} />
            </Field>
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.total_sessions)}>
              <FieldLabel htmlFor="course-total-sessions">Tổng số buổi</FieldLabel>
              <Input
                id="course-total-sessions"
                inputMode="numeric"
                placeholder="Bỏ trống nếu chưa rõ"
                aria-invalid={Boolean(errors.total_sessions)}
                {...form.register("total_sessions")}
              />
              <FieldError errors={[errors.total_sessions]} />
            </Field>
            <Field data-invalid={Boolean(errors.duration_min)}>
              <FieldLabel htmlFor="course-duration">Thời lượng mỗi buổi (phút)</FieldLabel>
              <Input
                id="course-duration"
                inputMode="numeric"
                placeholder="VD: 90"
                aria-invalid={Boolean(errors.duration_min)}
                {...form.register("duration_min")}
              />
              <FieldError errors={[errors.duration_min]} />
            </Field>
          </div>
          <Field data-invalid={Boolean(errors.description)}>
            <FieldLabel htmlFor="course-description">Mô tả</FieldLabel>
            <textarea
              id="course-description"
              rows={3}
              className={textareaClassName}
              aria-invalid={Boolean(errors.description)}
              {...form.register("description")}
            />
            <FieldError errors={[errors.description]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
