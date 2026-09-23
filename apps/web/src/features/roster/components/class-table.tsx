import { Link } from "react-router";

import { HvBadge } from "@/components/hv";
import { cn } from "@/lib/utils";

import { phaseLabel, phaseVariant } from "../lib/class-labels";
import { formatFullDate, formatScheduleLabel } from "../lib/roster-format";
import type { Class } from "../schemas/roster-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px]";

interface ClassTableProps {
  classes: Class[];
  /** ISO date used to pick the schedule rows effective now. */
  today: string;
  onOpen: (klass: Class) => void;
}

/**
 * The class catalog table. The whole row opens the detail; the trailing
 * "Sửa" link goes straight to the settings screen and stops the row click
 * so the two never race.
 */
export function ClassTable({ classes, today, onOpen }: ClassTableProps) {
  return (
    <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
      <table className="w-full min-w-[880px] border-collapse text-left text-[14px]">
        <thead>
          <tr>
            <th className={headCellClassName}>Lớp</th>
            <th className={headCellClassName}>Mã lớp</th>
            <th className={headCellClassName}>Khóa học</th>
            <th className={headCellClassName}>Lịch học</th>
            <th className={headCellClassName}>Thẻ</th>
            <th className={headCellClassName}>Trạng thái</th>
            <th className={headCellClassName}>Khai giảng</th>
            <th className={headCellClassName}>Kết thúc</th>
            <th className={headCellClassName}>
              <span className="sr-only">Thao tác</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {classes.map((klass) => (
            <tr
              key={klass.id}
              onClick={() => onOpen(klass)}
              className="cursor-pointer transition-colors hover:bg-cream-100"
            >
              <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                <Link
                  to={`/classes/${klass.id}`}
                  onClick={(event) => event.stopPropagation()}
                  className="hover:text-mint-600"
                >
                  {klass.name}
                </Link>
              </td>
              <td className={cn(cellClassName, "font-mono text-[13px] text-ink-700")}>
                {klass.code || "—"}
              </td>
              <td className={cellClassName}>
                {klass.course ? (
                  <HvBadge variant="info" size="sm" title={klass.course.name}>
                    {klass.course.code}
                  </HvBadge>
                ) : (
                  <span className="text-ink-400">—</span>
                )}
              </td>
              <td className={cn(cellClassName, "text-ink-500")}>
                {formatScheduleLabel(klass.schedules, today) || "Chưa có lịch"}
              </td>
              <td className={cellClassName}>
                {klass.tags.length === 0 ? (
                  <span className="text-ink-400">Chưa có thẻ</span>
                ) : (
                  <div className="flex flex-wrap gap-1">
                    {klass.tags.map((tag) => (
                      <HvBadge key={tag} variant="neutral" size="sm">
                        {tag}
                      </HvBadge>
                    ))}
                  </div>
                )}
              </td>
              <td className={cellClassName}>
                <HvBadge variant={phaseVariant[klass.phase]} size="sm" dot>
                  {phaseLabel[klass.phase]}
                </HvBadge>
              </td>
              <td className={cn(cellClassName, "text-ink-500")}>
                {formatFullDate(klass.start_date)}
              </td>
              <td className={cn(cellClassName, "text-ink-500")}>
                {klass.end_date ? formatFullDate(klass.end_date) : "—"}
              </td>
              <td className={cn(cellClassName, "text-right")}>
                <Link
                  to={`/classes/${klass.id}/settings`}
                  onClick={(event) => event.stopPropagation()}
                  className="font-display text-[13px] font-bold text-mint-600 hover:underline"
                >
                  Sửa
                </Link>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
