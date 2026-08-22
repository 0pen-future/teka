import { cn, formatSessionDate } from "@/lib/utils";

import type { StudentSessionRow } from "../lib/student-stats";

interface StudentSessionsTableProps {
  rows: StudentSessionRow[];
  /** Saved personal note per session id. */
  notes: Record<string, string>;
  /** Fired on blur only when the input's text differs from the saved note. */
  onSaveNote: (sessionId: string, text: string) => void;
}

const gridClassName = "grid grid-cols-[84px_64px_64px_1fr] items-center gap-2";

/** Per-session marks + inline personal-note inputs (saved on blur). */
export function StudentSessionsTable({ rows, notes, onSaveNote }: StudentSessionsTableProps) {
  return (
    <div className="min-w-[380px] flex-[1.3] overflow-hidden rounded-[24px] bg-white shadow-soft-md">
      <div
        className={cn(
          gridClassName,
          "border-b-[1.5px] border-line-200 px-[18px] py-3 text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400",
        )}
      >
        <div>BUỔI</div>
        <div>BÀI</div>
        <div>ĐIỂM</div>
        <div>NHẬN XÉT CÁ NHÂN (không bắt buộc)</div>
      </div>
      <div className="max-h-[400px] overflow-auto">
        {rows.length === 0 ? (
          <p className="px-[18px] py-4 text-[13px] text-ink-400">
            Chưa có buổi học nào được điểm danh trong tháng.
          </p>
        ) : (
          rows.map((row) => {
            const saved = notes[row.session.id] ?? "";
            return (
              <div
                key={row.session.id}
                className={cn(gridClassName, "border-b border-line-100 px-[18px] py-[7px]")}
              >
                <div className="text-[13px] font-extrabold text-ink-900">
                  {formatSessionDate(row.session.session_date)}
                </div>
                <div className="text-[12.5px] text-ink-500">
                  {row.lessonIndex === null ? "—" : `Bài ${row.lessonIndex + 1}`}
                </div>
                <div>
                  {row.absent ? (
                    <span className="rounded-full bg-coral-100 px-2.5 py-[3px] text-center text-[13px] font-extrabold text-coral-600">
                      Vắng
                    </span>
                  ) : row.score === null ? (
                    <span className="text-[13px] text-ink-400">—</span>
                  ) : (
                    <span className="rounded-full bg-mint-50 px-2.5 py-[3px] text-center text-[13px] font-extrabold text-mint-600">
                      {row.score.toFixed(1)}
                    </span>
                  )}
                </div>
                <input
                  // Remount per saved value so an external store update (or a
                  // class switch) refreshes this uncontrolled draft.
                  key={saved}
                  defaultValue={saved}
                  aria-label={`Nhận xét buổi ${formatSessionDate(row.session.session_date)}`}
                  placeholder="—"
                  onBlur={(event) => {
                    const text = event.target.value.trim();
                    if (text !== saved) {
                      onSaveNote(row.session.id, text);
                    }
                  }}
                  className="w-full rounded-[10px] border-[1.5px] border-transparent bg-transparent px-2 py-[5px] text-[12.5px] outline-none hover:bg-cream-100 focus:border-mint-400 focus:bg-white"
                />
              </div>
            );
          })
        )}
      </div>
    </div>
  );
}
