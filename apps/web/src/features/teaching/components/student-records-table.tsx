import { cn } from "@/lib/utils";

import type { Trend, TrendTone } from "../lib/student-stats";

export interface StudentRecordSummary {
  studentId: string;
  name: string;
  /** Raw mean (not rounded); null when the student has no scores yet. */
  average: number | null;
  scoreCount: number;
  trend: Trend;
  absences: number;
}

interface StudentRecordsTableProps {
  rows: StudentRecordSummary[];
  onOpen: (studentId: string) => void;
}

const gridClassName = "grid grid-cols-[1fr_110px_84px_110px_70px_100px] items-center gap-2";

const trendColor: Record<TrendTone, string> = {
  up: "text-mint-600",
  down: "text-coral-600",
  flat: "text-ink-400",
};

function averageColor(row: StudentRecordSummary): string {
  if (row.average === null) {
    return "text-ink-900";
  }
  if (row.average >= 8) {
    return "text-mint-600";
  }
  return row.average < 6.5 ? "text-coral-600" : "text-ink-900";
}

/** Hồ sơ học sinh list table. NGÀY SINH is always "—": no dob data exists. */
export function StudentRecordsTable({ rows, onOpen }: StudentRecordsTableProps) {
  return (
    <div className="overflow-hidden rounded-[24px] bg-white shadow-soft-md">
      <div
        className={cn(
          gridClassName,
          "border-b-[1.5px] border-line-200 px-5 py-3 text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400",
        )}
      >
        <div>HỌC SINH</div>
        <div>NGÀY SINH</div>
        <div>ĐIỂM TB</div>
        <div>XU HƯỚNG</div>
        <div>VẮNG</div>
        <div />
      </div>
      <div className="max-h-[520px] overflow-auto">
        {rows.map((row) => (
          <div
            key={row.studentId}
            className={cn(
              gridClassName,
              "border-b border-line-100 px-5 py-[9px] hover:bg-cream-100",
            )}
          >
            <div className="text-[14px] font-extrabold text-ink-900">{row.name}</div>
            <div className="text-[13px] text-ink-500">—</div>
            <div className={cn("font-extrabold", averageColor(row))}>
              {row.average === null ? "—" : row.average.toFixed(1)}
            </div>
            <div className="flex items-center gap-1.5">
              <span className={cn("text-[16px] font-black", trendColor[row.trend.tone])}>
                {row.trend.arrow}
              </span>
              <span className="text-[12.5px] font-bold text-ink-500">{row.trend.label}</span>
            </div>
            <div className="text-[13px] text-ink-500">
              {row.absences > 0 ? `${row.absences} buổi` : "0"}
            </div>
            <button
              type="button"
              onClick={() => onOpen(row.studentId)}
              className="rounded-xl border-2 border-line-200 px-2.5 py-[5px] text-[12.5px] font-extrabold text-ink-500 hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none"
            >
              Xem hồ sơ
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
