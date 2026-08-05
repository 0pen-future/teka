import { useEffect, useState, type FormEvent, type ReactNode } from "react";

import { HvButton, HvModal, hvToast } from "@/components/hv";
import { Field, FieldError, FieldGroup, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { formatMoney } from "@/lib/utils";

import { useClass, useClassesList } from "../hooks/use-classes";
import { useCreateEnrollment } from "../hooks/use-enrollments";
import { useStudent, useStudentsList } from "../hooks/use-students";
import { formatWeekday } from "../lib/roster-format";
import { enrollmentCreateInputSchema } from "../schemas/roster-schemas";
import type { Class, Enrollment, Student } from "../schemas/roster-schemas";

type EnrollStudentDialogProps =
  | {
      open: boolean;
      onOpenChange: (open: boolean) => void;
      /** Enrolling one known student into a class they pick — from `StudentsPage` or `StudentDetailPage`. */
      mode: "student";
      studentId: string;
      /** Renders a "Bước 2/2" step pill — the add-student wizard's second screen. */
      stepBadge?: string;
      /**
       * Replaces the cancel button with "Để sau" — the wizard's escape hatch.
       * The student is already saved, so this dismisses without losing work.
       */
      onLater?: () => void;
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

/** "T3 · T5 — 17:30" from a class's weekly schedules, for the class picker option label. */
function formatSchedule(klass: Class): string {
  const first = klass.schedules[0];
  if (!first) {
    return "";
  }
  const days = klass.schedules.map((s) => formatWeekday(s.weekday, { short: true })).join(" · ");
  return `${days} — ${first.start_time}`;
}

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

/** The known student being enrolled, echoed back so the teacher can't mix children up. */
function StudentChip({ student }: { student: Student }) {
  return (
    <div className="flex items-center gap-3 rounded-[16px] bg-cream-100 p-3">
      <span
        aria-hidden
        className="flex h-[34px] w-[34px] shrink-0 items-center justify-center rounded-full bg-sky-50 font-display text-[15px] font-bold text-sky-500"
      >
        {student.full_name.charAt(0)}
      </span>
      <span className="min-w-0">
        <span className="block truncate font-display text-[14px] font-bold text-ink-900">
          {student.full_name}
        </span>
        <span className="block truncate text-[12.5px] text-ink-500">{student.contact_name}</span>
      </span>
    </div>
  );
}

/**
 * `POST /enrollments`, wired for either invocation direction (fixed student
 * picking a class, or fixed class searching for a student). Unit price is
 * never entered here — it is inherited from the class server-side (PRD
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
  const classOptions = classSearch.data?.items ?? [];

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
    const className = resolvedClass?.name;
    createMutation.mutate(parsed.data, {
      onSuccess: (enrollment) => {
        handleOpenChange(false);
        hvToast(
          props.mode === "student"
            ? `Đã ghi danh ${resolvedStudentName ?? "học sinh"} vào ${className ?? "lớp"} — tính tiền từ buổi có mặt đầu tiên`
            : `Học phí của ${resolvedStudentName ?? "học sinh"} được tính từ buổi học tiếp theo kể từ ${startedOn}.`,
          { variant: "success" },
        );
        onSuccess?.(enrollment);
      },
      onError: (err) => {
        setError(err instanceof Error ? err.message : "Không thể ghi danh");
      },
    });
  }

  const stepBadge = props.mode === "student" ? props.stepBadge : undefined;
  const onLater = props.mode === "student" ? props.onLater : undefined;

  return (
    <HvModal
      open={open}
      onOpenChange={handleOpenChange}
      title={
        stepBadge ? (
          <span className="flex items-center gap-2">
            <span className="rounded-full bg-mint-50 px-[9px] py-[4px] font-body text-[length:var(--text-2xs)] font-bold text-mint-600">
              {stepBadge}
            </span>
            Ghi danh vào lớp
          </span>
        ) : (
          "Ghi danh vào lớp"
        )
      }
      footer={
        <>
          {onLater ? (
            <HvButton
              type="button"
              variant="ghost"
              onClick={() => {
                handleOpenChange(false);
                onLater();
              }}
            >
              Để sau
            </HvButton>
          ) : (
            <HvButton type="button" variant="ghost" onClick={() => handleOpenChange(false)}>
              Huỷ
            </HvButton>
          )}
          <HvButton type="submit" form="enroll-student-form" disabled={createMutation.isPending}>
            {createMutation.isPending
              ? "Đang lưu…"
              : props.mode === "student"
                ? "Ghi danh vào lớp"
                : "Ghi danh"}
          </HvButton>
        </>
      }
    >
      <form id="enroll-student-form" onSubmit={handleSubmit} noValidate>
        <FieldGroup>
          {props.mode === "student" ? (
            <>
              {fixedStudent.data ? <StudentChip student={fixedStudent.data} /> : null}
              <Field>
                <FieldLabel htmlFor="enroll-class">Lớp</FieldLabel>
                <Select
                  value={pickedClass?.id ?? ""}
                  onValueChange={(value) =>
                    setPickedClass(classOptions.find((klass) => klass.id === value))
                  }
                >
                  <SelectTrigger id="enroll-class" className="w-full">
                    <SelectValue placeholder="Chọn lớp…" />
                  </SelectTrigger>
                  <SelectContent>
                    {classOptions.map((klass) => (
                      <SelectItem key={klass.id} value={klass.id}>
                        {klass.name} — {formatSchedule(klass)} ·{" "}
                        {formatMoney(klass.default_unit_price)}/buổi
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </Field>
              <Field>
                <FieldLabel htmlFor="enroll-started-on">Ngày bắt đầu</FieldLabel>
                <Input
                  id="enroll-started-on"
                  type="date"
                  value={startedOn}
                  onChange={(event) => setStartedOn(event.target.value)}
                />
              </Field>
              <Field>
                <FieldLabel htmlFor="enroll-unit-price">Đơn giá / buổi</FieldLabel>
                <div
                  id="enroll-unit-price"
                  className="rounded-[14px] border-2 border-dashed border-line-200 bg-cream-50 px-3.5 py-2.5 font-display text-[15px] font-bold text-ink-700"
                >
                  {pickedClass ? `${formatMoney(pickedClass.default_unit_price)}/buổi` : "—"}
                </div>
              </Field>
              <p className="text-[12.5px] text-ink-400">
                Đơn giá kế thừa từ lớp, lưu ở lượt ghi danh này. Chỉ tính tiền từ buổi có mặt đầu
                tiên — kỳ đã chốt không đổi.
              </p>
            </>
          ) : (
            <>
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
                  Đơn giá: {formatMoney(resolvedClass.default_unit_price)}. Đơn giá kế thừa từ lớp,
                  V1 không sửa được.
                </p>
              ) : null}
            </>
          )}
          {error ? <FieldError>{error}</FieldError> : null}
        </FieldGroup>
      </form>
    </HvModal>
  );
}
