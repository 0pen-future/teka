import { useState } from "react";

import { hvToast } from "@/components/hv";
import { useAuthStore } from "@/features/auth";
import type { Enrollment } from "@/features/roster";

import { monthlyHeadcount, nextLessonIndex, retentionStat } from "../lib/classbook-stats";
import {
  lessonPlanKey,
  transitionLessonPlanStatus,
  updateTeachingState,
  useTeachingStore,
  type LessonPlan,
} from "../lib/teaching-store";
import { CurriculumCard } from "./curriculum-card";
import { CurriculumEditorModal } from "./curriculum-editor-modal";
import { MonthlyHeadcountCard } from "./monthly-headcount-card";
import { NextPlanCard } from "./next-plan-card";
import { PlanEditorModal, type PlanEditorDraft } from "./plan-editor-modal";

interface CourseViewProps {
  centerId: string | null;
  classId: string;
  classTitle: string;
  /** Held sessions this month — how far the class is along the lesson axis. */
  doneCount: number;
  enrollments: Enrollment[];
  /** YYYY-MM-01 for the current month. */
  monthStart: string;
  monthNumber: number;
  previousMonthNumber: number;
}

/**
 * The classbook's "Chương trình & giáo án" view: curriculum progress, the
 * next session's giáo án (the teacher half of the review loop Phase 6's
 * owner screen closes), and the monthly headcount chart. The next-plan card
 * targets the same lesson index the sessions table assigns to the upcoming
 * session, so submitting here updates that row's chip immediately.
 */
export function CourseView({
  centerId,
  classId,
  classTitle,
  doneCount,
  enrollments,
  monthStart,
  monthNumber,
  previousMonthNumber,
}: CourseViewProps) {
  const teaching = useTeachingStore(centerId);
  const teacherName = useAuthStore((state) => state.user?.full_name);
  const [openModal, setOpenModal] = useState<"plan" | "curriculum" | null>(null);

  const curriculum = teaching.curricula[classId];
  const total = curriculum?.lessons.length ?? 0;
  const done = Math.min(doneCount, total);
  const nextIndex = nextLessonIndex(total, done);
  const planKey = lessonPlanKey(classId, nextIndex);
  const plan = teaching.lessonPlans[planKey];
  const lessonTitle = curriculum?.lessons[nextIndex];
  const lessonLabel = `Bài ${nextIndex + 1}${lessonTitle ? ` · ${lessonTitle}` : ""}`;

  function writePlan(recipe: (current: LessonPlan | undefined) => LessonPlan) {
    if (!centerId) {
      return;
    }
    updateTeachingState(centerId, (state) => ({
      ...state,
      lessonPlans: { ...state.lessonPlans, [planKey]: recipe(state.lessonPlans[planKey]) },
    }));
  }

  function savePlan(draft: PlanEditorDraft) {
    // A plan under or after review has no legal "save" — editing must go
    // through the owner (mở lại) so approved content can't drift silently.
    const next = transitionLessonPlanStatus(plan?.status ?? "none", "save");
    if (next === null) {
      return;
    }
    writePlan((current) => ({
      ...current,
      goal: draft.goal.trim(),
      activities: draft.activities
        .split("\n")
        .map((line) => line.trim())
        .filter(Boolean),
      homework: draft.homework.trim(),
      status: next,
    }));
    setOpenModal(null);
    hvToast(`Đã lưu giáo án ${classTitle} — nộp duyệt khi sẵn sàng`);
  }

  function attachFile(fileName: string) {
    const next = transitionLessonPlanStatus(plan?.status ?? "none", "save");
    if (next === null) {
      return;
    }
    writePlan((current) => ({
      goal: "",
      activities: [],
      homework: "",
      ...current,
      fileName,
      status: next,
    }));
  }

  function submitPlan() {
    if (!plan) {
      return;
    }
    const next = transitionLessonPlanStatus(plan.status, "submit");
    if (next === null) {
      return;
    }
    writePlan((current) => ({
      ...(current ?? plan),
      status: next,
      redoNote: undefined,
      // The owner's review queue shows who submitted; falls back to "—" there.
      submittedBy: teacherName,
    }));
    hvToast(`Đã nộp giáo án ${classTitle} — chờ chủ trung tâm duyệt`);
  }

  function saveCurriculum(lessons: string[]) {
    if (!centerId) {
      return;
    }
    updateTeachingState(centerId, (state) => {
      // Clamp so removing lessons can never leave the pointer past the end
      // (or below 0 — a negative index fails the schema and would drop the
      // whole stored state on next load).
      const currentIndex = Math.max(
        0,
        Math.min(state.curricula[classId]?.currentIndex ?? 0, lessons.length - 1),
      );
      return { ...state, curricula: { ...state.curricula, [classId]: { lessons, currentIndex } } };
    });
    setOpenModal(null);
    hvToast(`Đã lưu chương trình ${classTitle} — khóa ${lessons.length} buổi`);
  }

  return (
    <div className="flex flex-wrap items-start gap-3.5">
      <CurriculumCard
        classTitle={classTitle}
        curriculum={curriculum}
        doneCount={doneCount}
        onEdit={() => setOpenModal("curriculum")}
      />
      <div className="flex min-w-[270px] flex-1 flex-col gap-3.5">
        {curriculum ? (
          <NextPlanCard
            nextIndex={nextIndex}
            totalLessons={total}
            lessonTitle={lessonTitle}
            plan={plan}
            onEdit={() => setOpenModal("plan")}
            onAttachFile={attachFile}
            onSubmit={submitPlan}
          />
        ) : null}
        <MonthlyHeadcountCard
          history={monthlyHeadcount(enrollments, monthStart)}
          retention={retentionStat(enrollments, monthStart)}
          monthNumber={monthNumber}
          previousMonthNumber={previousMonthNumber}
        />
      </div>
      {openModal === "plan" ? (
        <PlanEditorModal
          lessonLabel={lessonLabel}
          initial={{
            goal: plan?.goal ?? "",
            activities: plan?.activities.join("\n") ?? "",
            homework: plan?.homework ?? "",
          }}
          onCancel={() => setOpenModal(null)}
          onSave={savePlan}
        />
      ) : null}
      {openModal === "curriculum" ? (
        <CurriculumEditorModal
          classTitle={classTitle}
          initial={curriculum?.lessons ?? Array.from({ length: 8 }, () => "")}
          onCancel={() => setOpenModal(null)}
          onSave={saveCurriculum}
        />
      ) : null}
    </div>
  );
}
