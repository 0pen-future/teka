import { Link } from "react-router";

import { HvBadge, HvButton, HvIcon } from "@/components/hv";
import { cn } from "@/lib/utils";

import { formatDuration, lessonModeLabel } from "../lib/library-labels";
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
    onDuplicate: (lesson: TemplateLesson) => void;
  };
}

/** Ordered lessons of one version. Callers pass `lessons` already sorted by position. */
export function LessonsTable({ lessons, templateId, editing }: LessonsTableProps) {
  return (
    <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-line-200 bg-white">
      <table className="w-full min-w-[760px] border-collapse text-left text-[14px]">
        <thead>
          <tr>
            <th className={cn(headCellClassName, "w-[64px]")}>STT</th>
            <th className={headCellClassName}>Buổi học</th>
            <th className={headCellClassName}>Hình thức</th>
            <th className={headCellClassName}>Phút</th>
            <th className={headCellClassName}>Bài tập</th>
            <th className={headCellClassName}>Nội dung</th>
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
                {lesson.unit ? (
                  <div className="text-[12px] font-normal text-ink-500">{lesson.unit}</div>
                ) : null}
              </td>
              <td className={cellClassName}>
                <HvBadge variant="neutral" size="sm">
                  {lessonModeLabel[lesson.mode]}
                </HvBadge>
              </td>
              <td className={cn(cellClassName, "text-ink-700")}>
                {formatDuration(lesson.duration_min)}
              </td>
              <td className={cn(cellClassName, "text-ink-700")}>{lesson.exercise_count}</td>
              <td className={cn(cellClassName, "text-ink-700")}>{lesson.material_count}</td>
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
                      onClick={() => editing.onDuplicate(lesson)}
                    >
                      Nhân bản
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
