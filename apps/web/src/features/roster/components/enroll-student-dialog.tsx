import { useEffect, useState, type FormEvent, type ReactNode } from "react";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { formatMoney } from "@/lib/utils";

import { useClass, useClassesList } from "../hooks/use-classes";
import { useCreateEnrollment } from "../hooks/use-enrollments";
import { useStudent, useStudentsList } from "../hooks/use-students";
import { enrollmentCreateInputSchema } from "../schemas/roster-schemas";
import type { Class, Enrollment, Student } from "../schemas/roster-schemas";

type EnrollStudentDialogProps =
  | {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      /** Enrolling one known student into a class they pick — from `StudentDetailPage`. */
      mode: "student";
      studentId: string;
      onSuccess?: (enrollment: Enrollment) => void;
    }
  | {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      /** Enrolling a searched-for student into one known class — from `ClassDetailPage`. */
      mode: "class";
      classId: string;
      onSuccess?: (enrollment: Enrollment) => void;
    };

const today = () => new Date().toISOString().slice(0, 10);

function EntitySearchList<T extends { id: string }>({
  items,
  isFetching,
  selectedId,
  onSelect,
  renderRow,
  emptyLabel,
}: {
  items: T[];
  isFetching: boolean;
  selectedId: string;
  onSelect: (item: T) => void;
  renderRow: (item: T) => ReactNode;
  emptyLabel: string;
}) {
  return (
    <div
      role="listbox"
      className="mt-2 max-h-48 overflow-y-auto rounded-[var(--radius-md)] border border-line-200"
    >
      {isFetching ? <p className="p-3 text-[13px] text-ink-400">Đang tìm…</p> : null}
      {!isFetching && items.length === 0 ? (
        <p className="p-3 text-[13px] text-ink-400">{emptyLabel}</p>
      ) : null}
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="option"
          aria-selected={item.id === selectedId}
          onClick={() => onSelect(item)}
          className="flex w-full items-center justify-between px-3 py-2 text-left text-[14px] hover:bg-cream-100"
        >
          {renderRow(item)}
        </button>
      ))}
    </div>
  );
}

/**
 * `POST /enrollments`, wired for either invocation direction (fixed student
 * searching for a class, or fixed class searching for a student). Unit price
 * is never entered here — it is inherited from the class server-side (PRD
 * section 4's single V1 pricing model) and only ever shown read-only.
 */
