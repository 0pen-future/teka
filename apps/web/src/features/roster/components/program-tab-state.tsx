import type { UseQueryResult } from "@tanstack/react-query";

import { HvStateBlock } from "@/components/hv";

import type { ClassLesson, ClassProgram } from "../schemas/class-program-schemas";

interface ProgramTabStateProps {
  program: UseQueryResult<ClassProgram | null>;
  lessons: UseQueryResult<ClassLesson[]>;
  emptyDescription: string;
  children: (lessons: ClassLesson[]) => React.ReactNode;
}

/** Shared loading / error / no-program gate for the tabs that read through the applied template. */
export function ProgramTabState({
  program,
  lessons,
  emptyDescription,
  children,
}: ProgramTabStateProps) {
  if (program.isPending || (program.data && lessons.isPending)) {
    return <HvStateBlock state="loading" title="Đang tải chương trình" />;
  }
  if (program.isError || lessons.isError) {
    return <HvStateBlock state="error" title="Không tải được chương trình của lớp" />;
  }
  if (!program.data) {
    return (
      <HvStateBlock
        state="empty"
        title="Lớp chưa áp dụng chương trình mẫu"
        description={emptyDescription}
      />
    );
  }
  return <>{children(lessons.data ?? [])}</>;
}

/** One titled section per template lesson, in position order. */
export function LessonSections({
  lessons,
  children,
}: {
  lessons: ClassLesson[];
  children: (lesson: ClassLesson) => React.ReactNode;
}) {
  const ordered = [...lessons].sort((a, b) => a.position - b.position);
  return (
    <div className="flex flex-col gap-3">
      {ordered.map((lesson) => (
        <section
          key={lesson.id}
          className="rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
        >
          <h3 className="font-display text-[15px] font-bold text-ink-900">
            Buổi {lesson.position} · {lesson.title}
          </h3>
          <div className="mt-2">{children(lesson)}</div>
        </section>
      ))}
    </div>
  );
}
