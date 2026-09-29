import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useState } from "react";
import { useForm, type UseFormReturn } from "react-hook-form";

import { HvButton, HvModal, HvSelect } from "@/components/hv";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useTemplatesList, useVersions, versionStatusLabel } from "@/features/library";
import { useCenterContext } from "@/features/teaching";
import { textareaClassName } from "@/lib/forms/textarea-class";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn } from "@/lib/utils";

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
import { CourseDeleteConfirm } from "./course-delete-confirm";

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
  template_id: "",
  default_template_version_id: "",
};

const FORM_ID = "course-dialog-form";

/** Template ids are UUIDs, so this never collides with a real option. */
const NO_TEMPLATE = "none";
/** Stands for a stored version whose template has been deleted. */
const ORPHAN_TEMPLATE = "orphan";

const hintClassName = "text-[12px] text-ink-400";

export type CourseDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
} & (
  | { mode: "create"; onCreated: (course: Course) => void }
  | {
      mode: "edit";
      course: Course;
      onSaved?: (course: Course) => void;
      onDeleted: (course: Course) => void;
    }
);

/**
 * The "Tạo / Sửa khóa học" form: the design's fields first (code, name,
 * session length, price, program template and its default version),
 * then the course's remaining own fields. The code is fixed once created.
 * Stages are assigned from the learning paths screen, since one course can
 * sit in several stages. Editing also offers "Xoá", whose confirmation
 * stacks on top of the form so cancelling it returns to the edit.
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
  const [deleting, setDeleting] = useState(false);

  useEffect(() => {
    if (open) {
      setDeleting(false);
      form.reset(props.mode === "edit" ? toCourseForm(props.course) : EMPTY_FORM);
    }
    // The form instance is stable; only the opening (and the row it opens on) matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open]);

  const onSubmit = form.handleSubmit((values) => {
    if (props.mode === "edit") {
      // The code is read-only here; resend the stored one whatever the input holds.
      updateMutation.mutate(
        { ...toCourseInput(values), code: props.course.code },
        {
          onSuccess: (course) => {
            onOpenChange(false);
            props.onSaved?.(course);
          },
          onError: handleApiError,
        },
      );
      return;
    }
    createMutation.mutate(toCourseInput(values), {
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
      description="Mã không đổi sau khi tạo. Lịch dạy, giáo viên, học phí thuộc lớp — không thuộc khóa."
      footer={
        <>
          {editing ? (
            <HvButton
              type="button"
              variant="ghost"
              className="mr-auto text-coral-600"
              disabled={pending}
              onClick={() => setDeleting(true)}
            >
              Xoá
            </HvButton>
          ) : null}
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form={FORM_ID} disabled={pending}>
            {pending ? "Đang lưu…" : editing ? "Lưu thay đổi" : "Tạo mới"}
          </HvButton>
        </>
      }
    >
      <form id={FORM_ID} onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field data-invalid={Boolean(errors.code)}>
              <FieldLabel htmlFor="course-code">Mã khóa học</FieldLabel>
              <Input
                id="course-code"
                placeholder="VD: TOAN-6"
                autoCapitalize="characters"
                readOnly={editing}
                aria-describedby="course-code-hint"
                aria-invalid={Boolean(errors.code)}
                className={cn(editing && "bg-cream-100 text-ink-500")}
                {...form.register("code")}
              />
              <FieldDescription id="course-code-hint" className={hintClassName}>
                {editing ? "Không đổi được mã sau khi tạo" : "Viết hoa, không dấu"}
              </FieldDescription>
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
            <Field data-invalid={Boolean(errors.duration_min)}>
              <FieldLabel htmlFor="course-duration">Thời lượng buổi (phút)</FieldLabel>
              <Input
                id="course-duration"
                inputMode="numeric"
                placeholder="VD: 90"
                aria-invalid={Boolean(errors.duration_min)}
                {...form.register("duration_min")}
              />
              <FieldError errors={[errors.duration_min]} />
            </Field>
            <Field data-invalid={Boolean(errors.default_unit_price)}>
              <FieldLabel htmlFor="course-unit-price">Giá / buổi (đ)</FieldLabel>
              <Input
                id="course-unit-price"
                inputMode="numeric"
                aria-invalid={Boolean(errors.default_unit_price)}
                {...form.register("default_unit_price")}
              />
              <FieldError errors={[errors.default_unit_price]} />
            </Field>
            <TemplateFields
              form={form}
              open={open}
              course={props.mode === "edit" ? props.course : null}
            />
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
            <Field data-invalid={Boolean(errors.description)} className="sm:col-span-2">
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
          </div>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
      {props.mode === "edit" ? (
        <CourseDeleteConfirm
          course={props.course}
          open={deleting}
          onOpenChange={setDeleting}
          onDeleted={(course) => {
            onOpenChange(false);
            props.onDeleted(course);
          }}
        />
      ) : null}
    </HvModal>
  );
}

interface TemplateFieldsProps {
  form: UseFormReturn<CourseFormInput, unknown, CourseFormValues>;
  open: boolean;
  /** The stored course when editing, null when creating. */
  course: Course | null;
}

/**
 * "Chương trình mẫu" and "Phiên bản mặc định". Picking a template lands on
 * its newest published version; only published versions are offered, since
 * the API refuses a draft or archived one as a new choice. The stored
 * version stays listed even if archived since, because resending it is not
 * a new choice. A member who cannot read the library sees the binding
 * read-only and saves it unchanged.
 */
