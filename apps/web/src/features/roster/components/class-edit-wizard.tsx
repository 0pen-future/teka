import { zodResolver } from "@hookform/resolvers/zod";
import { useEffect, useId, useRef, useState } from "react";
import { useForm, type FieldErrors } from "react-hook-form";

import { HvButton, HvModal, HvSelect, HvStateBlock, hvToast } from "@/components/hv";
import { Field, FieldError, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { useAuthStore } from "@/features/auth";
import { useCenterContext } from "@/features/teaching";
import { useMemberDirectory } from "@/features/tasks";
import { ApiError } from "@/lib/api/errors";
import { useApiFormErrors } from "@/lib/forms/use-api-form-errors";
import { cn } from "@/lib/utils";

import { MoneyInput } from "./money-input";
import { ScheduleRowsEditor, type ScheduleRowError } from "./schedule-rows-editor";
import { getClassAvailability } from "../api/classes-api";
import { useClassInvitations } from "../hooks/use-class-invitations";
import { useClassStaff } from "../hooks/use-class-staff";
import { useClass, useClassesList, useClassRooms, useCourseOptions } from "../hooks/use-classes";
import { useSaveClassWizard } from "../hooks/use-save-class-wizard";
import { phaseLabel } from "../lib/class-labels";
import { canWriteClass } from "../lib/class-permissions";
import { activeScheduleRows } from "../lib/schedule-diff";
import {
  classWizardInputSchema,
  type Class,
  type ClassWizardInput,
  type ScheduleRowInput,
  type StudyMode,
} from "../schemas/roster-schemas";

const today = () => new Date().toISOString().slice(0, 10);

const SECTIONS = [
  { id: "info", label: "Thông tin lớp học" },
  { id: "links", label: "Liên kết lớp" },
  { id: "mode", label: "Hình thức học" },
  { id: "sched", label: "Lịch học hàng tuần" },
  { id: "res", label: "Nguồn lực dự kiến" },
  { id: "note", label: "Thông tin bổ sung" },
] as const;

type SectionId = (typeof SECTIONS)[number]["id"];

/** The prototype's suggested tags; the class's own tags are always offered too. */
const SUGGESTED_TAGS = ["Ưu tiên", "Online", "Lớp ghép", "Thí điểm"];

const QUICK_PRESETS: { label: string; rows: ScheduleRowInput[] }[] = [
  {
    label: "T3 · T5 — 19:00 (90’)",
    rows: [2, 4].map((weekday) => ({ weekday, start_time: "19:00", duration_min: 90 })),
  },
  {
    label: "T2 · T4 · T6 — 17:30 (90’)",
    rows: [1, 3, 5].map((weekday) => ({ weekday, start_time: "17:30", duration_min: 90 })),
  },
  {
    label: "T7 · CN — 09:00 (120’)",
    rows: [6, 0].map((weekday) => ({ weekday, start_time: "09:00", duration_min: 120 })),
  },
];

const MODE_OPTIONS: { value: StudyMode; label: string }[] = [
  { value: "scheduled", label: "Học theo lịch" },
  { value: "self_paced", label: "Tự học" },
];

const MODE_HINT: Record<StudyMode, string> = {
  scheduled: "Lớp học theo lịch cần ngày khai giảng và ít nhất một lịch hàng tuần.",
  self_paced: "Lớp tự học không chiếm slot thời khóa biểu; học viên học theo chương trình mẫu.",
};

/** Field order the first-error toast walks, matching the order the sections render. */
const ERROR_ORDER = [
  "name",
  "course_id",
  "code",
  "default_unit_price",
  "tags",
  "parent_class_id",
  "next_class_id",
  "start_date",
  "end_date",
  "slots",
  "room",
  "note",
] as const;

const textareaClassName = cn(
  "min-h-24 w-full rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[14.5px] text-ink-700 outline-none transition-colors",
  "placeholder:text-ink-400 focus-visible:border-mint-400 aria-invalid:border-coral-400",
);

/** "75.000đ" — the prototype's compact đồng form. */
function formatDong(amount: number): string {
  return `${String(amount).replace(/\B(?=(\d{3})+(?!\d))/g, ".")}đ`;
}

function toWizardDefaults(klass: Class): ClassWizardInput {
  return {
    name: klass.name,
    course_id: klass.course?.id ?? "",
    code: klass.code,
    default_unit_price: klass.default_unit_price,
    tags: klass.tags,
    parent_class_id: klass.parent_class_id ?? "",
    next_class_id: klass.next_class_id ?? "",
    study_mode: klass.study_mode,
    start_date: klass.start_date,
    end_date: klass.end_date ?? "",
    slots: activeScheduleRows(klass.schedules, today()),
    room: klass.room,
    teacher_id: "",
    note: klass.note ?? "",
  };
}

/** Rows complete enough to ask the availability endpoint about. */
function completeRows(rows: ScheduleRowInput[]) {
  return rows.flatMap((row) =>
    /^\d{2}:\d{2}$/.test(row.start_time) && row.duration_min
      ? [{ weekday: row.weekday, start_time: row.start_time, duration_min: row.duration_min }]
      : [],
  );
}

/** The first validation message in section order, for the prototype's save toast. */
function firstErrorMessage(errors: FieldErrors<ClassWizardInput>): string | undefined {
  for (const key of ERROR_ORDER) {
    const error = errors[key];
    if (!error) continue;
    if (key === "slots" && errors.slots) {
      if (errors.slots.message) return errors.slots.message;
      if (errors.slots.root?.message) return errors.slots.root.message;
      for (const row of Object.values(errors.slots)) {
        const rowErrors = row as FieldErrors<ScheduleRowInput> | undefined;
        const message =
          rowErrors?.start_time?.message ??
          rowErrors?.duration_min?.message ??
          rowErrors?.weekday?.message;
        if (typeof message === "string" && message) return message;
      }
    }
    if (typeof error.message === "string" && error.message) return error.message;
  }
  return errors.root?.message;
}

function SectionCard({
  id,
  title,
  children,
}: {
  id: SectionId;
  title: string;
  children: React.ReactNode;
}) {
  const headingId = `wiz-${id}-title`;
  return (
    <section
      id={`wiz-${id}`}
      aria-labelledby={headingId}
      className="rounded-[24px] border-2 border-line-100 bg-white p-5"
    >
      <h3 id={headingId} className="mb-4 font-display text-[17px] font-bold text-ink-900">
        {title}
      </h3>
      <div className="flex flex-col gap-4">{children}</div>
    </section>
  );
}

function Hint({ children }: { children: React.ReactNode }) {
  return <p className="text-[12.5px] font-semibold text-ink-400">{children}</p>;
}

/**
 * "Sửa lớp học" (prototype `cls.wiz`): one form in six sections behind a
 * left section nav — class info, lineage links, study mode, weekly
 * timetable, planned room/teacher and a note. Changes apply from today;
 * rows that already generated sessions are closed, never rewritten.
 */
export function ClassEditWizard({
  classId,
  open,
  onOpenChange,
}: {
  classId: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { data: klass, isPending, isError, error } = useClass(open ? classId : undefined);
  const { isOwner } = useCenterContext();
  const selfId = useAuthStore((state) => state.user)?.id;
  const courseOptions = useCourseOptions(open);
  const classList = useClassesList({ status: "all", per_page: 100 }, open);
  const rooms = useClassRooms(classId, open);
  const directory = useMemberDirectory(open && isOwner);
  const staff = useClassStaff(classId, open && isOwner);
  const pendingInvitations = useClassInvitations(
    { class_id: classId, status: "pending" },
    open && isOwner,
  );
  const form = useForm<ClassWizardInput>({
    resolver: zodResolver(classWizardInputSchema),
    defaultValues: {
      name: "",
      course_id: "",
      code: "",
      default_unit_price: 0,
      tags: [],
      parent_class_id: "",
      next_class_id: "",
      study_mode: "scheduled",
      start_date: "",
      end_date: "",
      slots: [],
      room: "",
      teacher_id: "",
      note: "",
    },
  });
  const { save, isPending: saving } = useSaveClassWizard(klass);
  const handleApiError = useApiFormErrors(form);
  const [activeSection, setActiveSection] = useState<SectionId>("info");
  const [quickOpen, setQuickOpen] = useState(false);
  const [finding, setFinding] = useState<"room" | "teacher" | null>(null);
  const scrollRef = useRef<HTMLDivElement>(null);
  const roomListId = useId();

  // A detail invalidation during a multi-request save must not erase user edits or its error.
  const resetForClassId = useRef<string | null>(null);
  useEffect(() => {
    if (!open) {
      resetForClassId.current = null;
      setActiveSection("info");
      setQuickOpen(false);
    } else if (klass && resetForClassId.current !== klass.id) {
      resetForClassId.current = klass.id;
      form.reset(toWizardDefaults(klass));
    }
    // form is stable from react-hook-form.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open, klass]);

  const values = form.watch();
  const { errors } = form.formState;
  const canWrite = klass ? canWriteClass(isOwner, klass) : false;
  const courses = courseOptions.data ?? [];
  const course = courses.find((option) => option.id === values.course_id);
  const courseSelectOptions = [
    // A class that has a course keeps one: the blank choice is only for classes that never had one.
    ...(klass?.course ? [] : [{ value: "", label: "— Chọn khóa học —" }]),
    // An archived course drops out of the options but stays the class's course.
    ...(klass?.course && !courses.some((option) => option.id === klass.course?.id)
      ? [{ value: klass.course.id, label: klass.course.name, meta: klass.course.code }]
      : []),
    ...courses.map((option) => ({ value: option.id, label: option.name, meta: option.code })),
  ];
  const rateChanged = klass !== undefined && values.default_unit_price !== klass.default_unit_price;
  const otherClasses = (classList.data?.items ?? []).filter((item) => item.id !== classId);
  const linkOptions = [
    { value: "", label: "— Không —" },
    ...otherClasses.map((item) => ({
      value: item.id,
      label: `${item.name} · ${phaseLabel[item.phase]}`,
      meta: item.code,
    })),
    // A linked class outside the first page (or unreadable to the viewer) must still show as linked.
    ...[klass?.parent_class_id, klass?.next_class_id]
      .filter((id): id is string => Boolean(id) && !otherClasses.some((item) => item.id === id))
      .map((id) => ({ value: id, label: "Lớp đã liên kết" })),
  ];
  const activeTeacherIds = new Set(
    (staff.data ?? []).filter((row) => row.ended_at === null).map((row) => row.teacher_id),
  );
  if (klass) activeTeacherIds.add(klass.teacher_id);
  // A second invitation to someone already invited is refused by the API.
  for (const invitation of pendingInvitations.data ?? [])
    activeTeacherIds.add(invitation.teacher_id);
  const teacherCandidates = (directory.data ?? []).filter(
    (entry) => !activeTeacherIds.has(entry.teacher_id) && entry.teacher_id !== selfId,
  );
  const pickedTeacher = teacherCandidates.find((entry) => entry.teacher_id === values.teacher_id);
  const tagOptions = [...new Set([...SUGGESTED_TAGS, ...(klass?.tags ?? []), ...values.tags])];
  const scheduled = values.study_mode === "scheduled";

  function setField<K extends keyof ClassWizardInput>(key: K, value: ClassWizardInput[K]) {
    form.setValue(key, value as never, {
      shouldDirty: true,
      shouldValidate: form.formState.isSubmitted,
    });
  }

  function jumpTo(section: SectionId) {
    setActiveSection(section);
    const container = scrollRef.current;
    const target = container?.querySelector<HTMLElement>(`#wiz-${section}`);
    if (container && target) {
      container.scrollTo?.({ top: target.offsetTop - 8, behavior: "smooth" });
    }
  }

  function trackSection() {
    const container = scrollRef.current;
    if (!container) return;
    let current: SectionId = "info";
    for (const section of SECTIONS) {
      const target = container.querySelector<HTMLElement>(`#wiz-${section.id}`);
      if (target && target.offsetTop - 16 <= container.scrollTop) current = section.id;
    }
    setActiveSection(current);
  }

  function pickCourse(nextId: string) {
    // Re-picking the current course must not reset a class-specific price.
    if (nextId === values.course_id) return;
    setField("course_id", nextId);
    const picked = courses.find((option) => option.id === nextId);
    if (picked) setField("default_unit_price", picked.default_unit_price);
  }

  function toggleTag(tag: string) {
    setField(
      "tags",
      values.tags.includes(tag)
        ? values.tags.filter((item) => item !== tag)
        : [...values.tags, tag],
    );
  }

  function applyRows(rows: ScheduleRowInput[]) {
    setField("slots", rows);
    setQuickOpen(false);
  }

  function copyCourseSchedule() {
    const source = otherClasses.find(
      (item) =>
        item.course?.id === values.course_id &&
        activeScheduleRows(item.schedules, today()).length > 0,
    );
    if (!source) {
      hvToast("Chưa có lớp nào của khóa để sao chép lịch");
      return;
    }
    applyRows(activeScheduleRows(source.schedules, today()));
  }

  async function findFree(kind: "room" | "teacher") {
    const rows = scheduled ? completeRows(values.slots) : [];
    if (rows.length === 0) {
      hvToast(
        kind === "room"
          ? "Thêm lịch học trước để tìm phòng trống"
          : "Thêm lịch học trước để tìm giáo viên rảnh",
      );
      return;
    }
    setFinding(kind);
    try {
      const availability = await getClassAvailability(rows, classId);
      if (kind === "room") {
        const free = availability.rooms.find((room) => room.free);
        if (free) {
          setField("room", free.name);
          hvToast(`Phòng ${free.name} trống vào các khung giờ đã chọn`);
        } else {
          hvToast("Không còn phòng trống — đổi lịch");
        }
      } else {
        const freeIds = new Set(
          availability.teachers.filter((row) => row.free).map((row) => row.teacher_id),
        );
        const free = teacherCandidates.find((entry) => freeIds.has(entry.teacher_id));
        if (free) {
          setField("teacher_id", free.teacher_id);
          hvToast(`${free.display_name} rảnh vào các khung giờ đã chọn`);
        } else {
          hvToast("Không có giáo viên rảnh");
        }
      }
    } catch (findError) {
      hvToast(
        findError instanceof ApiError ? findError.message : "Không kiểm tra được lịch trống",
        {
          variant: "danger",
        },
      );
    } finally {
      setFinding(null);
    }
  }

  function showSaveError(saveError: unknown) {
    if (saveError instanceof ApiError && saveError.code === "CLASS_CODE_TAKEN") {
      const message = `Mã lớp ${form.getValues("code")} đã tồn tại`;
      form.setError("code", { type: "server", message });
      hvToast(message, { variant: "danger" });
      return;
    }
    handleApiError(saveError);
    hvToast(
      saveError instanceof ApiError
        ? (Object.values(saveError.fields ?? {})[0] ?? saveError.message)
        : "Không lưu được lớp. Thử lại sau.",
      { variant: "danger" },
    );
  }

  const submit = form.handleSubmit(
    async (submitted) => {
      if (!canWrite) return;
      const result = await save(submitted, today());
      if (result.ok) {
        hvToast("Đã lưu thay đổi", { variant: "success" });
        onOpenChange(false);
      } else if (result.partial) {
        const detail = result.error instanceof ApiError ? ` (${result.error.message})` : "";
        const message =
          result.stage === "invitation"
            ? `Đã lưu lớp nhưng chưa gửi được lời mời giáo viên${detail}.`
            : "Chỉ lưu được một phần thay đổi — kiểm tra lại lịch của lớp rồi lưu lại lần nữa.";
        form.setError("root", { message });
        hvToast(message, { variant: "danger" });
      } else {
        showSaveError(result.error);
      }
    },
    (invalid) => {
      const message = firstErrorMessage(invalid);
      if (message) hvToast(message, { variant: "danger" });
    },
  );

  const slotRowErrors: ScheduleRowError[] = values.slots.map((_, index) => {
    const rowError = errors.slots?.[index];
    for (const field of ["start_time", "duration_min", "weekday"] as const) {
      const message = rowError?.[field]?.message;
      if (message) return { field, message };
    }
    return undefined;
  });

  return (
    <HvModal
      open={open}
      onOpenChange={onOpenChange}
      title="Sửa lớp học"
      description="Thay đổi áp dụng ngay; lịch đã điểm danh không đổi."
      size="xl"
      className="sm:max-w-[980px]"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={() => onOpenChange(false)}>
            Hủy
          </HvButton>
          <HvButton type="submit" form="class-wizard-form" disabled={saving || !canWrite || !klass}>
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
        <form
          id="class-wizard-form"
          onSubmit={(event) => void submit(event)}
          noValidate
          className="flex gap-6 sm:h-[min(640px,calc(90dvh-170px))]"
        >
          <nav aria-label="Các mục của lớp" className="hidden w-[200px] shrink-0 sm:block">
            <ul className="flex flex-col gap-1">
              {SECTIONS.map((section) => {
                const active = section.id === activeSection;
                return (
                  <li key={section.id}>
                    <button
                      type="button"
                      aria-current={active ? "true" : undefined}
                      onClick={() => jumpTo(section.id)}
                      className={cn(
                        "w-full rounded-r-[12px] border-l-[3px] px-3 py-2.5 text-left text-[13.5px] font-extrabold transition-colors",
                        active
                          ? "border-sky-500 bg-sky-50 text-sky-600"
                          : "border-line-100 bg-transparent text-ink-500 hover:text-ink-700",
                      )}
                    >
                      {section.label}
                    </button>
                  </li>
                );
              })}
            </ul>
          </nav>

          <div
            id="wizScroll"
            ref={scrollRef}
            onScroll={trackSection}
            className="relative flex min-w-0 flex-1 flex-col gap-4 sm:overflow-y-auto sm:pr-1"
          >
            <SectionCard id="info" title="Thông tin lớp học">
              <Field data-invalid={Boolean(errors.name)}>
                <FieldLabel htmlFor="wiz-name">Tên lớp *</FieldLabel>
                <Input
                  id="wiz-name"
                  aria-invalid={Boolean(errors.name)}
                  {...form.register("name")}
                />
                <FieldError errors={[errors.name]} />
              </Field>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field data-invalid={Boolean(errors.course_id)}>
                  <FieldLabel htmlFor="wiz-course">Khóa học</FieldLabel>
                  <HvSelect
                    id="wiz-course"
                    aria-label="Khóa học"
                    value={values.course_id}
                    onValueChange={pickCourse}
                    options={courseSelectOptions}
                    sheetTitle="Chọn khóa học"
                    placeholder="— Chọn khóa học —"
                    searchNoun="khóa học"
                    aria-invalid={Boolean(errors.course_id)}
                    disabled={courseOptions.isPending}
                  />
                  <FieldError errors={[errors.course_id]} />
                </Field>
                <Field data-invalid={Boolean(errors.code)}>
                  <FieldLabel htmlFor="wiz-code">Mã lớp</FieldLabel>
                  <Input
                    id="wiz-code"
                    className="uppercase"
                    aria-invalid={Boolean(errors.code)}
                    {...form.register("code")}
                  />
                  <FieldError errors={[errors.code]} />
                </Field>
              </div>
              <Field className="sm:max-w-[320px]" data-invalid={Boolean(errors.default_unit_price)}>
                <FieldLabel htmlFor="wiz-rate">Học phí / buổi (đ)</FieldLabel>
                <MoneyInput
                  id="wiz-rate"
                  aria-invalid={Boolean(errors.default_unit_price)}
                  value={values.default_unit_price}
                  onChange={(value) => setField("default_unit_price", value)}
                />
                <Hint>
                  {course
                    ? `Giá khóa ${course.name}: ${formatDong(course.default_unit_price)}/buổi — lớp có thể đặt khác`
                    : "Mặc định lấy theo giá khóa học"}
                </Hint>
                <FieldError errors={[errors.default_unit_price]} />
              </Field>
              {rateChanged ? (
                <p className="rounded-[var(--radius-md)] bg-sun-100 px-4 py-3 text-[13px] font-bold text-sun-600">
                  Học phí mới chỉ áp cho lượt ghi danh từ nay về sau và buổi học kế tiếp. Học phí đã
                  chốt và đã gửi không thay đổi.
                </p>
              ) : null}
              <div>
                <p className="mb-2 text-[13px] font-extrabold text-ink-500" id="wiz-tags-label">
                  Thẻ
                </p>
                <div role="group" aria-labelledby="wiz-tags-label" className="flex flex-wrap gap-2">
                  {tagOptions.map((tag) => {
                    const on = values.tags.includes(tag);
                    return (
                      <button
                        key={tag}
                        type="button"
                        aria-pressed={on}
                        onClick={() => toggleTag(tag)}
                        className={cn(
                          "rounded-full border-[1.5px] px-3 py-1.5 text-[12.5px] font-extrabold transition-colors",
                          on
                            ? "border-sky-300 bg-sky-100 text-sky-600"
                            : "border-line-200 bg-white text-ink-500 hover:border-line-300",
                        )}
                      >
                        {tag}
                      </button>
                    );
                  })}
                </div>
                <FieldError errors={[errors.tags]} />
              </div>
            </SectionCard>

            <SectionCard id="links" title="Liên kết lớp">
              <div className="grid gap-4 sm:grid-cols-2">
                <Field data-invalid={Boolean(errors.parent_class_id)}>
                  <FieldLabel htmlFor="wiz-prev">Lớp trước</FieldLabel>
                  <HvSelect
                    id="wiz-prev"
                    aria-label="Lớp trước"
                    value={values.parent_class_id}
                    onValueChange={(next) => setField("parent_class_id", next)}
                    options={linkOptions}
                    sheetTitle="Chọn lớp trước"
                    placeholder="— Không —"
                    searchNoun="lớp"
                    disabled={classList.isPending}
                  />
                  <FieldError errors={[errors.parent_class_id]} />
                </Field>
                <Field data-invalid={Boolean(errors.next_class_id)}>
                  <FieldLabel htmlFor="wiz-next">Lớp sau</FieldLabel>
                  <HvSelect
                    id="wiz-next"
                    aria-label="Lớp sau"
                    value={values.next_class_id}
                    onValueChange={(next) => setField("next_class_id", next)}
                    options={linkOptions}
                    sheetTitle="Chọn lớp sau"
                    placeholder="— Không —"
                    searchNoun="lớp"
                    disabled={classList.isPending}
                  />
                  <FieldError errors={[errors.next_class_id]} />
                </Field>
              </div>
              <Hint>Lớp sau sẽ nhận lớp này làm lớp trước; mỗi lớp chỉ có một lớp sau.</Hint>
            </SectionCard>

            <SectionCard id="mode" title="Hình thức học">
              <div role="radiogroup" aria-label="Hình thức học" className="flex flex-wrap gap-5">
                {MODE_OPTIONS.map((option) => {
                  const on = values.study_mode === option.value;
                  return (
                    <label
                      key={option.value}
                      className="flex cursor-pointer items-center gap-2 text-[14px] font-bold text-ink-700"
                    >
                      <input
                        type="radio"
                        name="study_mode"
                        value={option.value}
                        checked={on}
                        onChange={() => setField("study_mode", option.value)}
                        className="peer sr-only"
                      />
                      <span
                        aria-hidden
                        className={cn(
                          "size-[18px] rounded-full border-2 transition-colors peer-focus-visible:ring-4 peer-focus-visible:ring-sky-200",
                          on
                            ? "border-sky-500 bg-sky-500 shadow-[inset_0_0_0_3px_white]"
                            : "border-line-300 bg-white",
                        )}
                      />
                      {option.label}
                    </label>
                  );
                })}
              </div>
              <Hint>{MODE_HINT[values.study_mode]}</Hint>
              <div className="grid gap-4 sm:grid-cols-2">
                <Field data-invalid={Boolean(errors.start_date)}>
                  <FieldLabel htmlFor="wiz-start">
                    Ngày khai giảng{scheduled ? " *" : ""}
                  </FieldLabel>
                  <Input
                    id="wiz-start"
                    type="date"
                    aria-invalid={Boolean(errors.start_date)}
                    {...form.register("start_date")}
                  />
                  <FieldError errors={[errors.start_date]} />
                </Field>
                <Field data-invalid={Boolean(errors.end_date)}>
                  <FieldLabel htmlFor="wiz-end">Ngày kết thúc</FieldLabel>
                  <Input
                    id="wiz-end"
                    type="date"
                    aria-invalid={Boolean(errors.end_date)}
                    {...form.register("end_date")}
                  />
                  <FieldError errors={[errors.end_date]} />
                </Field>
              </div>
            </SectionCard>

            <SectionCard id="sched" title="Lịch học hàng tuần">
              {scheduled ? (
                <>
                  <div className="flex flex-wrap items-center gap-2">
                    <HvButton
                      type="button"
                      variant="secondary"
                      size="sm"
                      aria-expanded={quickOpen}
                      onClick={() => setQuickOpen((value) => !value)}
                    >
                      Chọn nhanh
                    </HvButton>
                    <HvButton
                      type="button"
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        setField("slots", [
                          ...values.slots,
                          { weekday: 2, start_time: "", duration_min: 90 },
                        ])
                      }
                    >
                      + Thêm lịch học
                    </HvButton>
                  </div>
                  {quickOpen ? (
                    <div
                      role="group"
                      aria-label="Lịch chọn nhanh"
                      className="flex flex-wrap gap-2 rounded-[16px] bg-sky-50 p-3"
                    >
                      {QUICK_PRESETS.map((preset) => (
                        <button
                          key={preset.label}
                          type="button"
                          onClick={() => applyRows(preset.rows)}
                          className="rounded-full border-[1.5px] border-sky-200 bg-white px-3 py-1.5 text-[12.5px] font-extrabold text-sky-600 hover:border-sky-400"
                        >
                          {preset.label}
                        </button>
                      ))}
                      {course ? (
                        <button
                          type="button"
                          onClick={copyCourseSchedule}
                          className="rounded-full border-[1.5px] border-mint-200 bg-white px-3 py-1.5 text-[12.5px] font-extrabold text-mint-700 hover:border-mint-400"
                        >
                          Sao chép lịch khóa {course.name}
                        </button>
                      ) : null}
                    </div>
                  ) : null}
                  {values.slots.length > 0 ? (
                    <ScheduleRowsEditor
                      value={values.slots}
                      onChange={(next) => setField("slots", next)}
                      rowErrors={slotRowErrors}
                      idPrefix="wiz-slot"
                    />
                  ) : (
                    <p className="rounded-[16px] border-2 border-dashed border-line-200 px-4 py-5 text-center text-[13px] font-bold text-ink-400">
                      Chưa có lịch. Dùng “Chọn nhanh” hoặc “+ Thêm lịch học”.
                    </p>
                  )}
                  <FieldError
                    errors={[
                      errors.slots?.root ?? (errors.slots?.message ? errors.slots : undefined),
                    ]}
                  />
                </>
              ) : (
                <p className="rounded-[16px] border-2 border-dashed border-line-200 px-4 py-5 text-center text-[13px] font-bold text-ink-400">
                  Lớp tự học — không cần lịch cố định.
                </p>
              )}
            </SectionCard>

            <SectionCard id="res" title="Nguồn lực dự kiến">
              <Field data-invalid={Boolean(errors.room)}>
                <FieldLabel htmlFor="wiz-room">Phòng học</FieldLabel>
                <div className="flex flex-wrap gap-2">
                  <Input
                    id="wiz-room"
                    list={roomListId}
                    placeholder="Phòng học"
                    className="min-w-0 flex-1 basis-48"
                    aria-invalid={Boolean(errors.room)}
                    {...form.register("room")}
                  />
                  <datalist id={roomListId}>
                    {(rooms.data ?? []).map((name) => (
                      <option key={name} value={name} />
                    ))}
                  </datalist>
                  <HvButton
                    type="button"
                    variant="secondary"
                    size="sm"
                    disabled={finding !== null}
                    onClick={() => void findFree("room")}
                  >
                    {finding === "room" ? "Đang tìm…" : "Tìm phòng trống"}
                  </HvButton>
                </div>
                <Hint>
                  {values.room.trim()
                    ? `Phòng ${values.room.trim()} — kiểm tra trùng lịch khi xác nhận.`
                    : "Chưa có phòng học; có thể bỏ qua."}
                </Hint>
                <FieldError errors={[errors.room]} />
              </Field>
              <Field>
                <FieldLabel htmlFor="wiz-teacher">Giáo viên dự kiến</FieldLabel>
                <div className="flex flex-wrap gap-2">
                  <HvSelect
                    id="wiz-teacher"
                    aria-label="Giáo viên dự kiến"
                    className="min-w-0 flex-1 basis-48"
                    value={values.teacher_id}
                    onValueChange={(next) => setField("teacher_id", next)}
                    options={[
                      { value: "", label: "Giáo viên dự kiến" },
                      ...teacherCandidates.map((entry) => ({
                        value: entry.teacher_id,
                        label: entry.display_name,
                        meta: entry.role_name ?? undefined,
                      })),
                    ]}
                    sheetTitle="Chọn giáo viên dự kiến"
                    placeholder="Giáo viên dự kiến"
                    searchNoun="giáo viên"
                    disabled={!isOwner || directory.isPending}
                  />
                  <HvButton
                    type="button"
                    variant="secondary"
                    size="sm"
                    disabled={!isOwner || finding !== null}
                    onClick={() => void findFree("teacher")}
                  >
                    {finding === "teacher" ? "Đang tìm…" : "Tìm giáo viên rảnh"}
                  </HvButton>
                </div>
                <Hint>
                  {!isOwner
                    ? "Chỉ chủ trung tâm mới mời được giáo viên."
                    : pickedTeacher
                      ? `Lưu sẽ gửi lời mời dạy lớp cho ${pickedTeacher.display_name} — chờ xác nhận.`
                      : "Chọn giáo viên để gửi lời mời khi lưu; có thể bỏ qua."}
                </Hint>
              </Field>
            </SectionCard>

            <SectionCard id="note" title="Thông tin bổ sung">
              <Field data-invalid={Boolean(errors.note)}>
                <FieldLabel htmlFor="wiz-note-text">Ghi chú</FieldLabel>
                <textarea
                  id="wiz-note-text"
                  className={textareaClassName}
                  placeholder="Ghi chú vận hành cho lớp"
                  aria-invalid={Boolean(errors.note)}
                  {...form.register("note")}
                />
                <FieldError errors={[errors.note]} />
              </Field>
              {!canWrite ? (
                <p className="text-[13px] text-ink-400">
                  Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được lớp.
                </p>
              ) : null}
              <FieldError errors={[errors.root]} />
            </SectionCard>
          </div>
        </form>
      )}
    </HvModal>
  );
}
