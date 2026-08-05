import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router";

import { HvBadge, HvButton, HvCard, hvToast } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { cn, formatPhoneLocal } from "@/lib/utils";

import { AnonymizeStudentDialog } from "../components/anonymize-student-dialog";
import { ClassDialog } from "../components/class-dialog";
import { EnrollStudentDialog } from "../components/enroll-student-dialog";
import { StudentDialog } from "../components/student-dialog";
import { useClassesList } from "../hooks/use-classes";
import { useStudentsList } from "../hooks/use-students";
import type { Student } from "../schemas/roster-schemas";

/**
 * The "Chưa ghi danh" tab's sentinel in the `class_id` search param — no
 * class has this id, so it can never collide with a real tab.
 */
const UNENROLLED_TAB = "none";

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

  const { data: classesPage } = useClassesList({ status: "active" });
  const classes = classesPage?.items ?? [];
  const { data: studentsPage, isPending } = useStudentsList({
    query: urlQuery,
    class_id: isUnenrolledTab ? undefined : activeClassId || undefined,
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
    <div className="flex flex-col gap-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h1 className="font-display text-[22px] font-bold text-ink-900">Lớp &amp; học sinh</h1>
        <div className="flex gap-2">
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
      </div>

      <div className="flex flex-wrap items-center gap-2">
        <div role="tablist" aria-label="Lớp" className="flex flex-wrap gap-2">
          {[
            { id: "", label: "Tất cả" },
            ...classes.map((klass) => ({ id: klass.id, label: klass.name })),
            { id: UNENROLLED_TAB, label: "Chưa ghi danh" },
          ].map((tab) => (
            <button
              key={tab.id || "all"}
              type="button"
              role="tab"
              aria-selected={activeClassId === tab.id}
              onClick={() => selectClass(tab.id)}
              className={cn(
                "min-h-11 rounded-full px-4 font-display text-[14px] font-bold transition-[background-color,color,box-shadow]",
                activeClassId === tab.id
                  ? "bg-mint-400 text-white shadow-press-mint"
                  : "bg-white text-ink-500 shadow-sm hover:bg-cream-100",
              )}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="ml-auto flex items-center gap-3">
          {/* Prototype: the ⚙ pill shows only while a real class tab is active —
              a stale or mistyped class_id in the URL matches no tab and gets no pill. */}
          {classes.some((klass) => klass.id === activeClassId) ? (
            <Link
              to={`/classes/${activeClassId}/settings`}
              className="inline-flex min-h-11 items-center rounded-full border-[1.5px] border-line-300 px-4 font-display text-[13px] font-bold text-ink-500 transition-colors hover:border-mint-400 hover:text-mint-600"
            >
              ⚙ Cài đặt lớp
            </Link>
          ) : null}
        </div>
      </div>

      <Input
        placeholder="Tìm theo tên học sinh"
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        className="max-w-sm"
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

      <HvCard variant="flat" padding="sm" className="hidden overflow-x-auto p-0 sm:block">
        <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
          <thead>
            <tr className="border-b border-line-200 text-[13px] text-ink-400">
              <th className="px-4 py-3 font-display font-bold">Học sinh</th>
              <th className="px-4 py-3 font-display font-bold">Ghi chú</th>
              <th className="px-4 py-3 font-display font-bold">Người liên hệ</th>
              <th className="px-4 py-3 font-display font-bold">Số điện thoại</th>
              <th className="px-4 py-3 font-display font-bold">Hành động</th>
            </tr>
          </thead>
          <tbody>
            {students.map((student) => (
              <tr key={student.id} className="border-b border-line-100 last:border-0">
                <td className="px-4 py-3">
                  <div className="flex items-center gap-2">
                    <Link
                      to={`/students/${student.id}`}
                      className="font-display font-bold text-ink-900 hover:text-mint-600"
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
                <td className="px-4 py-3">
                  {student.display_note ? (
                    <HvBadge variant="info">{student.display_note}</HvBadge>
                  ) : (
                    <span className="text-ink-300">—</span>
                  )}
                </td>
                <td className="px-4 py-3">
                  <Link to={`/contacts/${student.contact_id}`} className="hover:text-mint-600">
                    {student.contact_name}
                  </Link>
                </td>
                <td className="px-4 py-3">
                  <a
                    href={`tel:${student.contact_phone}`}
                    className="text-mint-600 hover:underline"
                  >
                    {formatPhoneLocal(student.contact_phone)}
                  </a>
                </td>
                <td className="px-4 py-3">
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
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </HvCard>

      <p className="text-[12px] text-ink-400">
        Chỉ lưu thông tin cần thiết để tính học phí: họ tên, ghi chú phân biệt và người liên hệ.
        Không lưu tuổi, ngày sinh, địa chỉ, trường học hay ảnh của học sinh.
      </p>

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
