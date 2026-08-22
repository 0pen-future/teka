import { cn } from "@/lib/utils";

import type { StudentSessionRow } from "../lib/student-stats";

interface ScoreBarChartProps {
  rows: StudentSessionRow[];
  monthNumber: number;
}

function barColor(row: StudentSessionRow): string {
  if (row.absent) {
    return "bg-coral-100";
  }
  if (row.score === null) {
    return "bg-cream-200";
  }
  if (row.score >= 8) {
    return "bg-mint-400";
  }
  return row.score < 6 ? "bg-coral-400" : "bg-sky-300";
}

/** ĐIỂM KIỂM TRA TỪNG BUỔI card: one hand-sized bar per held session. */
export function ScoreBarChart({ rows, monthNumber }: ScoreBarChartProps) {
  return (
    <section className="min-w-[340px] flex-1 rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md">
      <div className="text-[12.5px] font-extrabold tracking-[0.4px] text-ink-400">
        ĐIỂM KIỂM TRA TỪNG BUỔI — THÁNG {String(monthNumber).padStart(2, "0")}
      </div>
      {rows.length === 0 ? (
        <p className="mt-3.5 text-[13px] text-ink-400">
          Chưa có buổi học nào trong tháng {monthNumber}.
        </p>
      ) : (
        <div className="mt-3.5 flex h-[124px] items-end gap-2.5 overflow-x-auto pt-1.5 pb-1">
          {rows.map((row) => (
            <div key={row.session.id} className="flex flex-col items-center gap-1">
              <div className="text-[11px] font-extrabold text-ink-700">
                {row.absent ? "V" : row.score === null ? "—" : row.score.toFixed(1)}
              </div>
              <div
                className={cn("w-[22px] rounded-t-[7px] rounded-b-[3px]", barColor(row))}
                style={{
                  height: `${Math.max(6, Math.round(((row.score ?? 0) / 10) * 80))}px`,
                }}
              />
              <div className="text-[10.5px] font-bold text-ink-400">
                {row.session.session_date.slice(8, 10)}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}
