import { useState } from "react";

import type { LessonPlan } from "../lib/teaching-store";
import { PlanStatusPill } from "./plan-status-pill";
import { PlanSummary } from "./plan-summary";

interface PlanReviewPanelProps {
  classTitle: string;
  /** Submitting teacher, or "—" when no submission carried a name yet. */
  teacher: string;
  /** "Bài 3/6" eyebrow; empty when the class has no curriculum. */
  lessonNumber: string;
  lessonTitle: string | undefined;
  plan: LessonPlan | undefined;
  onApprove: (comment: string) => void;
  onRequestRedo: (comment: string) => void;
  onReopen: () => void;
  onRemind: () => void;
  /**
   * Whether the viewer may approve/redo/reopen. The API's write gate is
   * owner-only, so a `teaching.review_queue` grantee gets a read-only panel —
   * rendering the buttons would only manufacture 403s.
   */
  canAct: boolean;
}

const statusSubtitles: Record<string, string> = {
  none: "Giáo án buổi tới · chưa nộp",
  draft: "Giáo án buổi tới · giáo viên đang soạn, chưa nộp",
};

/**
 * Owner's detail panel for one class's upcoming giáo án. The comment draft is
 * local; the parent keys this component by class id so it resets on
 * selection change. Yêu cầu sửa stays disabled until a comment exists — a
 * redo without a reason gives the teacher nothing to act on.
 */
export function PlanReviewPanel({
  classTitle,
  teacher,
  lessonNumber,
  lessonTitle,
  plan,
  onApprove,
  onRequestRedo,
  onReopen,
  onRemind,
  canAct,
}: PlanReviewPanelProps) {
  const [comment, setComment] = useState("");
  const status = plan?.status ?? "none";
  const notSubmitted = status === "none" || status === "draft";
  const hasComment = comment.trim().length > 0;

  return (
    <section className="min-w-[340px] flex-1 overflow-hidden rounded-[24px] bg-white shadow-soft-lg">
      <div className="bg-sky-300 px-[18px] py-[14px] text-white">
        <h2 className="font-display text-[17px] font-bold">
          {classTitle} — {teacher}
        </h2>
        <div className="text-[12.5px] opacity-95">
          {statusSubtitles[status] ?? "Giáo án buổi tới · đã nộp duyệt"}
        </div>
      </div>

      <div className="px-[18px] pt-4 pb-[18px]">
        <div className="flex items-center gap-2">
          {lessonNumber ? (
            <div className="text-[12px] font-extrabold tracking-[0.3px] text-ink-400">
              {lessonNumber}
            </div>
          ) : null}
          <PlanStatusPill status={status} />
        </div>
        {lessonTitle ? (
          <div className="mt-1 font-display text-[16px] font-bold text-ink-900">{lessonTitle}</div>
        ) : null}
        {plan ? <PlanSummary plan={plan} /> : null}
        {plan?.submittedBy ? (
          <div className="mt-2 text-[12px] font-bold text-sky-500">
            Soạn trực tiếp bởi {plan.submittedBy}
          </div>
        ) : null}
        {status === "redo" && plan?.redoNote ? (
          <div className="mt-3 rounded-xl bg-coral-100 px-3 py-[9px] text-[13px] text-coral-600">
            <b>Yêu cầu sửa:</b> {plan.redoNote}
          </div>
        ) : null}

        {notSubmitted ? (
          <>
            <div className="mt-3.5 rounded-[14px] bg-cream-100 px-3.5 py-3 text-[13px] text-ink-500">
              Chưa có giáo án để duyệt — giáo viên nộp trong màn Quản lý lớp học.
            </div>
            <button
              type="button"
              onClick={onRemind}
              className="mt-2.5 w-full rounded-[16px] border-2 border-sky-300 bg-white px-4 py-[11px] text-[14px] font-extrabold text-sky-500 hover:bg-sky-50 focus-visible:ring-4 focus-visible:outline-none"
            >
              Nhắc giáo viên nộp qua Zalo
            </button>
          </>
        ) : null}

        {status === "pending" && canAct ? (
          <>
            <label
              htmlFor="owner-review-comment"
              className="mt-3.5 block text-[12px] font-extrabold tracking-[0.3px] text-ink-400"
            >
              NHẬN XÉT CỦA CHỦ TRUNG TÂM
            </label>
            <textarea
              id="owner-review-comment"
              rows={2}
              value={comment}
              onChange={(event) => setComment(event.target.value)}
              placeholder="Góp ý về mục tiêu, thời lượng, độ khó…"
              className="mt-1.5 w-full resize-y rounded-[14px] border-2 border-line-200 px-3 py-2.5 text-[13.5px] outline-none focus:border-mint-400"
            />
            <div className="mt-3 flex gap-2.5">
              <button
                type="button"
                onClick={() => onApprove(comment.trim())}
                className="flex-1 rounded-[16px] bg-mint-400 px-4 py-[11px] text-[14px] font-extrabold text-white shadow-press-mint transition-transform active:translate-y-[3px] active:shadow-none"
              >
                Duyệt giáo án
              </button>
              <button
                type="button"
                disabled={!hasComment}
                onClick={() => onRequestRedo(comment.trim())}
                className="flex-1 rounded-[16px] border-2 border-coral-400 bg-white px-4 py-[11px] text-[14px] font-extrabold text-coral-600 hover:bg-coral-100 disabled:cursor-not-allowed disabled:border-line-200 disabled:text-ink-400"
              >
                Yêu cầu sửa
              </button>
            </div>
            {!hasComment ? (
              <p className="mt-1.5 text-[12px] text-ink-400">
                Ghi rõ cần sửa gì để giáo viên biết đường sửa.
              </p>
            ) : null}
          </>
        ) : null}

        {(status === "approved" || status === "redo") && canAct ? (
          <button
            type="button"
            onClick={onReopen}
            className="mt-3 p-0 text-[13px] font-extrabold text-sky-500 hover:text-sky-600"
          >
            Mở lại để duyệt lại
          </button>
        ) : null}
      </div>
    </section>
  );
}
