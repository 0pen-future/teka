import { Link } from "react-router";

import { HvBadge } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { ClassShift } from "../api/classes-api";
import { phaseLabel, phaseVariant } from "../lib/class-labels";
import { formatFullDate, formatScheduleLines } from "../lib/roster-format";
import type { Class } from "../schemas/roster-schemas";

// Cell padding carries the prototype grid's 8px gap; the outer cells add the row's 18px inset.
const headCellClassName =
  "whitespace-nowrap bg-cream-200 px-1 py-[10px] text-[11.5px] font-extrabold uppercase tracking-[0.4px] text-ink-500 first:pl-[18px] last:pr-[18px]";
const cellClassName = "border-t border-line-100 px-1 py-3 first:pl-[18px] last:pr-[18px]";
const actionClassName =
  "rounded-[10px] border-[1.5px] border-line-200 px-2.5 py-[5px] text-[12px] font-extrabold";

/** Session dot per shift: morning mint, afternoon sun, evening sky. */
const shiftDotClassName: Record<ClassShift, string> = {
  morning: "bg-mint-400",
  afternoon: "bg-sun-400",
  evening: "bg-sky-400",
};

interface ClassTableProps {
  classes: Class[];
  /** ISO date used to pick the schedule rows effective now. */
  today: string;
  onOpen: (klass: Class) => void;
  onEdit?: (klass: Class) => void;
  canEdit?: (klass: Class) => boolean;
  /** Shown in place of the rows when `classes` is empty. */
  emptyLabel?: string;
}

/**
 * The class catalog table (prototype "Danh sách lớp học"). The whole row
 * opens the detail; the trailing "Sửa" / "Mở" actions stop the row click.
 */
export function ClassTable({
  classes,
  today,
  onOpen,
  onEdit,
  canEdit,
  emptyLabel,
}: ClassTableProps) {
  return (
    <div className="overflow-x-auto rounded-[20px] bg-white shadow-soft-md">
      <table className="w-full min-w-[980px] table-fixed border-collapse text-left text-[13.5px]">
        <colgroup>
          <col className="w-[62px]" />
          <col className="w-[26%]" />
          <col className="w-[24%]" />
          <col className="w-[11%]" />
          <col className="w-[13%]" />
          <col className="w-[13%]" />
          <col className="w-[13%]" />
          <col className="w-[138px]" />
        </colgroup>
        <thead>
          <tr>
            <th className={headCellClassName}>STT</th>
            <th className={headCellClassName}>Lớp học</th>
            <th className={headCellClassName}>Lịch học</th>
            <th className={headCellClassName}>Gắn thẻ</th>
            <th className={headCellClassName}>Trạng thái</th>
            <th className={headCellClassName}>Ngày bắt đầu</th>
            <th className={headCellClassName}>Ngày kết thúc</th>
            <th className={headCellClassName}>
              <span className="sr-only">Thao tác</span>
            </th>
          </tr>
        </thead>
        <tbody>
          {classes.length === 0 && emptyLabel ? (
            <tr>
              <td
                colSpan={8}
                className="border-t border-line-100 p-[34px] text-center font-bold text-ink-400"
              >
                {emptyLabel}
              </td>
            </tr>
          ) : null}
          {classes.map((klass, index) => {
            const lines = formatScheduleLines(klass.schedules, today);
            const courseLine = [klass.course?.name, klass.code].filter(Boolean).join(" · ");
            return (
              <tr
                key={klass.id}
                onClick={() => onOpen(klass)}
                className="cursor-pointer align-middle transition-colors hover:bg-cream-100"
              >
                <td className={cn(cellClassName, "font-extrabold text-ink-400")}>{index + 1}</td>
                <td className={cellClassName}>
                  <Link
                    to={`/classes/${klass.id}`}
                    onClick={(event) => event.stopPropagation()}
                    className="font-extrabold text-ink-900 hover:text-mint-600"
                  >
                    {klass.name}
                  </Link>
                  {courseLine ? (
                    <div title={courseLine} className="mt-0.5 truncate text-[12px] text-ink-400">
                      {courseLine}
                    </div>
                  ) : null}
                </td>
                <td className={cellClassName}>
                  {lines.length === 0 ? (
                    <span className="font-bold text-ink-300">Chưa có lịch</span>
                  ) : (
                    <div className="flex flex-col gap-1">
                      {lines.map((line) => (
                        <div
                          key={line.key}
                          className="flex items-center gap-2 text-[13px] text-ink-700"
                        >
                          <span
                            aria-hidden="true"
                            className={cn(
                              "size-1.5 shrink-0 rounded-full",
                              shiftDotClassName[line.shift],
                            )}
                          />
                          {line.text}
                        </div>
                      ))}
                    </div>
                  )}
                </td>
                <td className={cellClassName}>
                  {klass.tags.length === 0 ? (
                    <span className="text-ink-300">-</span>
                  ) : (
                    <div className="flex flex-wrap gap-1">
                      {klass.tags.map((tag) => (
                        <span
                          key={tag}
                          className="rounded-[8px] bg-sky-50 px-2 py-0.5 text-[11.5px] font-extrabold text-sky-600"
                        >
                          {tag}
                        </span>
                      ))}
                    </div>
                  )}
                </td>
                <td className={cellClassName}>
                  <HvBadge variant={phaseVariant[klass.phase]} size="sm">
                    {phaseLabel[klass.phase]}
                  </HvBadge>
                </td>
                <td className={cn(cellClassName, "text-ink-700")}>
                  {formatFullDate(klass.start_date)}
                </td>
                <td className={cn(cellClassName, "text-ink-700")}>
                  {klass.end_date ? formatFullDate(klass.end_date) : "—"}
                </td>
                <td className={cellClassName}>
                  <div className="flex justify-end gap-1">
                    {onEdit && (!canEdit || canEdit(klass)) ? (
                      <button
                        type="button"
                        aria-label={`Sửa lớp ${klass.name}`}
                        onClick={(event) => {
                          event.stopPropagation();
                          onEdit(klass);
                        }}
                        className={cn(
                          actionClassName,
                          "text-ink-500 hover:border-mint-400 hover:text-mint-600",
                        )}
                      >
                        Sửa
                      </button>
                    ) : null}
                    <button
                      type="button"
                      aria-label={`Mở lớp ${klass.name}`}
                      onClick={(event) => {
                        event.stopPropagation();
                        onOpen(klass);
                      }}
                      className={cn(actionClassName, "text-sky-500 hover:border-sky-300")}
                    >
                      Mở
                    </button>
                  </div>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
