import { useState } from "react";

import { hvToast } from "@/components/hv";
import type { Enrollment } from "@/features/roster";

import { useClassTeaching } from "../hooks/use-class-teaching";
import { usePlanAction, useSaveCurriculum, useSavePlan } from "../hooks/use-teaching-mutations";
import { monthlyHeadcount, nextLessonIndex, retentionStat } from "../lib/classbook-stats";
import { lessonPlanKey, transitionLessonPlanStatus } from "../lib/teaching-store";
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
  /** Whether the viewer may edit curriculum/giáo án — see `SessionDetailPanel`'s prop of the same name. */
  canWrite: boolean;
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
  canWrite,
}: CourseViewProps) {
  const { curriculum, lessonPlans } = useClassTeaching(classId);
  const saveCurriculumMutation = useSaveCurriculum(classId);
  const savePlanMutation = useSavePlan(classId);
  const planActionMutation = usePlanAction(classId);
  const [openModal, setOpenModal] = useState<"plan" | "curriculum" | null>(null);

  const total = curriculum?.lessons.length ?? 0;
  const done = Math.min(doneCount, total);
  const nextIndex = nextLessonIndex(total, done);
  const planKey = lessonPlanKey(classId, nextIndex);
  const plan = lessonPlans[planKey];
  const lessonTitle = curriculum?.lessons[nextIndex];
  const lessonLabel = `Bài ${nextIndex + 1}${lessonTitle ? ` · ${lessonTitle}` : ""}`;

  function savePlan(draft: PlanEditorDraft) {
    // A plan under or after review has no legal "save" — editing must go
    // through the owner (mở lại) so approved content can't drift silently.
    // The server enforces the same table; this gate just avoids a doomed call.
    const next = transitionLessonPlanStatus(plan?.status ?? "none", "save");
    if (!centerId || next === null) {
      return;
    }
    savePlanMutation.mutate({
      lessonIndex: nextIndex,
      input: {
        goal: draft.goal.trim(),
        activities: draft.activities
          .split("\n")
          .map((line) => line.trim())
          .filter(Boolean),
        homework: draft.homework.trim(),
        file_name: plan?.fileName ?? null,
      },
    });
    setOpenModal(null);
    hvToast(`Đã lưu giáo án ${classTitle} — nộp duyệt khi sẵn sàng`);
  }

  function attachFile(fileName: string) {
    const next = transitionLessonPlanStatus(plan?.status ?? "none", "save");
    if (!centerId || next === null) {
      return;
    }
    savePlanMutation.mutate({
      lessonIndex: nextIndex,
      input: {
        goal: plan?.goal ?? "",
        activities: plan?.activities ?? [],
        homework: plan?.homework ?? "",
        file_name: fileName,
      },
    });
  }

  function submitPlan() {
    if (!centerId || !plan || transitionLessonPlanStatus(plan.status, "submit") === null) {
      return;
    }
    // The server stamps submitted_by(_name) from the caller's token; the
    // owner's review queue reads it from the response.
    planActionMutation.mutate({ lessonIndex: nextIndex, action: "submit" });
    hvToast(`Đã nộp giáo án ${classTitle} — chờ chủ trung tâm duyệt`);
  }

  function saveCurriculum(lessons: string[]) {
    if (!centerId) {
      return;
    }
    // Clamp so removing lessons can never leave the pointer past the end;
    // the server clamps identically.
    const currentIndex = Math.max(0, Math.min(curriculum?.currentIndex ?? 0, lessons.length - 1));
    saveCurriculumMutation.mutate({ lessons, current_index: currentIndex });
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
        canWrite={canWrite}
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
            canWrite={canWrite}
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
