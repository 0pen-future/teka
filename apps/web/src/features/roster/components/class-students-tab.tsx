import { Link } from "react-router";

import { HvNotice, HvStateBlock } from "@/components/hv";
import { useCenterContext } from "@/features/teaching";
import { cn, formatPhoneLocal } from "@/lib/utils";

import { useEnrollmentsList } from "../hooks/use-enrollments";
import { useStudentsList } from "../hooks/use-students";
import { formatFullDate } from "../lib/roster-format";
import type { Class } from "../schemas/roster-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px]";

/**
 * Active enrollments of the class. The contact columns come from the
 * students list, which only members holding `students.list` may read; the
 * roster itself still renders for everyone who can open the class.
 */
export function ClassStudentsTab({ klass }: { klass: Class }) {
  const { has } = useCenterContext();
  const enrollments = useEnrollmentsList({ class_id: klass.id, active: true, per_page: 100 });
  const canReadStudents = has("students.list");
  const students = useStudentsList(
    { class_id: klass.id, per_page: 100 },
    { enabled: canReadStudents },
  );
  const contactByStudent = new Map(
    (students.data?.items ?? []).map((student) => [student.id, student] as const),
  );
  const rows = enrollments.data?.items ?? [];
  const total = enrollments.data?.meta.total ?? rows.length;
  const truncated = rows.length < total;

  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="font-display text-[16px] font-bold text-ink-900">Học viên ({total})</h2>
        <Link
          to={`/students?class_id=${klass.id}`}
          className="font-display text-[13px] font-bold text-mint-600 hover:underline"
        >
          Thêm học viên
        </Link>
      </div>

      {truncated ? (
        <HvNotice tone="warning">
          Đang hiển thị {rows.length}/{total} học viên.{" "}
          <Link to={`/students?class_id=${klass.id}`} className="font-bold underline">
            Xem tất cả
          </Link>
        </HvNotice>
      ) : null}

      {enrollments.isPending ? (
        <HvStateBlock state="loading" title="Đang tải học viên" />
      ) : enrollments.isError ? (
        <HvStateBlock state="error" title="Không tải được danh sách học viên" />
      ) : rows.length === 0 ? (
        <HvStateBlock
          state="empty"
          title="Lớp chưa có học viên. Bật “Cần tuyển sinh” để đưa lớp vào danh sách tuyển sinh."
        />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
          <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
            <thead>
              <tr>
                <th className={headCellClassName}>Học viên</th>
                <th className={headCellClassName}>Phụ huynh</th>
                <th className={headCellClassName}>Điện thoại</th>
                <th className={headCellClassName}>Ngày vào lớp</th>
              </tr>
            </thead>
            <tbody>
              {rows.map((enrollment) => {
                const student = contactByStudent.get(enrollment.student_id);
                return (
                  <tr key={enrollment.id}>
                    <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                      <Link
                        to={`/students/${enrollment.student_id}`}
                        className="hover:text-mint-600"
                      >
                        {enrollment.student_name}
                      </Link>
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {student?.contact_name ?? "—"}
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {student?.contact_phone ? formatPhoneLocal(student.contact_phone) : "—"}
                    </td>
                    <td className={cn(cellClassName, "text-ink-500")}>
                      {formatFullDate(enrollment.started_on)}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
