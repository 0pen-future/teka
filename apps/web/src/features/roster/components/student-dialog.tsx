import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect } from "react";
import { useForm } from "react-hook-form";

import { HvButton, HvModal } from "@/components/hv";
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";

import { ContactPicker } from "./contact-picker";
import { useCreateStudent, useUpdateStudent } from "../hooks/use-students";
import { studentInputSchema, type Student, type StudentInput } from "../schemas/roster-schemas";

export interface StudentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** Present in edit mode; absent when creating a new student. */
  student?: Student;
  /** Pre-fills the owning contact, e.g. when adding a second child from a contact's detail page. */
  defaultContactId?: string;
  onSuccess?: (student: Student) => void;
}

function toDefaultValues(student?: Student, defaultContactId?: string): StudentInput {
  return {
    full_name: student?.full_name ?? "",
    contact_id: student?.contact_id ?? defaultContactId ?? "",
    display_note: student?.display_note ?? "",
  };
}

/**
 * Student create/edit. This form intentionally has exactly three inputs —
 * full name, an optional disambiguating note, and the owning contact. PRD
 * R1's closed field list forbids age, grade, birth date, address, school, or
 * photo on a student record: collecting them would be a personal-data
 * liability under Nghị định 13/2023 (PRD Q2). Do not add a field here
 * without first getting that legal question re-opened; the accompanying test
 * asserts exactly three textbox/combobox roles to catch an accidental
 * extension.
 */
export function StudentDialog({
  open,
  onOpenChange,
  student,
  defaultContactId,
  onSuccess,
}: StudentDialogProps) {
  const isEdit = Boolean(student);
  const form = useForm<StudentInput>({
    resolver: zodResolver(studentInputSchema),
    defaultValues: toDefaultValues(student, defaultContactId),
  });
  const createMutation = useCreateStudent();
  const updateMutation = useUpdateStudent(student?.id ?? "");
  const mutation = isEdit ? updateMutation : createMutation;
  const handleApiError = useApiFormErrors(form);

  useEffect(() => {
    if (open) {
      form.reset(toDefaultValues(student, defaultContactId));
    }
    // form is stable from react-hook-form and not a meaningful dependency here.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, student, defaultContactId]);

  const onSubmit = form.handleSubmit((values) => {
    mutation.mutate(values, {
      onSuccess: (result) => {
        onOpenChange(false);
        onSuccess?.(result);
      },
      onError: handleApiError,
    });
  });

  const { errors } = form.formState;

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title={isEdit ? "Sửa học sinh" : "Thêm học sinh"}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="submit" form="student-dialog-form" disabled={mutation.isPending}>
            {mutation.isPending ? "Đang lưu…" : "Lưu"}
          </HvButton>
        </>
      }
    >
      <form id="student-dialog-form" onSubmit={(event) => void onSubmit(event)} noValidate>
        <FieldGroup>
          <Field data-invalid={Boolean(errors.full_name)}>
            <FieldLabel htmlFor="student-full-name">Họ và tên</FieldLabel>
            <Input
              id="student-full-name"
              autoComplete="name"
              aria-invalid={Boolean(errors.full_name)}
              {...form.register("full_name")}
            />
            <FieldError errors={[errors.full_name]} />
          </Field>
          <Field data-invalid={Boolean(errors.display_note)}>
            <FieldLabel htmlFor="student-display-note">Ghi chú phân biệt</FieldLabel>
            <Input
              id="student-display-note"
              aria-invalid={Boolean(errors.display_note)}
              {...form.register("display_note")}
            />
            <FieldDescription>
              Dùng khi hai anh em cùng lớp trùng tên, ví dụ: An lớp 9A
            </FieldDescription>
            <FieldError errors={[errors.display_note]} />
          </Field>
          <Field data-invalid={Boolean(errors.contact_id)}>
            <FieldLabel htmlFor="student-contact">Người liên hệ</FieldLabel>
            <ContactPicker
              id="student-contact"
              aria-invalid={Boolean(errors.contact_id)}
              value={form.watch("contact_id")}
              onChange={(contactId) =>
                form.setValue("contact_id", contactId, { shouldValidate: true, shouldDirty: true })
              }
            />
            <FieldError errors={[errors.contact_id]} />
          </Field>
          <FieldError errors={[errors.root]} />
        </FieldGroup>
      </form>
    </HvModal>
  );
}
