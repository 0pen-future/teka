import { useState } from "react";

import { HvButton, HvModal } from "@/components/hv";
import { cn } from "@/lib/utils";

export interface PlanEditorDraft {
  goal: string;
  /** One activity per line, split by the caller on save. */
  activities: string;
  homework: string;
}

interface PlanEditorModalProps {
  /** "Bài 3 · Tên bài" — the lesson this giáo án belongs to. */
  lessonLabel: string;
  initial: PlanEditorDraft;
  onCancel: () => void;
  onSave: (draft: PlanEditorDraft) => void;
}

const fieldClassName =
  "mt-1.5 w-full rounded-[14px] border-2 border-line-200 px-3 py-2.5 text-[13.5px] outline-none focus:border-mint-400";
const labelClassName = "block text-[13px] font-extrabold text-ink-500";

/** Mount only while open (the parent conditionally renders) so the draft resets per session. */
export function PlanEditorModal({ lessonLabel, initial, onCancel, onSave }: PlanEditorModalProps) {
  const [draft, setDraft] = useState(initial);

  return (
    <HvModal
      open
      onOpenChange={(open) => {
        if (!open) {
          onCancel();
        }
      }}
      title={`Soạn giáo án — ${lessonLabel}`}
      description="Giáo án này sẽ hiện đúng như vậy ở màn Duyệt giáo án của chủ trung tâm."
      className="sm:max-w-[580px]"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={onCancel}>
            Hủy
          </HvButton>
          <HvButton type="button" onClick={() => onSave(draft)}>
            Lưu giáo án
          </HvButton>
        </>
      }
    >
      <label htmlFor="plan-goal" className={labelClassName}>
        Mục tiêu buổi học
      </label>
      <textarea
        id="plan-goal"
        rows={2}
        value={draft.goal}
        onChange={(event) => setDraft((current) => ({ ...current, goal: event.target.value }))}
        className={cn(fieldClassName, "resize-y")}
      />
      <label htmlFor="plan-activities" className={cn(labelClassName, "mt-3")}>
        Hoạt động trên lớp <span className="font-bold text-ink-400">(mỗi dòng một hoạt động)</span>
      </label>
      <textarea
        id="plan-activities"
        rows={5}
        value={draft.activities}
        onChange={(event) =>
          setDraft((current) => ({ ...current, activities: event.target.value }))
        }
        className={cn(fieldClassName, "resize-y")}
      />
      <label htmlFor="plan-homework" className={cn(labelClassName, "mt-3")}>
        Bài tập về nhà
      </label>
      <input
        id="plan-homework"
        value={draft.homework}
        onChange={(event) => setDraft((current) => ({ ...current, homework: event.target.value }))}
        className={fieldClassName}
      />
    </HvModal>
  );
}