function TemplateFields({ form, open, course }: TemplateFieldsProps) {
  const { has } = useCenterContext();
  const canPick = has("library.read");
  const templateId = form.watch("template_id");
  const versionId = form.watch("default_template_version_id");
  const stored = course?.default_template ?? null;
  const storedVersionId = course?.default_template_version_id ?? null;

  const templates = useTemplatesList({ per_page: 100, sort: "name" }, open && canPick);
  const versions = useVersions(canPick && templateId ? templateId : undefined);
  const choices = (versions.data ?? [])
    .filter((version) => version.status === "published" || version.id === storedVersionId)
    .sort((a, b) => b.version_no - a.version_no);
  const newestPublished = choices.find((version) => version.status === "published");

  useEffect(() => {
    if (!templateId || versionId !== "" || !newestPublished) return;
    form.setValue("default_template_version_id", newestPublished.id, {
      shouldDirty: true,
      shouldValidate: true,
    });
  }, [form, templateId, versionId, newestPublished]);

  function pickTemplate(next: string) {
    if (next === ORPHAN_TEMPLATE) return;
    const id = next === NO_TEMPLATE ? "" : next;
    // Picking "— Chưa gắn —" over an orphaned binding still clears its version.
    if (id === templateId && (id !== "" || versionId === "")) return;
    form.setValue("template_id", id, { shouldDirty: true });
    // Returning to the stored template restores its version rather than the newest one.
    const version = id === stored?.template_id ? (storedVersionId ?? "") : "";
    form.setValue("default_template_version_id", version, {
      shouldDirty: true,
      shouldValidate: true,
    });
  }

  const versionError = form.formState.errors.default_template_version_id;
  const orphaned = storedVersionId !== null && stored === null && versionId === storedVersionId;
  const templateValue = templateId || (orphaned ? ORPHAN_TEMPLATE : NO_TEMPLATE);

  if (!canPick) {
    return (
      <>
        <Field>
          <FieldLabel htmlFor="course-dialog-template">Chương trình mẫu</FieldLabel>
          <Input
            id="course-dialog-template"
            readOnly
            value={stored?.name ?? (storedVersionId ? "Chương trình đã bị xoá" : "— Chưa gắn —")}
            aria-describedby="course-dialog-template-hint"
            className="bg-cream-100 text-ink-500"
          />
          <FieldDescription id="course-dialog-template-hint" className={hintClassName}>
            Cần quyền xem Kho học liệu để chọn chương trình mẫu.
          </FieldDescription>
        </Field>
        <Field>
          <FieldLabel htmlFor="course-dialog-version">Phiên bản mặc định</FieldLabel>
          <Input
            id="course-dialog-version"
            readOnly
            value={stored ? `v${stored.version_no} — ${versionStatusLabel[stored.status]}` : "—"}
            className="bg-cream-100 text-ink-500"
          />
        </Field>
      </>
    );
  }

  const templateOptions = (templates.data?.items ?? []).map((template) => ({
    value: template.id,
    label: template.name,
    meta: template.code,
  }));
  // Past the first page of templates the stored one may be missing from the list.
  if (stored && !templateOptions.some((option) => option.value === stored.template_id)) {
    templateOptions.push({ value: stored.template_id, label: stored.name, meta: stored.code });
  }
  const leadOptions = [{ value: NO_TEMPLATE, label: "— Chưa gắn —" }];
  if (orphaned) leadOptions.push({ value: ORPHAN_TEMPLATE, label: "Chương trình đã bị xoá" });

  const versionPlaceholder = !templateId
    ? "—"
    : versions.isPending
      ? "Đang tải phiên bản…"
      : versions.isError
        ? "Không tải được phiên bản"
        : "Chưa có phiên bản đã phát hành";

  return (
    <>
      <Field>
        <FieldLabel htmlFor="course-dialog-template">Chương trình mẫu</FieldLabel>
        <HvSelect
          id="course-dialog-template"
          aria-label="Chương trình mẫu"
          value={templateValue}
          onValueChange={pickTemplate}
          options={[...leadOptions, ...templateOptions]}
          sheetTitle="Chương trình mẫu"
          searchNoun="chương trình"
        />
        {orphaned ? (
          <FieldDescription className="text-[12px] font-bold text-coral-600">
            Chương trình mẫu đã gắn không còn trong kho; chọn chương trình khác hoặc “— Chưa gắn —”.
          </FieldDescription>
        ) : null}
      </Field>
      <Field data-invalid={Boolean(versionError)}>
        <FieldLabel htmlFor="course-dialog-version">Phiên bản mặc định</FieldLabel>
        <HvSelect
          id="course-dialog-version"
          aria-label="Phiên bản mặc định"
          aria-invalid={Boolean(versionError)}
          value={templateId ? versionId : ""}
          disabled={!templateId || choices.length === 0}
          onValueChange={(next) =>
            form.setValue("default_template_version_id", next, {
              shouldDirty: true,
              shouldValidate: true,
            })
          }
          options={choices.map((version) => ({
            value: version.id,
            label: `v${version.version_no} — ${versionStatusLabel[version.status]}`,
          }))}
          placeholder={versionPlaceholder}
          sheetTitle="Phiên bản mặc định"
          searchThreshold={Infinity}
        />
        {course ? (
          <FieldDescription className={hintClassName}>
            Lớp đang chạy giữ phiên bản cũ
          </FieldDescription>
        ) : null}
        <FieldError errors={[versionError]} />
      </Field>
    </>
  );
}