export function EnrollStudentDialog(props: EnrollStudentDialogProps) {
  const { open, onOpenChange, onSuccess } = props;
  const [query, setQuery] = useState("");
  const [debouncedQuery, setDebouncedQuery] = useState("");
  const [startedOn, setStartedOn] = useState(today());
  const [pickedClass, setPickedClass] = useState<Class | undefined>(undefined);
  const [pickedStudent, setPickedStudent] = useState<Student | undefined>(undefined);
  const [error, setError] = useState<string | null>(null);

  /**
   * Clears the form as it closes (any path: Cancel, Esc, overlay click, or a
   * successful submit) rather than as it opens, so the component is never
   * reacting to its own `open` prop with a `useEffect` — the state is simply
   * already empty by the time it is shown again.
   */
  function handleOpenChange(next: boolean) {
    if (!next) {
      setQuery("");
      setDebouncedQuery("");
      setStartedOn(today());
      setPickedClass(undefined);
      setPickedStudent(undefined);
      setError(null);
    }
    onOpenChange(next);
  }

  useEffect(() => {
    const timer = setTimeout(() => setDebouncedQuery(query), 300);
    return () => clearTimeout(timer);
  }, [query]);

  const fixedClass = useClass(props.mode === "class" ? props.classId : undefined);
  const fixedStudent = useStudent(props.mode === "student" ? props.studentId : undefined);
  const classSearch = useClassesList(props.mode === "student" ? { status: "active" } : undefined);
  const studentSearch = useStudentsList(
    props.mode === "class" ? { query: debouncedQuery, per_page: 10 } : undefined,
  );

  const createMutation = useCreateEnrollment();

  const resolvedClass = props.mode === "class" ? fixedClass.data : pickedClass;
  const resolvedStudentId = props.mode === "student" ? props.studentId : pickedStudent?.id;
  const resolvedStudentName =
    props.mode === "student" ? fixedStudent.data?.full_name : pickedStudent?.full_name;

  function handleSubmit(event: FormEvent) {
    event.preventDefault();
    setError(null);
    const classId = props.mode === "class" ? props.classId : pickedClass?.id;
    if (!classId || !resolvedStudentId) {
      setError(props.mode === "class" ? "Chọn một học sinh" : "Chọn một lớp");
      return;
    }
    const parsed = enrollmentCreateInputSchema.safeParse({
      student_id: resolvedStudentId,
      class_id: classId,
      started_on: startedOn,
    });
    if (!parsed.success) {
      setError(parsed.error.issues[0]?.message ?? "Dữ liệu không hợp lệ");
      return;
    }
    createMutation.mutate(parsed.data, {
      onSuccess: (enrollment) => {
        handleOpenChange(false);
        hvToast(
          `Học phí của ${resolvedStudentName ?? "học sinh"} được tính từ buổi học tiếp theo kể từ ${startedOn}.`,
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
      title="Ghi danh vào lớp"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
            Huỷ
          </HvButton>
          <HvButton type="submit" form="enroll-student-form" disabled={createMutation.isPending}>
            {createMutation.isPending ? "Đang lưu…" : "Ghi danh"}
          </HvButton>
        </>
      }
    >
      <form id="enroll-student-form" onSubmit={handleSubmit} noValidate>
        <FieldGroup>
          {props.mode === "student" ? (
            <Field>
              <FieldLabel htmlFor="enroll-class-search">Tìm lớp</FieldLabel>
              <Input
                id="enroll-class-search"
                role="combobox"
                placeholder="Tìm theo tên lớp"
                value={pickedClass ? pickedClass.name : query}
                onChange={(event) => {
                  setPickedClass(undefined);
                  setQuery(event.target.value);
                }}
              />
              <EntitySearchList
                items={(classSearch.data?.items ?? []).filter((klass) =>
                  klass.name.toLowerCase().includes(query.toLowerCase()),
                )}
                isFetching={classSearch.isFetching}
                selectedId={pickedClass?.id ?? ""}
                onSelect={setPickedClass}
                emptyLabel="Không tìm thấy lớp."
                renderRow={(klass) => (
                  <>
                    <span className="font-bold text-ink-900">{klass.name}</span>
                    <span className="text-ink-400">{formatMoney(klass.default_unit_price)}</span>
                  </>
                )}
              />
            </Field>
          ) : (
            <Field>
              <FieldLabel htmlFor="enroll-student-search">Tìm học sinh</FieldLabel>
              <Input
                id="enroll-student-search"
                role="combobox"
                placeholder="Tìm theo tên học sinh"
                value={pickedStudent ? pickedStudent.full_name : query}
                onChange={(event) => {
                  setPickedStudent(undefined);
                  setQuery(event.target.value);
                }}
              />
              <EntitySearchList
                items={studentSearch.data?.items ?? []}
                isFetching={studentSearch.isFetching}
                selectedId={pickedStudent?.id ?? ""}
                onSelect={setPickedStudent}
                emptyLabel="Không tìm thấy học sinh."
                renderRow={(student) => (
                  <>
                    <span className="font-bold text-ink-900">{student.full_name}</span>
                    {student.display_note ? (
                      <span className="text-ink-400">{student.display_note}</span>
                    ) : null}
                  </>
                )}
              />
            </Field>
          )}
          <Field>
            <FieldLabel htmlFor="enroll-started-on">Ngày nhập học</FieldLabel>
            <Input
              id="enroll-started-on"
              type="date"
              value={startedOn}
              onChange={(event) => setStartedOn(event.target.value)}
            />
          </Field>
          {resolvedClass ? (
            <p className="text-[13px] text-ink-400">
              Đơn giá: {formatMoney(resolvedClass.default_unit_price)}. Đơn giá kế thừa từ lớp, V1
              không sửa được.
            </p>
          ) : null}
          {error ? <FieldError>{error}</FieldError> : null}
        </FieldGroup>
      </form>
    </HvModal>
  );
}
