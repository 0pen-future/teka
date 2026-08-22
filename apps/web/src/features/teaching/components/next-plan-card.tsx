import type { LessonPlan } from "../lib/teaching-store";
import { PlanStatusPill } from "./plan-status-pill";

interface NextPlanCardProps {
  nextIndex: number;
  totalLessons: number;
  lessonTitle: string | undefined;
  plan: LessonPlan | undefined;
  onEdit: () => void;
  /** Called with the picked file's name — the UI stores the name only, no upload. */
  onAttachFile: (fileName: string) => void;
  onSubmit: () => void;
}

/**
 * GIÁO ÁN BUỔI TỚI card: status chip, the owner's redo note when changes were
 * requested, direct editing, file-name attach, and submit-for-review.
 */
export function NextPlanCard({
  nextIndex,
  totalLessons,
  lessonTitle,
  plan,
  onEdit,
  onAttachFile,
  onSubmit,
}: NextPlanCardProps) {
  const status = plan?.status ?? "none";
  const canSubmit = plan !== undefined && (status === "draft" || status === "redo");
  // The status machine has no "save" from pending/approved: content under or
  // after review is locked until the owner responds or reopens it.
  const canEdit = status !== "pending" && status !== "approved";

  return (
    <section className="rounded-[24px] bg-white px-5 py-[18px] shadow-soft-md">
      <div className="flex items-center gap-2">
        <div className="text-[12.5px] font-extrabold tracking-[0.4px] text-ink-400">
          GIÁO ÁN BUỔI TỚI
        </div>
        <PlanStatusPill status={status} />
      </div>
      <div className="mt-2 text-[14px] font-extrabold text-ink-900">
        Bài {nextIndex + 1}/{totalLessons}
        {lessonTitle ? ` · ${lessonTitle}` : ""}
      </div>
      {status === "redo" && plan?.redoNote ? (
        <div className="mt-2 rounded-xl bg-coral-100 px-3 py-2 text-[12.5px] text-coral-600">
          <b>Chủ trung tâm yêu cầu sửa:</b> {plan.redoNote}
        </div>
      ) : null}
      {plan?.fileName ? (
        <div className="mt-2 overflow-hidden rounded-[10px] bg-cream-100 px-2.5 py-1.5 text-[12.5px] whitespace-nowrap text-ellipsis text-ink-500">
          📎 {plan.fileName}
        </div>
      ) : null}
      {canEdit ? (
        <>
          <button
            type="button"
            onClick={onEdit}
            className="mt-2.5 w-full rounded-[14px] bg-sky-50 px-3.5 py-2.5 text-[13px] font-extrabold text-sky-600 hover:bg-sky-100"
          >
            ✎ Soạn giáo án trực tiếp
          </button>
          <label className="mt-2 block cursor-pointer rounded-[14px] border-2 border-dashed border-line-300 px-3.5 py-2 text-center text-[12.5px] font-extrabold text-ink-400 hover:border-sky-300 hover:bg-sky-50">
            hoặc đính kèm file Word/PDF
            <input
              type="file"
              accept=".doc,.docx,.pdf"
              className="hidden"
              onChange={(event) => {
                const file = event.target.files?.[0];
                if (file) {
                  onAttachFile(file.name);
                }
                event.target.value = "";
              }}
            />
          </label>
        </>
      ) : (
        <div className="mt-2.5 rounded-[14px] bg-cream-100 px-3.5 py-2.5 text-[12.5px] text-ink-500">
          {status === "pending"
            ? "Đã nộp duyệt — chờ chủ trung tâm phản hồi trước khi sửa."
            : "Đã duyệt — cần sửa thì nhờ chủ trung tâm mở lại để duyệt lại."}
        </div>
      )}
      {canSubmit ? (
        <button
          type="button"
          onClick={onSubmit}
          className="mt-2 w-full rounded-[14px] bg-mint-400 px-3.5 py-2.5 text-[13px] font-extrabold text-white shadow-press-mint transition-transform active:translate-y-[3px] active:shadow-none"
        >
          Nộp duyệt giáo án
        </button>
      ) : null}
    </section>
  );
}
