import { X } from "lucide-react";

import { cn, findFoldedMatch } from "@/lib/utils";

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
  /** Active student search; matched name fragments are wrapped in `<mark>`. */
  query?: string;
  /** Called from the "Xoá tìm kiếm" button of the no-match state. */
  onClearSearch?: () => void;
  /** Replaces the body with shimmer rows while the month's sessions load. */
  loading?: boolean;
  /** Screen-reader text announced while `loading`. */
  loadingLabel?: string;
  /** Below `md`: no header, two-line rows and a short "Xem" button. */
  compact?: boolean;
}

const gridClassName = "grid grid-cols-[1fr_110px_84px_110px_70px_100px] items-center gap-2";

/** The white outlined pill used by the CSV buttons and the empty-state action. */
export const ghostButtonClassName =
  "flex items-center gap-2 rounded-[14px] border-2 border-line-200 bg-white px-4 py-[9px] text-[13px] font-extrabold text-ink-700 transition-colors hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none";

const viewButtonClassName =
  "rounded-xl border-2 border-line-200 px-2.5 py-[5px] text-[12.5px] font-extrabold text-ink-500 hover:border-mint-400 hover:text-mint-600 focus-visible:ring-4 focus-visible:outline-none";

const shimmerClassName =
  "h-3.5 rounded-[6px] bg-[linear-gradient(90deg,var(--color-cream-200),var(--color-cream-300),var(--color-cream-200))] bg-[length:200%_100%] animate-[hv-shimmer_1.2s_linear_infinite] motion-reduce:animate-none";

const SKELETON_ROWS = 5;

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

function formatAverage(row: StudentRecordSummary): string {
  return row.average === null ? "—" : row.average.toFixed(1);
}

/**
 * The student's name with the folded match wrapped in `<mark>`. Renders the
 * bare string when nothing matches so the name cell keeps a plain text node.
 */
function HighlightedName({ name, query }: { name: string; query: string }) {
  const match = query ? findFoldedMatch(name, query) : null;
  if (!match) {
    return name;
  }
  return (
    <>
      {name.slice(0, match.start)}
      <mark className="rounded-[4px] bg-sun-200 px-px text-ink-900">
        {name.slice(match.start, match.end)}
      </mark>
      {name.slice(match.end)}
    </>
  );
}

function TableHeader() {
  return (
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
  );
}

function DesktopRow({
  row,
  query,
  onOpen,
}: {
  row: StudentRecordSummary;
  query: string;
  onOpen: (studentId: string) => void;
}) {
  return (
    <div className={cn(gridClassName, "border-b border-line-100 px-5 py-[9px] hover:bg-cream-100")}>
      <div className="text-[14px] font-extrabold text-ink-900">
        <HighlightedName name={row.name} query={query} />
      </div>
      <div className="text-[13px] text-ink-500">—</div>
      <div className={cn("font-extrabold", averageColor(row))}>{formatAverage(row)}</div>
      <div className="flex items-center gap-1.5">
        <span className={cn("text-[16px] font-black", trendColor[row.trend.tone])}>
          {row.trend.arrow}
        </span>
        <span className="text-[12.5px] font-bold text-ink-500">{row.trend.label}</span>
      </div>
      <div className="text-[13px] text-ink-500">
        {row.absences > 0 ? `${row.absences} buổi` : "0"}
      </div>
      <button type="button" onClick={() => onOpen(row.studentId)} className={viewButtonClassName}>
        Xem hồ sơ
      </button>
    </div>
  );
}

/** Two-line phone row: name over "TB 8.6 · ↗ Tiến bộ · vắng 0", "Xem" on the right. */
function CompactRow({
  row,
  query,
  onOpen,
}: {
  row: StudentRecordSummary;
  query: string;
  onOpen: (studentId: string) => void;
}) {
  return (
    <div className="grid grid-cols-[1fr_auto] gap-x-2 gap-y-0.5 border-b border-line-100 px-3.5 py-2.5 hover:bg-cream-100">
      <div className="text-[14px] font-extrabold text-ink-900">
        <HighlightedName name={row.name} query={query} />
      </div>
      <button
        type="button"
        aria-label="Xem hồ sơ"
        onClick={() => onOpen(row.studentId)}
        className={cn(viewButtonClassName, "row-span-2 self-center justify-self-end")}
      >
        Xem
      </button>
      <div className="col-start-1 text-[12px] text-ink-500">
        TB <b className={cn("font-extrabold", averageColor(row))}>{formatAverage(row)}</b> ·{" "}
        <span className={trendColor[row.trend.tone]}>{row.trend.arrow}</span> {row.trend.label} ·
        vắng {row.absences}
      </div>
    </div>
  );
}

function SkeletonRows({ label, compact }: { label: string; compact: boolean }) {
  return (
    <>
      <p role="status" className="sr-only">
        {label}
      </p>
      {Array.from({ length: SKELETON_ROWS }, (_, index) =>
        compact ? (
          <div
            key={index}
            aria-hidden="true"
            className="flex flex-col gap-2 border-b border-line-100 px-3.5 py-3.5"
          >
            <div className={shimmerClassName} style={{ width: 120 + index * 17 }} />
            <div className={shimmerClassName} style={{ width: 150 }} />
          </div>
        ) : (
          <div
            key={index}
            aria-hidden="true"
            className={cn(gridClassName, "border-b border-line-100 px-5 py-[13px]")}
          >
            <div className={shimmerClassName} style={{ width: 120 + index * 17 }} />
            <div className={shimmerClassName} style={{ width: 30 }} />
            <div className={shimmerClassName} style={{ width: 34 }} />
            <div className={shimmerClassName} style={{ width: 70 }} />
            <div className={shimmerClassName} style={{ width: 24 }} />
            <div />
          </div>
        ),
      )}
    </>
  );
}

function SearchEmptyState({ query, onClear }: { query: string; onClear?: () => void }) {
  return (
    <div className="px-5 py-[30px] text-center text-[13.5px] text-ink-400">
      <b className="mb-1.5 block font-display text-[15px] text-ink-700">
        Không tìm thấy học sinh nào khớp “{query}”
      </b>
      Thử gõ ít ký tự hơn hoặc kiểm tra lại họ tên.
      <button
        type="button"
        onClick={onClear}
        className={cn(ghostButtonClassName, "mx-auto mt-2.5 min-h-11")}
      >
        <X className="size-3.5" aria-hidden="true" />
        Xoá tìm kiếm
      </button>
    </div>
  );
}

/** Hồ sơ học sinh list table. NGÀY SINH is always "—": no dob data exists. */
export function StudentRecordsTable({
  rows,
  onOpen,
  query = "",
  onClearSearch,
  loading = false,
  loadingLabel = "Đang tải…",
  compact = false,
}: StudentRecordsTableProps) {
  return (
    <div className="overflow-hidden rounded-[24px] bg-white shadow-soft-md">
      {compact ? null : <TableHeader />}
      <div className="max-h-[520px] overflow-auto">
        {loading ? (
          <SkeletonRows label={loadingLabel} compact={compact} />
        ) : rows.length === 0 && query ? (
          <SearchEmptyState query={query} onClear={onClearSearch} />
        ) : (
          rows.map((row) =>
            compact ? (
              <CompactRow key={row.studentId} row={row} query={query} onOpen={onOpen} />
            ) : (
              <DesktopRow key={row.studentId} row={row} query={query} onOpen={onOpen} />
            ),
          )
        )}
      </div>
    </div>
  );
}
