import { useEffect, useState } from "react";
import { Link, Navigate, useSearchParams } from "react-router";

import { HvButton, HvCard, HvNotice, hvButtonVariants, hvToast } from "@/components/hv";
import { useSessionsList } from "@/features/attendance";
import { useCenterContext } from "@/features/teaching";
import { cn, formatDayMonth } from "@/lib/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { ClassSearchEmptyNote, ClassSearchInput } from "../components/class-search";
import { EnrollExistingStudentDialog } from "../components/enroll-existing-student-dialog";
import { ContactsTab } from "../components/contacts-tab";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { LoadMoreButton } from "../components/load-more-button";
import { RosterTable } from "../components/roster-table";
import { StudentDialog } from "../components/student-dialog";
import { useClassesList } from "../hooks/use-classes";
import { useClassSearch } from "../hooks/use-class-search";
import { useEnrollmentsList } from "../hooks/use-enrollments";
import { useStudentsInfinite } from "../hooks/use-students";
import { canWriteClass } from "../lib/class-permissions";
import { currentMonth } from "../lib/current-month";
import type { Enrollment, Student } from "../schemas/roster-schemas";

const PAGE_TABS = [
  { id: "all", label: "Tất cả" },
  { id: "by-class", label: "Theo lớp" },
  { id: "unenrolled", label: "Chưa vào lớp" },
  { id: "contacts", label: "Người liên hệ" },
] as const;

type PageTabId = (typeof PAGE_TABS)[number]["id"];

/**
 * A valid `?tab=` wins; old tab ids and pre-tab `?class_id=` deep links
 * (dashboard cards, class settings, old bookmarks) keep their meaning: the
 * retired classes/students tabs open "Theo lớp", `class_id=none` the
 * unenrolled list, a real class id "Theo lớp", and a bare URL "Tất cả". The
 * contacts tab falls back to "Tất cả" for a viewer who may not see it.
 */
function resolveTab(params: URLSearchParams, canSeeContacts: boolean): PageTabId {
  const tabParam = params.get("tab");
  if (tabParam === "classes" || tabParam === "students") {
    return "by-class";
  }
  if (tabParam === "contacts") {
    return canSeeContacts ? "contacts" : "all";
  }
  const known = PAGE_TABS.find((tab) => tab.id === tabParam);
  if (known) {
    return known.id;
  }
  const classId = params.get("class_id");
  // Legacy sentinel: before the page had a `tab` param, `?class_id=none`
  // meant the unenrolled view. No class can have that id, so old deep links
  // resolve without colliding with a real selection.
  if (classId === "none") {
    return "unenrolled";
  }
  return classId ? "by-class" : "all";
}

/**
 * The center's student list, open to every member holding students.list. The
 * guard is a shell around the content component so the roster queries never
 * mount for a viewer without it — an early return between hooks would still
 * let them subscribe and fire before <Navigate> takes effect.
 */
export function StudentsPage() {
  const { has, isResolved, isError } = useCenterContext();

  if (!isResolved && !isError) {
    return null;
  }
  if (!has("students.list")) {
    return <Navigate to="/" replace />;
  }
  return <StudentsPageContent />;
}

/**
 * "Học sinh": every student of the center, the per-class roster (class pills),
 * the unenrolled list, and — for contacts.view_all — the contacts with the
 * month's outstanding balance. The API scopes what each viewer reads; this
 * page only hides the actions the API would refuse.
 */
