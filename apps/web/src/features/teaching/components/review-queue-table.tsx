import { cn } from "@/lib/utils";

import type { LessonPlanStatus } from "../lib/teaching-store";
import { PlanStatusPill } from "./plan-status-pill";

export interface ReviewQueueRow {
  classId: string;
  className: string;
  /** Who submitted the plan; "—" until a submission carries a name. */
  teacher: string;
  /** "Bài 3/6 · Số thập phân" or the no-curriculum note. */
  lessonLabel: string;
  status: LessonPlanStatus;
}

const gridClassName = "grid grid-cols-[110px_150px_1fr_110px] items-center gap-2.5";

/** One row per class: the upcoming lesson's giáo án and its review status. */
export function ReviewQueueTable({
  rows,
  selectedClassId,
  onSelect,
}: {
  rows: ReviewQueueRow[];
  selectedClassId: string | undefined;
  onSelect: (classId: string) => void;
}) {
  return (
    <div className="flex-[1.4] overflow-hidden rounded-[24px] bg-white shadow-soft-md">
      <div className="overflow-x-auto">
        <div className="min-w-[440px]">
          <div
            className={cn(
              gridClassName,
              "border-b-[1.5px] border-line-200 px-[18px] py-3 text-[11.5px] font-extrabold tracking-[0.3px] text-ink-400",
            )}
          >
            <div>LỚP</div>
            <div>GIÁO VIÊN</div>
            <div>GIÁO ÁN BUỔI TỚI</div>
            <div>TRẠNG THÁI</div>
          </div>
          {rows.map((row) => (
            <button
              key={row.classId}
              type="button"
              onClick={() => onSelect(row.classId)}
              className={cn(
                gridClassName,
                "w-full cursor-pointer border-b border-line-100 px-[18px] py-3 text-left focus-visible:ring-4 focus-visible:outline-none",
                row.classId === selectedClassId ? "bg-mint-50" : "hover:bg-cream-100",
              )}
            >
              <div className="text-[14px] font-extrabold text-ink-900">{row.className}</div>
              <div className="text-[13px] text-ink-500">{row.teacher}</div>
              <div className="overflow-hidden text-[13px] whitespace-nowrap text-ellipsis text-ink-700">
                {row.lessonLabel}
              </div>
              <div>
                <PlanStatusPill status={row.status} />
              </div>
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
