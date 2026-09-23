import { formatDifficulty } from "@/features/library";

import { useClassProgram, useClassProgramLessons } from "../hooks/use-class-program";
import type { Class } from "../schemas/roster-schemas";
import { LessonSections, ProgramTabState } from "./program-tab-state";

interface ClassHomeworkTabProps {
  klass: Class;
}

/** "Bài tập": the applied template's exercises, grouped per lesson. */
export function ClassHomeworkTab({ klass }: ClassHomeworkTabProps) {
  const program = useClassProgram(klass.id);
  const lessons = useClassProgramLessons(klass.id, Boolean(program.data));

  return (
    <ProgramTabState
      program={program}
      lessons={lessons}
      emptyDescription="Áp dụng chương trình ở tab Thông tin để xem bài tập theo buổi."
    >
      {(items) => (
        <LessonSections lessons={items}>
          {(lesson) =>
            lesson.exercises.length === 0 ? (
              <p className="text-[13px] text-ink-400">Chưa gắn bài tập</p>
            ) : (
              <ul className="flex flex-col gap-2">
                {lesson.exercises.map((exercise) => (
                  <li key={exercise.id} className="text-[14px]">
                    <p className="flex flex-wrap items-baseline gap-x-2">
                      <span className="font-bold text-ink-900">{exercise.title}</span>
                      <span className="text-[13px] text-ink-500">
                        {formatDifficulty(exercise.difficulty)}
                      </span>
                    </p>
                    {exercise.description ? (
                      <p className="text-[13px] text-ink-600">{exercise.description}</p>
                    ) : null}
                  </li>
                ))}
              </ul>
            )
          }
        </LessonSections>
      )}
    </ProgramTabState>
  );
}