function StudentsPageContent() {
  const { isOwner, has } = useCenterContext();
  // Contacts are owner-managed (the API rejects non-owner contact writes) and
  // a student must attach to a contact, so only the owner adds, edits or
  // enrolls students.
  const canManageStudents = isOwner;
  const canSeeContacts = has("contacts.view_all");
  const canSeeDebt = has("billing.view_all");
  const canImport = has("imports.run");
  const canMapZalo = has("reports.send");

  const [searchParams, setSearchParams] = useSearchParams();
  const activeTab = resolveTab(searchParams, canSeeContacts);
  const isByClassTab = activeTab === "by-class";
  const isUnenrolledTab = activeTab === "unenrolled";
  const isContactsTab = activeTab === "contacts";
  const rawClassId = searchParams.get("class_id") ?? "";
  // The legacy `none` sentinel (see resolveTab) is a tab marker, not a class
  // selection.
  const activeClassId = rawClassId === "none" ? "" : rawClassId;
  const urlQuery = searchParams.get("q") ?? "";
  const [query, setQuery] = useState(urlQuery);
  const [studentDialogOpen, setStudentDialogOpen] = useState(false);
  const [editingStudent, setEditingStudent] = useState<Student | undefined>(undefined);
  const [anonymizing, setAnonymizing] = useState<Student | undefined>(undefined);
  /** Step 2 of the add-student wizard, or a direct enroll from the unenrolled tab. */
  const [enrolling, setEnrolling] = useState<Student | undefined>(undefined);
  const [enrollFromWizard, setEnrollFromWizard] = useState(false);
  /** The picker flow: enroll an existing student into the selected class. */
  const [enrollExistingOpen, setEnrollExistingOpen] = useState(false);

  useEffect(() => {
    // Arm the timer only while the input and the URL actually disagree.
    // `setSearchParams` gets a new identity on every URL change, so without
    // this guard the effect re-arms a write after each navigation — and a
    // timer that fires across someone else's URL write (enrolling switches
    // `class_id`) reverts it: react-router's functional updater receives the
    // params memoized at the setter's render, not the live URL.
    if (query === urlQuery) {
      return;
    }
    const timer = setTimeout(() => {
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
  }, [query, urlQuery, setSearchParams]);

  // per_page must cover every active class — the search filter asserts
  // "no class matches" over this list, so a truncated page would turn a
  // paging artifact into a false claim.
  // Only the class pills need the list.
  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 }, isByClassTab);
  const classes = classesPage?.items ?? [];
  const classSearch = useClassSearch(classes);
  // On the by-class tab a URL without class_id falls back to the first class
  // (same default as the attendance screen) instead of an unscoped list. The
  // other tabs select no class, which also silences the per-class queries
  // below.
  const effectiveClassId = isByClassTab ? activeClassId || (classes[0]?.id ?? "") : "";
  const {
    data: studentsPages,
    isPending,
    fetchNextPage,
    hasNextPage,
    isFetchingNextPage,
  } = useStudentsInfinite(
    {
      query: urlQuery,
      class_id: isByClassTab ? effectiveClassId || undefined : undefined,
      unenrolled: isUnenrolledTab || undefined,
      per_page: 50,
    },
    // The contacts tab lists no students; the by-class tab waits for a class
    // so it never flashes the unscoped list.
    { enabled: !isContactsTab && (!isByClassTab || Boolean(effectiveClassId)) },
  );
  const students = studentsPages?.pages.flatMap((page) => page.items) ?? [];
  const studentsTotal = studentsPages?.pages[studentsPages.pages.length - 1]?.meta.total;

  // A stale or mistyped class_id in the URL (or a tab that selects no class)
  // matches nothing: the class-scoped header actions hide and the per-class
  // queries stay silent.
  const selectedClass = classes.find((klass) => klass.id === effectiveClassId);
  const selectedClassId = selectedClass?.id;

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

  function setTab(tab: PageTabId, extra?: { class_id?: string }) {
    // Functional updater for the same reason as the search debounce above:
    // this also runs from dialog callbacks whose render (and captured
    // params) may be several URL writes old. `class_id` and `q` are left in
    // place when switching tabs so returning to the by-class tab restores
    // the previous selection and filter.
    setSearchParams(
      (prev) => {
        const next = new URLSearchParams(prev);
        next.set("tab", tab);
        if (extra?.class_id !== undefined) {
          next.set("class_id", extra.class_id);
        }
        return next;
      },
      { replace: true },
    );
  }

  const visibleTabs = PAGE_TABS.filter((tab) => tab.id !== "contacts" || canSeeContacts);

  function tabLabel(tab: (typeof PAGE_TABS)[number]): string {
    // Only the open tab carries a count — the total of the query it already
    // ran, so no extra request just to count.
    const counted = tab.id === "all" || tab.id === "unenrolled";
    if (counted && tab.id === activeTab && studentsTotal !== undefined) {
      return `${tab.label} (${studentsTotal})`;
    }
    return tab.label;
  }

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="font-display text-[26px] font-extrabold text-ink-900">Học sinh</h1>
          <p className="mt-1 text-[13.5px] text-ink-500">
            Chỉ lưu: họ tên · ngày nhập học · lớp · người liên hệ. Không thu thập gì thêm.
          </p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {canImport ? (
            <Link
              to="/students/import"
              className={hvButtonVariants({ variant: "secondary", size: "sm" })}
            >
              Nhập từ Excel
            </Link>
          ) : null}
          {canManageStudents &&
          isByClassTab &&
          selectedClass &&
          canWriteClass(isOwner, selectedClass) ? (
            // Hidden while the URL points at a stale/mistyped class —
            // selectedClass is undefined then and there is no class to
            // enroll into.
            <HvButton variant="secondary" size="sm" onClick={() => setEnrollExistingOpen(true)}>
              + Ghi danh học sinh
            </HvButton>
          ) : null}
          {canManageStudents ? (
            <HvButton
              size="sm"
              onClick={() => {
                setEditingStudent(undefined);
                setStudentDialogOpen(true);
              }}
            >
              + Thêm học sinh
            </HvButton>
          ) : null}
        </div>
      </div>

      {!canManageStudents ? (
        <HvNotice tone="info">Liên hệ chủ trung tâm để thêm hoặc sửa học sinh.</HvNotice>
      ) : null}

      {/* Same underline strip as the permission matrix. The pill tablist
          further down picks a class within the by-class tab, so the two
          layers carry distinct aria-labels ("Khu vực" vs "Lớp"). */}
      <div
        className="flex flex-wrap gap-x-[22px] border-b-[1.5px] border-line-200"
        role="tablist"
        aria-label="Khu vực"
      >
        {visibleTabs.map((tab) => (
          <button
            key={tab.id}
            type="button"
            role="tab"
            aria-selected={activeTab === tab.id}
            onClick={() => setTab(tab.id)}
            className={cn(
              "border-b-[3px] px-0.5 py-2.5 text-[14.5px] font-extrabold focus-visible:ring-4 focus-visible:outline-none",
              activeTab === tab.id
                ? "border-mint-400 text-ink-900"
                : "border-transparent text-ink-500",
            )}
          >
            {tabLabel(tab)}
          </button>
        ))}
      </div>

      {isByClassTab ? (
        <div className="flex flex-col gap-2">
          <h2 className="text-[12px] font-extrabold tracking-[var(--tracking-wide)] text-ink-500">
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
              {classSearch.filtered.map((klass) => (
                <button
                  key={klass.id}
                  type="button"
                  role="tab"
                  aria-selected={effectiveClassId === klass.id}
                  onClick={() => setTab("by-class", { class_id: klass.id })}
                  className={cn(
                    // The shadow utilities override the base :focus-visible
                    // box-shadow ring, so the ring must be re-added explicitly
                    // (same trap HvButton guards against).
                    "min-h-11 rounded-full px-[18px] font-display text-[14px] font-extrabold transition-[background-color,color,box-shadow] focus-visible:outline-none focus-visible:ring-4",
                    effectiveClassId === klass.id
                      ? "bg-mint-400 text-on-brand shadow-press-mint"
                      : "bg-white text-ink-500 shadow-soft-sm hover:bg-cream-100",
                  )}
                >
                  {klass.name}
                </button>
              ))}
            </div>
            {classSearch.emptyNote ? <ClassSearchEmptyNote note={classSearch.emptyNote} /> : null}
          </div>
        </div>
      ) : null}

      <input
        type="search"
        aria-label={isContactsTab ? "Tìm theo tên hoặc số điện thoại" : "Tìm theo tên học sinh"}
        placeholder={isContactsTab ? "Tìm theo tên hoặc số điện thoại" : "Tìm theo tên học sinh"}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className="w-full max-w-[280px] rounded-full border-2 border-line-200 bg-white px-4 py-2 text-[13.5px] font-bold text-ink-700 outline-none placeholder:text-ink-400 focus:border-mint-400"
      />

      {isContactsTab ? (
        <ContactsTab
          query={urlQuery}
          canCreate={isOwner}
          canSeeDebt={canSeeDebt}
          canMapZalo={canMapZalo}
        />
      ) : (
        <>
          {isPending && (!isByClassTab || effectiveClassId) ? (
            <p className="text-[13px] text-ink-500">Đang tải…</p>
          ) : null}
          {!isPending && students.length === 0 ? (
            <HvCard variant="flat" className="text-center text-[13px] text-ink-500">
              Không có học sinh nào.
            </HvCard>
          ) : null}

          <RosterTable
            variant={isUnenrolledTab ? "unenrolled" : isByClassTab ? "students" : "all"}
            students={students}
            monthNumber={monthNumber}
            enrollmentStartLabel={enrollmentStartLabel}
            monthSessionCount={monthSessionCount}
            canManage={canManageStudents}
            canOpenContact={canSeeContacts}
            onEnroll={(student) => {
              setEnrollFromWizard(false);
              setEnrolling(student);
            }}
            onEdit={(student) => {
              setEditingStudent(student);
              setStudentDialogOpen(true);
            }}
            onAnonymize={setAnonymizing}
          />

          <LoadMoreButton
            loaded={students.length}
            total={studentsTotal ?? 0}
            hasNextPage={hasNextPage}
            isFetchingNextPage={isFetchingNextPage}
            onLoadMore={() => void fetchNextPage()}
          />
        </>
      )}

      {canManageStudents && selectedClass ? (
        <EnrollExistingStudentDialog
          open={enrollExistingOpen}
          onOpenChange={setEnrollExistingOpen}
          klass={selectedClass}
        />
      ) : null}
      {canManageStudents ? (
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
      ) : null}
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
                  hvToast('Đã lưu hồ sơ — ghi danh sau ở tab "Chưa vào lớp"');
                  setTab("unenrolled");
                }
              : undefined
          }
          onSuccess={(enrollment) => setTab("by-class", { class_id: enrollment.class_id })}
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
