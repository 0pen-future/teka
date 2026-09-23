import { Link } from "react-router";

import { HvButton, HvIcon } from "@/components/hv";
import { cn } from "@/lib/utils";

import { formatDuration } from "../lib/library-labels";
import type { TemplateLesson } from "../schemas/library-schemas";

const headCellClassName =
  "sticky top-0 z-10 bg-cream-200 px-[18px] py-[10px] text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500";
const cellClassName = "border-t border-line-100 px-[18px] py-[11px] align-middle";

interface LessonsTableProps {
  lessons: TemplateLesson[];
  templateId: string;
  /** Present only when the version is a draft the viewer may author. */
  editing?: {
    pending: boolean;
    onMove: (lesson: TemplateLesson, direction: -1 | 1) => void;
    onDelete: (lesson: TemplateLesson) => void;
  };
}

/** Ordered lessons of one version. Callers pass `lessons` already sorted by position. */
export function LessonsTable({ lessons, templateId, editing }: LessonsTableProps) {
  return (
    <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
      <table className="w-full min-w-[640px] border-collapse text-left text-[14px]">
        <thead>
          <tr>
            <th className={cn(headCellClassName, "w-[64px]")}>STT</th>
            <th className={headCellClassName}>Tên buổi</th>
            <th className={headCellClassName}>Thời lượng</th>
            <th className={headCellClassName}>Bài tập về nhà</th>
            {editing ? (
              <th className={headCellClassName}>
                <span className="sr-only">Thao tác</span>
              </th>
            ) : null}
          </tr>
        </thead>
        <tbody>
          {lessons.map((lesson, index) => (
            <tr key={lesson.id} className="transition-colors hover:bg-cream-100">
              <td className={cn(cellClassName, "font-mono text-[13px] text-ink-500")}>
                {lesson.position}
              </td>
              <td className={cn(cellClassName, "font-extrabold text-ink-900")}>
                <Link
                  to={`/library/templates/${templateId}/lessons/${lesson.id}`}
                  className="hover:text-mint-600"
                >
                  {lesson.title}
                </Link>
              </td>
              <td className={cn(cellClassName, "text-ink-700")}>
                {formatDuration(lesson.duration_min)}
              </td>
              <td className={cn(cellClassName, "text-ink-700")}>
                {lesson.homework_note ? "Có" : "Không"}
              </td>
              {editing ? (
                <td className={cn(cellClassName, "whitespace-nowrap text-right")}>
                  <div className="inline-flex items-center gap-1">
                    <HvButton
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-label="Chuyển lên"
                      disabled={editing.pending || index === 0}
                      onClick={() => editing.onMove(lesson, -1)}
                    >
                      <HvIcon name="arrow-up" size={16} />
                    </HvButton>
                    <HvButton
                      type="button"
                      size="sm"
                      variant="ghost"
                      aria-label="Chuyển xuống"
                      disabled={editing.pending || index === lessons.length - 1}
                      onClick={() => editing.onMove(lesson, 1)}
                    >
                      <HvIcon name="arrow-down" size={16} />
                    </HvButton>
                    <HvButton
                      type="button"
                      size="sm"
                      variant="ghost"
                      disabled={editing.pending}
                      onClick={() => editing.onDelete(lesson)}
                    >
                      Xoá
                    </HvButton>
                  </div>
                </td>
              ) : null}
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}
