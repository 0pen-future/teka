import { Link } from "react-router";

import { HvButton, HvCard } from "@/components/hv";
import { cn, formatMoney } from "@/lib/utils";

import { formatScheduleSummary } from "../lib/roster-format";
import type { Class } from "../schemas/roster-schemas";

/** Same cream-200 header band as the roster table, kept in sync visually. */
const tableHeadCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";

const tableCellClassName = "border-t border-line-100 px-[18px] py-[11px]";

interface ClassesTabProps {
  classes: Class[];
  isPending: boolean;
  isError: boolean;
  onCreateClass: () => void;
}

/**
 * "Lớp học" tab: every active class with its schedule and unit price, plus
 * class creation and the per-class settings link. Deliberately renders no
 * per-class headcount — `Class` carries no count field and a roster query
 * per row would fan out N+1 requests.
 */
export function ClassesTab({ classes, isPending, isError, onCreateClass }: ClassesTabProps) {
  const today = new Date().toISOString().slice(0, 10);

  if (isPending) {
    return <p className="text-[13px] text-ink-400">Đang tải…</p>;
  }

  // A failed list must not read as "no classes yet" — that empty state
  // invites creating a duplicate class.
  if (isError) {
    return <p className="text-[14px] font-semibold text-coral-600">Không tải được danh sách lớp</p>;
  }

  if (classes.length === 0) {
    return (
      <HvCard variant="flat" className="flex flex-col items-center gap-3 py-8 text-center">
        <p className="text-[13.5px] text-ink-500">Chưa có lớp nào.</p>
        <HvButton size="sm" onClick={onCreateClass}>
          + Tạo lớp mới
        </HvButton>
      </HvCard>
    );
  }

  return (
    <div className="flex flex-col gap-3">
      <div className="flex justify-end">
        <HvButton variant="secondary" size="sm" onClick={onCreateClass}>
          + Tạo lớp mới
        </HvButton>
      </div>

      {/* Stacked cards under sm; the table below takes over from sm up. */}
      <div className="flex flex-col gap-2 sm:hidden">
        {classes.map((cls) => (
          <HvCard key={cls.id} variant="flat" className="flex flex-col gap-1">
            <p className="font-display text-[15px] font-bold text-ink-900">{cls.name}</p>
            <p className="text-[13px] text-ink-500">
              {formatScheduleSummary(cls.schedules, today)}
            </p>
            <p className="text-[13px] text-ink-500">{formatMoney(cls.default_unit_price)}/buổi</p>
            <div>
              <Link
                to={`/classes/${cls.id}/settings`}
                className="text-[13px] font-extrabold text-mint-600"
              >
                ⚙ Cài đặt
              </Link>
            </div>
          </HvCard>
        ))}
      </div>

      <div className="hidden flex-col overflow-hidden rounded-[20px] bg-white shadow-soft-md sm:flex">
        <div className="max-h-[62vh] overflow-auto">
          <table className="w-full min-w-[560px] border-collapse text-left text-[14px]">
            <colgroup>
              <col className="w-[30%]" />
              <col className="w-[35%]" />
              <col className="w-[20%]" />
              <col className="w-[15%]" />
            </colgroup>
            <thead>
              <tr>
                <th className={tableHeadCellClassName}>Lớp</th>
                <th className={tableHeadCellClassName}>Lịch học</th>
                <th className={tableHeadCellClassName}>Đơn giá</th>
                <th className={tableHeadCellClassName}>
                  <span className="sr-only">Hành động</span>
                </th>
              </tr>
            </thead>
            <tbody>
              {classes.map((cls) => (
                <tr key={cls.id}>
                  <td className={cn(tableCellClassName, "font-extrabold text-ink-900")}>
                    {cls.name}
                  </td>
                  <td className={cn(tableCellClassName, "text-ink-500")}>
                    {formatScheduleSummary(cls.schedules, today)}
                  </td>
                  <td className={cn(tableCellClassName, "text-ink-500")}>
                    {formatMoney(cls.default_unit_price)}/buổi
                  </td>
                  <td className={tableCellClassName}>
                    <div className="flex justify-end">
                      <Link
                        to={`/classes/${cls.id}/settings`}
                        className="inline-flex min-h-9 items-center rounded-full border-[1.5px] border-line-300 px-3 text-[13px] font-extrabold text-ink-500 transition-colors hover:border-mint-400 hover:text-mint-600"
                      >
                        ⚙ Cài đặt
                      </Link>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    </div>
  );
}
