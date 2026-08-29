import { useState } from "react";
import { Navigate } from "react-router";
import { useQueries } from "@tanstack/react-query";

import { hvToast } from "@/components/hv";
import { listClassSessions, sessionsKeys } from "@/features/attendance";
import { currentMonth, useClassesList } from "@/features/roster";

import { getCurriculum, listPlans } from "../api/teaching-api";
import { PlanReviewPanel } from "../components/plan-review-panel";
import { ReviewQueueTable, type ReviewQueueRow } from "../components/review-queue-table";
import { useCenterContext } from "../hooks/use-center-context";
import { toLessonPlan } from "../hooks/use-class-teaching";
import { teachingKeys } from "../hooks/teaching-keys";
import { usePlanAction } from "../hooks/use-teaching-mutations";
import { nextLessonIndex } from "../lib/classbook-stats";
import { transitionLessonPlanStatus, type LessonPlanAction } from "../lib/teaching-store";

/**
 * Duyệt giáo án — review queue across all active classes, closing the loop
 * the teacher's classbook screen opens. Reading takes the
 * `teaching.review_queue` permission (the owner always holds it); the
 * approve/redo/reopen writes stay owner-only, so a grantee gets a read-only
 * panel. Nav hiding alone is not a guard: accounts without the permission
 * landing here (deep link, stale bookmark) are routed back to the classbook
 * once the permission resolves.
 */
export function LessonPlansPage() {
  const { centerId, isOwner, isResolved, isError, has } = useCenterContext();
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
  // Curricula and plans share the classbook's query keys, so a teacher tab on
  // the same client and this queue read one cache.
  const curriculumQueries = useQueries({
    queries: classes.map((klass) => ({
      queryKey: teachingKeys.curriculum(klass.id),
      queryFn: () => getCurriculum(klass.id),
    })),
  });
  const planQueries = useQueries({
    queries: classes.map((klass) => ({
      queryKey: teachingKeys.plans(klass.id),
      queryFn: () => listPlans(klass.id),
    })),
  });

  const queue = classes.map((klass, index) => {
    const sessions = sessionQueries[index]?.data ?? [];
    const doneCount = sessions.filter((session) => session.status === "held").length;
    const curriculumData = curriculumQueries[index]?.data;
    const curriculum =
      curriculumData && curriculumData.lessons.length > 0 ? curriculumData : undefined;
    const total = curriculum?.lessons.length ?? 0;
    const nextIndex = nextLessonIndex(total, doneCount);
    const wirePlan = (planQueries[index]?.data ?? []).find(
      (plan) => plan.lesson_index === nextIndex,
    );
    const plan = wirePlan ? toLessonPlan(wirePlan) : undefined;
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
    return { row, nextIndex, plan, lessonTitle, lessonNumber };
  });

  const pendingCount = queue.filter((entry) => entry.row.status === "pending").length;
  const selected =
    queue.find((entry) => entry.row.classId === selectedClassId) ??
    queue.find((entry) => entry.row.status === "pending") ??
    queue[0];

  const planActionMutation = usePlanAction(selected?.row.classId ?? "");

  /** Runs one review action after the shared transition table allows it; false = no-op. */
  function applyAction(action: LessonPlanAction, comment?: string) {
    if (!centerId || !selected?.plan) {
      return false;
    }
    // The server enforces the same table (409 on an illegal move); this gate
    // just avoids a doomed call from a stale click.
    if (transitionLessonPlanStatus(selected.plan.status, action) === null) {
      return false;
    }
    const wireAction = action === "requestRedo" ? "request-redo" : action;
    if (wireAction === "save" || wireAction === "submit") {
      return false;
    }
    planActionMutation.mutate({ lessonIndex: selected.nextIndex, action: wireAction, comment });
    return true;
  }

  function approve(comment: string) {
    if (selected && applyAction("approve", comment || undefined)) {
      hvToast(
        `Đã duyệt giáo án ${selected.row.className} — giáo viên thấy trạng thái trong sổ lớp`,
      );
    }
  }

  function requestRedo(comment: string) {
    if (!comment || !selected) {
      return;
    }
    if (applyAction("requestRedo", comment)) {
      hvToast(`Đã yêu cầu sửa giáo án ${selected.row.className} — ghi chú hiển thị trong sổ lớp`);
    }
  }

  function reopen() {
    applyAction("reopen");
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
  // Unresolvable permissions (query failed) degrade like no permission — a
  // redirect, never a permanently blank page.
  if (!has("teaching.review_queue")) {
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
              canAct={isOwner}
            />
          ) : null}
        </div>
      )}
    </div>
  );
}
