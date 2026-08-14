import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";

import { HvBadge, HvButton, HvCard, hvToast } from "@/components/hv";
import { useSessionsList } from "@/features/attendance";
import { cn, formatDayMonth, formatPhoneLocal } from "@/lib/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { ClassDialog } from "../components/class-dialog";
import { ClassSearchEmptyNote, ClassSearchInput } from "../components/class-search";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { StudentDialog } from "../components/student-dialog";
import { useClassesList } from "../hooks/use-classes";
import { useClassSearch } from "../hooks/use-class-search";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import { useStudentsList } from "../hooks/use-students";
import { currentMonth } from "../lib/current-month";
import type { Enrollment, Student } from "../schemas/roster-schemas";

/**
 * The "Chưa ghi danh" tab's sentinel in the `class_id` search param — no
 * class has this id, so it can never collide with a real tab.
 */
const UNENROLLED_TAB = "none";

/**
 * Prototype header band — cream-200 background, 12px/800 ink-500 uppercase.
 * Sticky against the card's inner scroll container so the header stays
 * pinned while rows scroll beneath it.
 */
const tableHeadCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

const tableCellClassName = "border-t border-line-100 px-[18px] py-[11px]";

/**
 * Consolidated "Lớp & học sinh" screen — the roster's primary nav
 * destination. Class pill tabs filter the same combined student × contact
 * table rather than routing to per-class pages, matching the prototype.
 */
