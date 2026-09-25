import { useState } from "react";
import { Link } from "react-router";

import { HvIcon } from "@/components/hv";

import type { TemplateLesson } from "../schemas/library-schemas";

const UNGROUPED = "Chưa phân đơn vị";

interface LessonsTreeProps {
  lessons: TemplateLesson[];
  templateId: string;
}

/** Read-only grouping of a version's lessons by `unit`, each group collapsible. */
export function LessonsTree({ lessons, templateId }: LessonsTreeProps) {
  const groups = new Map<string, TemplateLesson[]>();
  for (const lesson of lessons) {
    const key = lesson.unit ?? UNGROUPED;
    const list = groups.get(key) ?? [];
    list.push(lesson);
    groups.set(key, list);
  }
  const [open, setOpen] = useState<Set<string>>(() => new Set(groups.keys()));

  function toggle(unit: string) {
    setOpen((current) => {
      const next = new Set(current);
      if (next.has(unit)) next.delete(unit);
      else next.add(unit);
      return next;
    });
  }

  return (
    <div className="flex flex-col gap-1 rounded-[var(--radius-lg)] border border-line-200 bg-white p-2">
      {[...groups.entries()].map(([unit, rows]) => {
        const isOpen = open.has(unit);
        const minutes = rows.reduce((sum, row) => sum + (row.duration_min ?? 0), 0);
        return (
          <div key={unit} className="flex flex-col">
            <button
              type="button"
              onClick={() => toggle(unit)}
              aria-expanded={isOpen}
              className="flex items-center gap-2 rounded-[var(--radius-md)] px-2 py-2 text-left hover:bg-cream-100"
            >
              <HvIcon name={isOpen ? "chevron-down" : "chevron-right"} size={16} />
              <span className="font-display text-[14px] font-extrabold text-ink-900">{unit}</span>
              <span className="text-[12px] text-ink-500">
                {rows.length} buổi · {minutes} phút
              </span>
            </button>
            {isOpen ? (
              <ul className="ml-6 flex flex-col border-l border-line-100 pl-3">
                {rows.map((lesson) => (
                  <li
                    key={lesson.id}
                    className="flex items-center justify-between gap-2 border-t border-line-100 py-2 first:border-t-0"
                  >
                    <Link
                      to={`/library/templates/${templateId}/lessons/${lesson.id}`}
                      className="font-bold text-ink-900 hover:text-mint-600"
                    >
                      {lesson.title}
                    </Link>
                    <span className="text-[12px] text-ink-500">
                      {lesson.material_count} nội dung · {lesson.exercise_count} bài
                    </span>
                  </li>
                ))}
              </ul>
            ) : null}
          </div>
        );
      })}
    </div>
  );
}
