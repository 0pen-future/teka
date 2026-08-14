import { useState } from "react";
import { Navigate } from "react-router";
import { useQueries } from "@tanstack/react-query";

import { hvToast } from "@/components/hv";
import { listClassSessions, sessionsKeys } from "@/features/attendance";
import { currentMonth, useClassesList } from "@/features/roster";

import { PlanReviewPanel } from "../components/plan-review-panel";
import { ReviewQueueTable, type ReviewQueueRow } from "../components/review-queue-table";
import { useCenterContext } from "../hooks/use-center-context";
import { nextLessonIndex } from "../lib/classbook-stats";
import {
  lessonPlanKey,
  transitionLessonPlanStatus,
  updateTeachingState,
  useTeachingStore,
  type LessonPlan,
  type LessonPlanAction,
} from "../lib/teaching-store";

/**
 * Duyệt giáo án — owner-only review queue across all active classes, closing
 * the loop Phase 4's teacher screen opens. Nav hiding alone is not a guard:
 * non-owners landing here (deep link, stale bookmark) are routed back to the
 * classbook once the role resolves.
 */
export function LessonPlansPage() {
  const { centerId, isOwner, isResolved, isError } = useCenterContext();
  const teaching = useTeachingStore(centerId);
  const [selectedClassId, setSelectedClassId] = useState<string | null>(null);

  const month = currentMonth();
  const { data: classesPage } = useClassesList({ status: "active", per_page: 100 });
  const classes = classesPage?.items ?? [];

  // Held-session counts drive which lesson is "buổi tới" per class — the
  // same axis the teacher's next-plan card uses, cached under the same keys.
  const sessionQueries = useQueries({
    queries: classes.map((klass) => ({
      queryKey: sessionsKeys.list(klass.id, { from: month.from, to: month.to }),
      queryFn: () => listClassSessions(klass.id, { from: month.from, to: month.to }),
    })),
  });

  const queue = classes.map((klass, index) => {
    const sessions = sessionQueries[index]?.data ?? [];
    const doneCount = sessions.filter((session) => session.status === "held").length;
    const curriculum = teaching.curricula[klass.id];
    const total = curriculum?.lessons.length ?? 0;
    const nextIndex = nextLessonIndex(total, doneCount);
    const planKey = lessonPlanKey(klass.id, nextIndex);
    const plan = teaching.lessonPlans[planKey];
    const lessonTitle = curriculum?.lessons[nextIndex];
    const lessonNumber = curriculum ? `Bài ${nextIndex + 1}/${total}` : "";
    const row: ReviewQueueRow = {
      classId: klass.id,
      className: klass.name,
      teacher: plan?.submittedBy ?? "—",
      lessonLabel: curriculum
        ? `${lessonNumber}${lessonTitle ? ` · ${lessonTitle}` : ""}`
        : "Chưa có chương trình học",
      status: plan?.status ?? "none",
    };
    return { row, planKey, plan, lessonTitle, lessonNumber };
  });

  const pendingCount = queue.filter((entry) => entry.row.status === "pending").length;
  const selected =
    queue.find((entry) => entry.row.classId === selectedClassId) ??
    queue.find((entry) => entry.row.status === "pending") ??
    queue[0];

  /** Runs one review action through the shared transition table; false = no-op. */
  function applyAction(action: LessonPlanAction, mutate: (plan: LessonPlan) => LessonPlan) {
    if (!centerId || !selected?.plan) {
      return false;
    }
    const next = transitionLessonPlanStatus(selected.plan.status, action);
    if (next === null) {
      return false;
    }
    const { planKey } = selected;
    updateTeachingState(centerId, (state) => {
      const current = state.lessonPlans[planKey];
      if (!current) {
        return state;
      }
      return {
        ...state,
        lessonPlans: { ...state.lessonPlans, [planKey]: { ...mutate(current), status: next } },
      };
    });
    return true;
  }

  function approve(comment: string) {
    if (
      selected &&
      applyAction("approve", (plan) => ({ ...plan, ownerComment: comment || undefined }))
    ) {
      hvToast(
        `Đã duyệt giáo án ${selected.row.className} — giáo viên thấy trạng thái trong sổ lớp`,
      );
    }
  }

  function requestRedo(comment: string) {
    if (!comment || !selected) {
      return;
    }
    if (applyAction("requestRedo", (plan) => ({ ...plan, redoNote: comment }))) {
      hvToast(`Đã yêu cầu sửa giáo án ${selected.row.className} — ghi chú hiển thị trong sổ lớp`);
    }
  }

  function reopen() {
    applyAction("reopen", (plan) => ({ ...plan, redoNote: undefined, ownerComment: undefined }));
  }

  function remind() {
    if (!selected) {
      return;
    }
    // UI-only: no Zalo integration exists — the copy must not claim delivery.
    hvToast(`Đã tạo lời nhắc nộp giáo án ${selected.row.className} — chưa gửi Zalo tự động`);
  }

  if (!isResolved && !isError) {
    return null;
  }
  // Unresolvable role (query failed) degrades like non-owner — a redirect,
  // never a permanently blank page.
  if (!isOwner) {
    return <Navigate to="/classbook" replace />;
  }

  return (
    <div className="flex flex-col gap-[18px]">
      <header>
        <h1 className="font-display text-[26px] font-extrabold text-ink-900">Duyệt giáo án</h1>
        <p className="mt-1 text-[14px] text-ink-500">
          {pendingCount > 0
            ? `Còn ${pendingCount} giáo án buổi tới chờ duyệt — duyệt xong giáo viên mới lên lớp.`
            : "Không có giáo án nào chờ duyệt."}
        </p>
      </header>

      {queue.length === 0 ? (
        <div className="rounded-[24px] bg-white p-6 text-center text-[13px] text-ink-400 shadow-soft-md">
          Chưa có lớp nào đang hoạt động.
        </div>
      ) : (
        <div className="flex flex-wrap items-start gap-4">
          <ReviewQueueTable
            rows={queue.map((entry) => entry.row)}
            selectedClassId={selected?.row.classId}
            onSelect={setSelectedClassId}
          />
          {selected ? (
            <PlanReviewPanel
              // Selection change must reset the comment draft.
              key={selected.row.classId}
              classTitle={selected.row.className}
              teacher={selected.row.teacher}
              lessonNumber={selected.lessonNumber}
              lessonTitle={selected.lessonTitle}
              plan={selected.plan}
              onApprove={approve}
              onRequestRedo={requestRedo}
              onReopen={reopen}
              onRemind={remind}
            />
          ) : null}
        </div>
      )}
    </div>
  );
}
