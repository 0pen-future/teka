import { cn } from "@/lib/utils";

import type { MonthHeadcount, RetentionStat } from "../lib/classbook-stats";

interface MonthlyHeadcountCardProps {
  history: MonthHeadcount[];
  retention: RetentionStat;
  monthNumber: number;
  previousMonthNumber: number;
}

/** SĨ SỐ THEO THÁNG mini bar chart with the retention summary line. */
export function MonthlyHeadcountCard({
  history,
  retention,
  monthNumber,
  previousMonthNumber,
}: MonthlyHeadcountCardProps) {
  const max = Math.max(1, ...history.map((item) => item.count));

  return (
    <section className="rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md">
      <div className="text-[12.5px] font-extrabold tracking-[0.4px] text-ink-400">
        SĨ SỐ THEO THÁNG
      </div>
      <div className="mt-3.5 flex h-[92px] items-end gap-4">
        {history.map((item, index) => (
          <div key={item.label} className="flex flex-col items-center gap-1">
            <div className="text-[12px] font-extrabold text-ink-700">{item.count}</div>
            <div
              className={cn(
                "w-[26px] rounded-t-lg rounded-b",
                index === history.length - 1 ? "bg-mint-400" : "bg-sky-100",
              )}
              style={{ height: `${Math.max(8, Math.round((item.count / max) * 64))}px` }}
            />
            <div className="text-[11.5px] font-bold text-ink-400">{item.label}</div>
          </div>
        ))}
      </div>
      <div className="mt-3 rounded-xl bg-mint-50 px-3 py-2 text-[12.5px] font-bold text-mint-600">
        Tái tục T{previousMonthNumber}→T{monthNumber}: {retention.pct}% — {retention.continuing}/
        {retention.previous} học sinh học tiếp
      </div>
    </section>
  );
}
