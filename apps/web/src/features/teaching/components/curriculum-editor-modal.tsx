import { useState } from "react";

import { HvButton, HvModal, hvToast } from "@/components/hv";

interface CurriculumEditorModalProps {
  classTitle: string;
  /** Current lesson titles, or blank seed rows when the class has no curriculum yet. */
  initial: string[];
  onCancel: () => void;
  /** Receives the trimmed, non-empty lesson list (already ≥ 4 items). */
  onSave: (lessons: string[]) => void;
}

/** Mount only while open (the parent conditionally renders) so rows reset per session. */
export function CurriculumEditorModal({
  classTitle,
  initial,
  onCancel,
  onSave,
}: CurriculumEditorModalProps) {
  const [rows, setRows] = useState(initial);

  function save() {
    const lessons = rows.map((title) => title.trim()).filter(Boolean);
    if (lessons.length < 4) {
      hvToast("Chương trình cần ít nhất 4 buổi");
      return;
    }
    onSave(lessons);
  }

  return (
    <HvModal
      open
      onOpenChange={(open) => {
        if (!open) {
          onCancel();
        }
      }}
      title={`Chương trình — ${classTitle}`}
      description="Sửa tên bài, thêm hoặc bớt buổi — số bài quyết định độ dài khóa."
      className="sm:max-w-[600px]"
      footer={
        <>
          <HvButton type="button" variant="ghost" onClick={onCancel}>
            Hủy
          </HvButton>
          <HvButton type="button" onClick={save}>
            Lưu chương trình
          </HvButton>
        </>
      }
    >
      <div className="flex max-h-[52vh] flex-col gap-1.5 overflow-y-auto">
        {rows.map((title, index) => (
          // Rows are positional (add/remove shifts them) — the index is the identity.
          <div key={index} className="flex items-center gap-2">
            <span className="w-6 text-right text-[12px] font-extrabold text-ink-400">
              {String(index + 1).padStart(2, "0")}
            </span>
            <input
              value={title}
              aria-label={`Bài ${index + 1}`}
              onChange={(event) =>
                setRows((current) =>
                  current.map((row, i) => (i === index ? event.target.value : row)),
                )
              }
              className="flex-1 rounded-xl border-2 border-line-200 px-2.5 py-[7px] text-[13.5px] outline-none focus:border-mint-400"
            />
            <button
              type="button"
              aria-label={`Xóa bài ${index + 1}`}
              onClick={() => setRows((current) => current.filter((_, i) => i !== index))}
              className="p-1 text-[13px] font-extrabold text-coral-400 hover:text-coral-600"
            >
              ✕
            </button>
          </div>
        ))}
        <button
          type="button"
          onClick={() => setRows((current) => [...current, "Bài mới — nhập tên bài"])}
          className="rounded-[14px] border-2 border-dashed border-line-300 px-3.5 py-2 text-[12.5px] font-extrabold text-mint-600 hover:border-mint-400 hover:bg-mint-50"
        >
          + Thêm bài
        </button>
      </div>
    </HvModal>
  );
}
