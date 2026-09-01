import { Link } from "react-router";

import { HvBadge, HvButton, HvCard } from "@/components/hv";
import { cn, formatPhoneLocal } from "@/lib/utils";

import type { Student } from "../schemas/roster-schemas";

/**
 * Prototype header band — cream-200 background, 12px/800 ink-500 uppercase.
 * Sticky against the card's inner scroll container so the header stays
 * pinned while rows scroll beneath it.
 */
const tableHeadCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

const tableCellClassName = "border-t border-line-100 px-[18px] py-[11px]";

interface RosterTableProps {
  /**
   * The two roster tabs share this table; "unenrolled" adds the warning badge
   * and the per-row enroll action while the enrollment columns degrade to "—"
   * through the label callbacks.
   */
  variant: "students" | "unenrolled";
  students: Student[];
  monthNumber: number;
  enrollmentStartLabel: (studentId: string) => string;
  monthSessionCount: (studentId: string) => string;
  onEnroll: (student: Student) => void;
  onEdit: (student: Student) => void;
  onAnonymize: (student: Student) => void;
}

/** The combined student × contact roster: stacked cards under sm, table above. */
export function RosterTable({
  variant,
  students,
  monthNumber,
  enrollmentStartLabel,
  monthSessionCount,
  onEnroll,
  onEdit,
  onAnonymize,
}: RosterTableProps) {
  const isUnenrolled = variant === "unenrolled";

  return (
    <>
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
            {isUnenrolled ? <HvBadge variant="warning">Chưa vào lớp nào</HvBadge> : null}
            <Link to={`/contacts/${student.contact_id}`} className="text-[13px] text-ink-500">
              {student.contact_name}
            </Link>
            {student.contact_phone ? (
              <a href={`tel:${student.contact_phone}`} className="text-[13px] text-mint-600">
                {formatPhoneLocal(student.contact_phone)}
              </a>
            ) : null}
            <div className="flex gap-2">
              {isUnenrolled ? (
                <HvButton size="sm" onClick={() => onEnroll(student)}>
                  Ghi danh vào lớp
                </HvButton>
              ) : null}
              <HvButton variant="ghost" size="sm" onClick={() => onEdit(student)}>
                Sửa
              </HvButton>
              <HvButton variant="danger" size="sm" onClick={() => onAnonymize(student)}>
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
                    {student.contact_phone ? (
                      <a
                        href={`tel:${student.contact_phone}`}
                        className="text-[12.5px] text-ink-400 hover:text-mint-600"
                      >
                        {formatPhoneLocal(student.contact_phone)}
                      </a>
                    ) : null}
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
                      {isUnenrolled ? (
                        <HvBadge variant="warning" size="sm">
                          Chưa vào lớp nào
                        </HvBadge>
                      ) : null}
                      {isUnenrolled ? (
                        <HvButton size="sm" onClick={() => onEnroll(student)}>
                          Ghi danh vào lớp
                        </HvButton>
                      ) : null}
                      <HvButton variant="ghost" size="sm" onClick={() => onEdit(student)}>
                        Sửa
                      </HvButton>
                      <HvButton variant="danger" size="sm" onClick={() => onAnonymize(student)}>
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
    </>
  );
}