export function StudentsPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const activeClassId = searchParams.get("class_id") ?? "";
  const isUnenrolledTab = activeClassId === UNENROLLED_TAB;
  const urlQuery = searchParams.get("q") ?? "";
  const [query, setQuery] = useState(urlQuery);
  const [classDialogOpen, setClassDialogOpen] = useState(false);
  const [studentDialogOpen, setStudentDialogOpen] = useState(false);
  const [editingStudent, setEditingStudent] = useState<Student | undefined>(undefined);
  const [anonymizing, setAnonymizing] = useState<Student | undefined>(undefined);
  /** Step 2 of the add-student wizard, or a direct enroll from the unenrolled tab. */
  const [enrolling, setEnrolling] = useState<Student | undefined>(undefined);
  const [enrollFromWizard, setEnrollFromWizard] = useState(false);

  useEffect(() => {
    const timer = setTimeout(() => {
      // Functional updater: the timer fires up to 300ms after this render,
      // and building from a captured `searchParams` would overwrite any
      // param written in between — e.g. revert a class tab clicked while
      // the user was still typing.
      setSearchParams(
        (prev) => {
          const next = new URLSearchParams(prev);
          if (query) {
            next.set("q", query);
          } else {
            next.delete("q");
          }
          return next;
        },
        { replace: true },
      );
    }, 300);
    return () => clearTimeout(timer);
  }, [query, setSearchParams]);

  // per_page must cover every active class — the search filter asserts
  // "no class matches" over this list, so a truncated page would turn a
  // paging artifact into a false claim.
  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];
  const classSearch = useClassSearch(classes);
  // No "all classes" tab — a URL without class_id falls back to the first
  // class (same default as the attendance screen) instead of an unscoped list.
  const effectiveClassId = isUnenrolledTab
    ? activeClassId
    : activeClassId || (classes[0]?.id ?? "");
  const { data: studentsPage, isPending } = useStudentsList({
    query: urlQuery,
    class_id: isUnenrolledTab ? undefined : effectiveClassId || undefined,
    unenrolled: isUnenrolledTab || undefined,
    per_page: 50,
  });
  const students = studentsPage?.items ?? [];

  // A stale or mistyped class_id in the URL (or the unenrolled tab) matches
  // no class: no settings link, no per-class queries.
  const selectedClassId = classes.some((klass) => klass.id === effectiveClassId)
    ? effectiveClassId
    : undefined;

  // BUỔI T{m} counts the selected class's scheduled (non-cancelled) sessions
  // month-to-date — the roster screen's workload view; attendance detail
  // lives on the classbook screen. currentMonth() caps `to` at today and is
  // the same helper class-settings uses, so the query key is identical and
  // React Query dedupes the fetch.
  const month = currentMonth();
  const monthNumber = Number(month.label);
  const { data: monthSessions } = useSessionsList(selectedClassId, {
    from: month.from,
    to: month.to,
  });
  const { data: classEnrollmentsPage } = useEnrollmentsList(
    { class_id: selectedClassId, active: true, per_page: 100 },
    { enabled: Boolean(selectedClassId) },
  );

  const enrollmentByStudent = new Map<string, Enrollment>();
  // Gate on the driving id: `keepPreviousData` keeps the previous class's
  // page in `data` while this query is disabled or refetching, and an
  // ungated map would show that class's dates against these students.
  if (selectedClassId) {
    for (const enrollment of classEnrollmentsPage?.items ?? []) {
      const existing = enrollmentByStudent.get(enrollment.student_id);
      // A re-enrolled student has several rows; the latest window is the live one.
      if (!existing || enrollment.started_on > existing.started_on) {
        enrollmentByStudent.set(enrollment.student_id, enrollment);
      }
    }
  }
  const countableSessionDates = (monthSessions ?? [])
    .filter((session) => session.status !== "cancelled")
    .map((session) => session.session_date);

  function monthSessionCount(studentId: string): string {
    const enrollment = enrollmentByStudent.get(studentId);
    // "—" until the sessions query resolves — a transient "0" would read as
    // "no sessions this month" while the data is simply not here yet.
    if (!enrollment || !monthSessions) {
      return "—";
    }
    // ISO dates compare correctly as strings; count only the sessions inside
    // the student's enrollment window.
    return String(
      countableSessionDates.filter(
        (date) =>
          date >= enrollment.started_on && (!enrollment.ended_on || date <= enrollment.ended_on),
      ).length,
    );
  }

  function enrollmentStartLabel(studentId: string): string {
    const enrollment = enrollmentByStudent.get(studentId);
    return enrollment ? formatDayMonth(enrollment.started_on) : "—";
  }

  function selectClass(classId: string) {
    // Functional updater for the same reason as the search debounce above:
    // this also runs from dialog callbacks whose render (and captured
    // params) may be several URL writes old.
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        if (classId) {
          next.set("class_id", classId);
        } else {
          next.delete("class_id");
        }
        return next;
      },
      { replace: true },
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <div>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="flex-1 font-display text-[26px] font-extrabold text-ink-900">
            Lớp &amp; học sinh
          </h1>
          <HvButton variant="secondary" size="sm" onClick={() => setClassDialogOpen(true)}>
            + Tạo lớp mới
          </HvButton>
          <HvButton
            size="sm"
            onClick={() => {
              setEditingStudent(undefined);
              setStudentDialogOpen(true);
            }}
          >
            + Thêm học sinh
          </HvButton>
        </div>
        <p className="mt-1 text-[13.5px] text-ink-500">
          Chỉ lưu: họ tên · ngày nhập học · lớp · người liên hệ. Không thu thập gì thêm.
        </p>
      </div>

      <div className="flex flex-col gap-2">
        <h2 className="text-[12px] font-extrabold tracking-[var(--tracking-wide)] text-ink-400">
          CHỌN LỚP
        </h2>
        <div className="flex flex-wrap items-center gap-2">
          {classSearch.showSearch ? (
            <ClassSearchInput value={classSearch.query} onChange={classSearch.setQuery} />
          ) : null}
          {/* `contents` dissolves the tablist's box so each tab wraps
              individually in the row shared with the search pill and empty
              note — otherwise the whole tab strip drops to its own line. */}
          <div role="tablist" aria-label="Lớp" className="contents">
            {[
              ...classSearch.filtered.map((klass) => ({ id: klass.id, label: klass.name })),
              { id: UNENROLLED_TAB, label: "Chưa ghi danh" },
            ].map((tab) => (
              <button
                key={tab.id}
                type="button"
                role="tab"
                aria-selected={effectiveClassId === tab.id}
                onClick={() => selectClass(tab.id)}
                className={cn(
                  // The shadow utilities override the base :focus-visible
                  // box-shadow ring, so the ring must be re-added explicitly
                  // (same trap HvButton guards against).
                  "min-h-11 rounded-full px-[18px] font-display text-[14px] font-extrabold transition-[background-color,color,box-shadow] focus-visible:outline-none focus-visible:ring-4",
                  effectiveClassId === tab.id
                    ? "bg-mint-400 text-white shadow-press-mint"
                    : "bg-white text-ink-500 shadow-soft-sm hover:bg-cream-100",
                )}
              >
                {tab.label}
              </button>
            ))}
          </div>
          {classSearch.emptyNote ? <ClassSearchEmptyNote note={classSearch.emptyNote} /> : null}
          {selectedClassId ? (
            <Link
              to={`/classes/${selectedClassId}/settings`}
              className="ml-auto inline-flex min-h-11 items-center rounded-full border-[1.5px] border-line-300 px-4 py-2 text-[13px] font-extrabold text-ink-500 transition-colors hover:border-mint-400 hover:text-mint-600"
            >
              ⚙ Cài đặt lớp
            </Link>
          ) : null}
        </div>
      </div>

      <input
        type="search"
        aria-label="Tìm theo tên học sinh"
        placeholder="Tìm theo tên học sinh"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className="w-full max-w-[240px] rounded-full border-2 border-line-200 bg-white px-4 py-2 text-[13.5px] font-bold text-ink-700 outline-none placeholder:text-ink-400 focus:border-mint-400"
      />

      {isPending ? <p className="text-[13px] text-ink-400">Đang tải…</p> : null}
      {!isPending && students.length === 0 ? (
        <HvCard variant="flat" className="text-center text-[13px] text-ink-400">
          Không có học sinh nào.
        </HvCard>
      ) : null}

      {/* Stacked cards under sm; the table below takes over from sm up. */}
      <div className="flex flex-col gap-2 sm:hidden">
        {students.map((student) => (
          <HvCard key={student.id} variant="flat" className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Link
                to={`/students/${student.id}`}
                className="font-display text-[15px] font-bold text-ink-900"
              >
                {student.full_name}
              </Link>
              {student.display_note ? (
                <HvBadge variant="info">{student.display_note}</HvBadge>
              ) : null}
            </div>
            {isUnenrolledTab ? <HvBadge variant="warning">Chưa vào lớp nào</HvBadge> : null}
            <Link to={`/contacts/${student.contact_id}`} className="text-[13px] text-ink-500">
              {student.contact_name}
            </Link>
            <a href={`tel:${student.contact_phone}`} className="text-[13px] text-mint-600">
              {formatPhoneLocal(student.contact_phone)}
            </a>
            <div className="flex gap-2">
              {isUnenrolledTab ? (
                <HvButton
                  size="sm"
                  onClick={() => {
                    setEnrollFromWizard(false);
                    setEnrolling(student);
                  }}
                >
                  Ghi danh vào lớp
                </HvButton>
              ) : null}
              <HvButton
                variant="ghost"
                size="sm"
                onClick={() => {
                  setEditingStudent(student);
                  setStudentDialogOpen(true);
                }}
              >
                Sửa
              </HvButton>
              <HvButton variant="danger" size="sm" onClick={() => setAnonymizing(student)}>
                Xoá
              </HvButton>
            </div>
          </HvCard>
        ))}
      </div>

      {/* Prototype table card: rounded-20 + soft shadow, cream-200 header
          band. The inner div scrolls on its own (capped at 62vh) with the
          header row sticky inside it, so long rosters scroll within the
          card while the document keeps its own scroll for the rest of the
          page and the footer. */}
      <div className="hidden flex-col overflow-hidden rounded-[20px] bg-white shadow-soft-md sm:flex">
        <div className="max-h-[62vh] overflow-auto">
          <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
            {/* Prototype grid 2fr 2fr 1.1fr 1fr 1.6fr as column ratios. */}
            <colgroup>
              <col className="w-[26%]" />
              <col className="w-[26%]" />
              <col className="w-[14%]" />
              <col className="w-[13%]" />
              <col className="w-[21%]" />
            </colgroup>
            <thead>
              <tr>
                <th className={tableHeadCellClassName}>Học sinh</th>
                <th className={tableHeadCellClassName}>Người liên hệ</th>
                <th className={tableHeadCellClassName}>Nhập học</th>
                <th className={tableHeadCellClassName}>Buổi T{monthNumber}</th>
                <th className={tableHeadCellClassName}>
                  {/* Visually empty per the prototype, but the cells hold the
                      display-note badge too, so the accessible name must
                      cover both. */}
                  <span className="sr-only">Ghi chú và hành động</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {students.map((student) => (
                <tr key={student.id}>
                  <td className={tableCellClassName}>
                    <Link
                      to={`/students/${student.id}`}
                      className="font-extrabold text-ink-900 hover:text-mint-600"
                    >
                      {student.full_name}
                    </Link>
                  </td>
                  <td className={tableCellClassName}>
                    <Link
                      to={`/contacts/${student.contact_id}`}
                      className="block font-bold hover:text-mint-600"
                    >
                      {student.contact_name}
                    </Link>
                    <a
                      href={`tel:${student.contact_phone}`}
                      className="text-[12.5px] text-ink-400 hover:text-mint-600"
                    >
                      {formatPhoneLocal(student.contact_phone)}
                    </a>
                  </td>
                  <td className={cn(tableCellClassName, "text-ink-500")}>
                    {enrollmentStartLabel(student.id)}
                  </td>
                  <td className={cn(tableCellClassName, "font-bold")}>
                    {monthSessionCount(student.id)}
                  </td>
                  <td className={tableCellClassName}>
                    <div className="flex flex-wrap items-center justify-end gap-2">
                      {student.display_note ? (
                        <HvBadge variant="info">{student.display_note}</HvBadge>
                      ) : null}
                      {isUnenrolledTab ? (
                        <HvBadge variant="warning" size="sm">
                          Chưa vào lớp nào
                        </HvBadge>
                      ) : null}
                      {isUnenrolledTab ? (
                        <HvButton
                          size="sm"
                          onClick={() => {
                            setEnrollFromWizard(false);
                            setEnrolling(student);
                          }}
                        >
                          Ghi danh vào lớp
                        </HvButton>
                      ) : null}
                      <HvButton
                        variant="ghost"
                        size="sm"
                        onClick={() => {
                          setEditingStudent(student);
                          setStudentDialogOpen(true);
                        }}
                      >
                        Sửa
                      </HvButton>
                      <HvButton variant="danger" size="sm" onClick={() => setAnonymizing(student)}>
                        Xoá
                      </HvButton>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>

      <ClassDialog open={classDialogOpen} onOpenChange={setClassDialogOpen} />
      <StudentDialog
        open={studentDialogOpen}
        onOpenChange={(open) => {
          setStudentDialogOpen(open);
          if (!open) {
            setEditingStudent(undefined);
          }
        }}
        student={editingStudent}
        wizard={!editingStudent}
        onSuccess={(created) => {
          if (!editingStudent) {
            setEnrollFromWizard(true);
            setEnrolling(created);
          }
        }}
      />
      {enrolling ? (
        <EnrollStudentDialog
          open={Boolean(enrolling)}
          onOpenChange={(open) => {
            if (!open) {
              setEnrolling(undefined);
              setEnrollFromWizard(false);
            }
          }}
          studentId={enrolling.id}
          stepBadge={enrollFromWizard ? "Bước 2/2" : undefined}
          onLater={
            enrollFromWizard
              ? () => {
                  hvToast('Đã lưu hồ sơ — ghi danh sau ở tab "Chưa ghi danh"');
                  selectClass(UNENROLLED_TAB);
                }
              : undefined
          }
          onSuccess={(enrollment) => selectClass(enrollment.class_id)}
        />
      ) : null}
      {anonymizing ? (
        <AnonymizeStudentDialog
          open={Boolean(anonymizing)}
          onOpenChange={(open) => {
            if (!open) {
              setAnonymizing(undefined);
            }
          }}
          student={anonymizing}
        />
      ) : null}
    </div>
  );
}
