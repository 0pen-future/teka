import { useEffect, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router";

import { HvBadge, HvButton, HvCard, hvToast } from "@/components/hv";
import { cn, formatPhoneLocal } from "@/lib/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { ClassDialog } from "../components/class-dialog";
import { ClassSearchEmptyNote, ClassSearchInput } from "../components/class-search";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { StudentDialog } from "../components/student-dialog";
import { useClassesList } from "../hooks/use-classes";
import { useClassSearch } from "../hooks/use-class-search";
import { useStudentsList } from "../hooks/use-students";
import type { Student } from "../schemas/roster-schemas";

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
  const navigate = useNavigate();
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
      const next = new URLSearchParams(searchParams);
      if (query) {
        next.set("q", query);
      } else {
        next.delete("q");
      }
      setSearchParams(next, { replace: true });
    }, 300);
    return () => clearTimeout(timer);
    // searchParams/setSearchParams intentionally excluded: only the debounced
    // query should retrigger this effect, not every URL change it causes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [query]);

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

  function selectClass(classId: string) {
    const next = new URLSearchParams(searchParams);
    if (classId) {
      next.set("class_id", classId);
    } else {
      next.delete("class_id");
    }
    setSearchParams(next, { replace: true });
  }

  return (
    // sm+ pins the page to the viewport so only the table body scrolls, never
    // the document. The subtracted offsets mirror DashboardLayout's chrome
    // (main padding + logout row) per breakpoint — if that layout's padding or
    // header rows change, these numbers must change with it.
    <div className="flex min-h-0 flex-col gap-4 sm:h-[calc(100svh-158px)] md:h-[calc(100svh-94px)] lg:h-[calc(100svh-102px)]">
      <div>
        <div className="flex flex-wrap items-center gap-3">
          <h1 className="flex-1 font-display text-[26px] font-extrabold text-ink-900">
            Lớp &amp; học sinh
          </h1>
          {/* Only while a real class is active — a stale or mistyped class_id
              in the URL matches no class and gets no settings button. */}
          {classes.some((klass) => klass.id === effectiveClassId) ? (
            <HvButton
              variant="secondary"
              size="sm"
              onClick={() => {
                void navigate(`/classes/${effectiveClassId}/settings`);
              }}
            >
              ⚙ Cài đặt lớp
            </HvButton>
          ) : null}
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
          Chỉ lưu thông tin cần thiết để tính học phí: họ tên, ghi chú phân biệt và người liên hệ.
          Không lưu tuổi, ngày sinh, địa chỉ, trường học hay ảnh của học sinh.
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
          band. The inner div is the scroll container, so the sticky header
          row stays pinned while only the rows scroll. */}
      {/* min-h floor keeps the table usable when the tab pills wrap into
          several rows on a short viewport — without it the fixed-height page
          would squeeze this card (the only min-h-0 flex child) to nothing. */}
      <div className="hidden min-h-[240px] flex-col overflow-hidden rounded-[20px] bg-white shadow-soft-md sm:flex">
        <div className="min-h-0 overflow-auto">
          <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={tableHeadCellClassName}>Học sinh</th>
                <th className={tableHeadCellClassName}>Ghi chú</th>
                <th className={tableHeadCellClassName}>Người liên hệ</th>
                <th className={tableHeadCellClassName}>
                  <span className="sr-only">Hành động</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {students.map((student) => (
                <tr key={student.id}>
                  <td className={tableCellClassName}>
                    <div className="flex items-center gap-2">
                      <Link
                        to={`/students/${student.id}`}
                        className="font-extrabold text-ink-900 hover:text-mint-600"
                      >
                        {student.full_name}
                      </Link>
                      {isUnenrolledTab ? (
                        <HvBadge variant="warning" size="sm">
                          Chưa vào lớp nào
                        </HvBadge>
                      ) : null}
                    </div>
                  </td>
                  <td className={tableCellClassName}>
                    {student.display_note ? (
                      <HvBadge variant="info">{student.display_note}</HvBadge>
                    ) : (
                      <span className="text-ink-300">—</span>
                    )}
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
                  <td className={tableCellClassName}>
                    <div className="flex flex-wrap items-center justify-end gap-2">
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
