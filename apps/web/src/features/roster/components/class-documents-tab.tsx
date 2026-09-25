import { useId, useState } from "react";

import { HvBadge } from "@/components/hv";
import { materialKindLabel } from "@/features/library";
import { cn } from "@/lib/utils";

import { useClassProgram, useClassProgramLessons } from "../hooks/use-class-program";
import type { Class } from "../schemas/roster-schemas";
import { LessonSections, ProgramTabState } from "./program-tab-state";

interface ClassDocumentsTabProps {
  klass: Class;
}

/**
 * "Tài liệu": the applied template's materials per lesson. By default only
 * what is shared with students shows, which is what a teacher hands out;
 * "Hiện tất cả" reveals the teacher-only ones too.
 */
export function ClassDocumentsTab({ klass }: ClassDocumentsTabProps) {
  const program = useClassProgram(klass.id);
  const lessons = useClassProgramLessons(klass.id, Boolean(program.data));
  const [showAll, setShowAll] = useState(false);
  const switchLabelId = useId();

  return (
    <div className="flex flex-col gap-3">
      {program.data ? (
        <div className="flex items-center justify-end gap-2">
          <span id={switchLabelId} className="text-[13px] font-bold text-ink-700">
            Hiện tất cả
          </span>
          <button
            type="button"
            role="switch"
            aria-checked={showAll}
            aria-labelledby={switchLabelId}
            onClick={() => setShowAll((value) => !value)}
            className={cn(
              "relative inline-flex h-7 w-12 shrink-0 items-center rounded-full border-2 border-transparent transition-colors",
              "focus-visible:outline-none focus-visible:ring-4 focus-visible:ring-mint-100",
              showAll ? "bg-mint-400" : "bg-line-300",
            )}
          >
            <span
              aria-hidden="true"
              className={cn(
                "inline-block size-5 rounded-full bg-white shadow transition-transform",
                showAll ? "translate-x-5" : "translate-x-0",
              )}
            />
          </button>
        </div>
      ) : null}

      <ProgramTabState
        program={program}
        lessons={lessons}
        emptyDescription="Áp dụng chương trình ở tab Thông tin để xem tài liệu theo buổi."
      >
        {(items) => (
          <LessonSections lessons={items}>
            {(lesson) => {
              const materials = showAll
                ? lesson.materials
                : lesson.materials.filter((material) => material.shared_with_students);
              return materials.length === 0 ? (
                <p className="text-[13px] text-ink-400">
                  {showAll ? "Chưa gắn tài liệu" : "Chưa có tài liệu chia sẻ với học sinh"}
                </p>
              ) : (
                <ul className="flex flex-col gap-2">
                  {materials.map((material) => (
                    <li key={material.id} className="flex flex-wrap items-center gap-2 text-[14px]">
                      {material.url ? (
                        <a
                          href={material.url}
                          target="_blank"
                          rel="noreferrer"
                          className="font-display font-bold text-mint-600 hover:underline"
                        >
                          {material.title}
                        </a>
                      ) : (
                        <span className="font-bold text-ink-900">{material.title}</span>
                      )}
                      <span className="text-[13px] text-ink-500">
                        {materialKindLabel[material.kind]}
                      </span>
                      {showAll && material.shared_with_students ? (
                        <HvBadge variant="success" size="sm">
                          Chia sẻ với học sinh
                        </HvBadge>
                      ) : null}
                    </li>
                  ))}
                </ul>
              );
            }}
          </LessonSections>
        )}
      </ProgramTabState>
    </div>
  );
}
