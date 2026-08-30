import { useEffect, useState, type FormEvent } from "react";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { formatMoney } from "@/lib/utils";

import { useCreateEnrollment, useEnrollableStudents } from "../hooks/use-enrollments";
import { enrollmentCreateInputSchema } from "../schemas/roster-schemas";
import type { Class, EnrollableStudent, Enrollment } from "../schemas/roster-schemas";

interface EnrollExistingStudentDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  /** The known class; the student is found by name through the picker. */
  klass: Class;
  onSuccess?: (enrollment: Enrollment) => void;
}

const today = () => new Date().toISOString().slice(0, 10);

/**
 * `POST /enrollments` for one known class picking an existing student —
 * `EnrollStudentDialog`'s mirror image, reached from the class tab on
 * `StudentsPage`. The autocomplete goes through the class-scoped
 * enrollable-students endpoint, which returns names only, so a teacher who
 * may not see contact data can still enroll a student the center already
 * has — instead of creating a duplicate through the add-student wizard.
 */
export function EnrollExistingStudentDialog(props: EnrollExistingStudentDialogProps) {
  const { open, onOpenChange, klass, onSuccess } = props;
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [picked, setPicked] = useState<EnrollableStudent | undefined>(undefined);
  const [startedOn, setStartedOn] = useState(today());
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), 300);
    return () => clearTimeout(timer);
  }, [query]);

  /** Same reset-on-close pattern as `EnrollStudentDialog`. */
  function handleOpenChange(next: boolean) {
    if (!next) {
      setQuery("");
      setDebouncedQuery("");
      setPicked(undefined);
      setStartedOn(today());
      setError(null);
    }
    onOpenChange(next);
  }

  const search = useEnrollableStudents(open ? klass.id : undefined, debouncedQuery);
  const students = search.data ?? [];
  const createMutation = useCreateEnrollment();

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    if (!picked) {
      setError("Chọn một học sinh");
      return;
    }
    const parsed = enrollmentCreateInputSchema.safeParse({
      student_id: picked.id,
      class_id: klass.id,
      started_on: startedOn,
    });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? "Dữ liệu không hợp lệ");
      return;
    }
    const studentName = picked.full_name;
    createMutation.mutate(parsed.data, {
      onSuccess: (enrollment) => {
        handleOpenChange(false);
        hvToast(
          `Đã ghi danh ${studentName} vào ${klass.name} — tính tiền từ buổi có mặt đầu tiên`,
          { variant: "success" },
        );
        onSuccess?.(enrollment);
      },
      onError: (err) => {
        setError(err instanceof Error ? err.message : "Không thể ghi danh");
      },
    });
  }

  return (
    <HvModal
      open={open}
      onOpenChange={handleOpenChange}
      title={`Ghi danh vào ${klass.name}`}
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton
            type="submit"
            form="enroll-existing-student-form"
            disabled={createMutation.isPending}
          >
            {createMutation.isPending ? "Đang lưu…" : "Ghi danh vào lớp"}
          </HvButton>
        </>
      }
    >
      <form id="enroll-existing-student-form" onSubmit={handleSubmit} noValidate>
        <FieldGroup>
          <Field>
            <FieldLabel htmlFor="enroll-existing-student">Học sinh</FieldLabel>
            {picked ? (
              <div className="flex items-center justify-between rounded-[var(--radius-md)] border border-line-200 bg-cream-100 px-3 py-2">
                <p className="font-display text-[14px] font-bold text-ink-900">
                  {picked.full_name}
                </p>
                <button
                  type="button"
                  onClick={() => setPicked(undefined)}
                  className="text-[13px] font-bold text-mint-600 hover:underline"
                >
                  Đổi
                </button>
              </div>
            ) : (
              <>
                <Input
                  id="enroll-existing-student"
                  role="combobox"
                  aria-expanded="true"
                  aria-controls="enroll-existing-student-listbox"
                  placeholder="Tìm theo tên học sinh"
                  value={query}
                  onChange={(event) => setQuery(event.target.value)}
                />
                <div
                  id="enroll-existing-student-listbox"
                  role="listbox"
                  aria-label="Học sinh"
                  className="mt-2 max-h-48 overflow-y-auto rounded-[var(--radius-md)] border border-line-200"
                >
                  {debouncedQuery.trim().length < 2 ? (
                    <p className="p-3 text-[13px] text-ink-400">
                      Nhập ít nhất 2 ký tự để tìm theo tên.
                    </p>
                  ) : null}
                  {search.isFetching ? (
                    <p className="p-3 text-[13px] text-ink-400">Đang tìm…</p>
                  ) : null}
                  {debouncedQuery.trim().length >= 2 &&
                  !search.isFetching &&
                  students.length === 0 ? (
                    <p className="p-3 text-[13px] text-ink-400">Không tìm thấy học sinh.</p>
                  ) : null}
                  {students.map((student) => (
                    <button
                      key={student.id}
                      type="button"
                      role="option"
                      aria-selected={false}
                      onClick={() => setPicked(student)}
                      className="w-full px-3 py-2 text-left text-[14px] font-bold text-ink-900 hover:bg-cream-100"
                    >
                      {student.full_name}
                    </button>
                  ))}
                </div>
              </>
            )}
          </Field>
          <Field>
            <FieldLabel htmlFor="enroll-existing-started-on">Ngày bắt đầu</FieldLabel>
            <Input
              id="enroll-existing-started-on"
              type="date"
              value={startedOn}
              onChange={(event) => setStartedOn(event.target.value)}
            />
          </Field>
          <Field>
            <FieldLabel htmlFor="enroll-existing-unit-price">Đơn giá / buổi</FieldLabel>
            <div
              id="enroll-existing-unit-price"
              className="rounded-[14px] border-2 border-dashed border-line-200 bg-cream-50 px-3.5 py-2.5 font-display text-[15px] font-bold text-ink-700"
            >
              {formatMoney(klass.default_unit_price)}/buổi
            </div>
          </Field>
          <p className="text-[12.5px] text-ink-400">
            Đơn giá kế thừa từ lớp, lưu ở lượt ghi danh này. Chỉ tính tiền từ buổi có mặt đầu tiên —
            kỳ đã chốt không đổi.
          </p>
          {error ? <FieldError>{error}</FieldError> : null}
        </FieldGroup>
      </form>
    </HvModal>
  );
}
