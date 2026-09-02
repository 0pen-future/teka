import { useState } from "react";

import { ProgressBar } from "@/components/hv";
import { cn } from "@/lib/utils";

import type { Curriculum } from "../lib/teaching-store";

interface CurriculumCardProps {
  classTitle: string;
  curriculum: Curriculum | undefined;
  /** Lessons already taught — held sessions on the lesson axis. */
  doneCount: number;
  onEdit: () => void;
  /** Whether the viewer may edit the curriculum — see `SessionExpandRow`'s prop of the same name. */
  canWrite: boolean;
}

/**
 * CHƯƠNG TRÌNH card: course progress bar, current/next lesson lines, and the
 * expandable two-column lesson list with done/current/future styling.
 */
export function CurriculumCard({
  classTitle,
  curriculum,
  doneCount,
  onEdit,
  canWrite,
}: CurriculumCardProps) {
  const [expanded, setExpanded] = useState(false);

  const editLink = canWrite ? (
    <button
      type="button"
      onClick={onEdit}
      className="p-0 text-[13px] font-extrabold text-mint-600 hover:text-mint-500"
    >
      ✎ Sửa chương trình
    </button>
  ) : null;

  if (!curriculum) {
    return (
      <section className="min-w-[340px] flex-[1.7] rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md">
        <div className="text-[12.5px] font-extrabold tracking-[0.4px] text-ink-400">
          CHƯƠNG TRÌNH
        </div>
        <p className="mt-2 text-[13.5px] text-ink-500">
          Chưa có chương trình cho lớp {classTitle} — tạo danh sách bài để theo dõi tiến độ khóa.
        </p>
        <div className="mt-2.5">{editLink}</div>
      </section>
    );
  }

  const total = curriculum.lessons.length;
  const done = Math.min(doneCount, total);
  // Prototype semantics: "Đang học" is the last taught lesson, the highlighted
  // list row (and "Buổi tới") is the next untaught one.
  const currentTitle = curriculum.lessons[Math.max(0, done - 1)];
  const nextIndex = Math.min(done, total - 1);

  return (
    <section className="min-w-[340px] flex-[1.7] rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md">
      <div className="flex flex-wrap items-baseline gap-2.5">
        <div className="text-[12.5px] font-extrabold tracking-[0.4px] text-ink-400">
          CHƯƠNG TRÌNH
        </div>
        <div className="font-display text-[17px] font-bold text-ink-900">
          {classTitle} — {total} buổi
        </div>
        <div className="ml-auto text-[13.5px] font-extrabold text-mint-600">
          Buổi {done}/{total}
        </div>
      </div>
      <ProgressBar value={(done / total) * 100} size="sm" className="mt-2.5" />
      <div className="mt-2.5 text-[14px] text-ink-700">
        Đang học:{" "}
        <b className="text-ink-900">
          Bài {Math.max(1, done)} · {currentTitle}
        </b>
      </div>
      <div className="mt-0.5 text-[13px] text-ink-500">
        Buổi tới: Bài {nextIndex + 1} · {curriculum.lessons[nextIndex]}
      </div>
      <div className="mt-2.5 flex gap-4">
        <button
          type="button"
          onClick={() => setExpanded((current) => !current)}
          className="p-0 text-[13px] font-extrabold text-sky-500 hover:text-sky-600"
        >
          {expanded ? "Thu gọn chương trình" : "Xem toàn bộ chương trình"}
        </button>
        {editLink}
      </div>
      {expanded ? (
        <div className="mt-2.5 grid grid-cols-2 gap-x-3.5 gap-y-[2px]">
          {curriculum.lessons.map((title, index) => (
            <div
              key={`${index}:${title}`}
              className={cn(
                "flex items-baseline gap-2 rounded-[10px] px-2.5 py-1.5 text-[13px] font-bold",
                index < done
                  ? "bg-mint-50 text-mint-600"
                  : index === done
                    ? "bg-sun-100 text-sun-600"
                    : "text-ink-400",
              )}
            >
              <span className="opacity-70">{String(index + 1).padStart(2, "0")}</span>
              <span>{title}</span>
            </div>
          ))}
        </div>
      ) : null}
    </section>
  );
}
