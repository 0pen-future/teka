import { PlusIcon, XIcon } from "lucide-react";
import { useState, type KeyboardEvent } from "react";

import { HvBadge, HvButton, HvSegmented, hvToast, type HvSegmentedOption } from "@/components/hv";
import { Input } from "@/components/ui/input";
import { useMemberDirectory } from "@/features/tasks";
import { useCenterContext } from "@/features/teaching";
import { formatDayMonth } from "@/lib/utils/format";

import { useUpdateLessonPrep } from "../hooks/use-prep";
import { prepStatusLabel, prepStatusVariant } from "../lib/library-labels";
import {
  PREP_STATUSES,
  type ChecklistItem,
  type PrepInput,
  type PrepStatus,
  type TemplateLesson,
} from "../schemas/library-schemas";

const MAX_CHECKLIST = 50;
const MAX_LABEL = 200;

const statusOptions: HvSegmentedOption<PrepStatus>[] = PREP_STATUSES.map((status) => ({
  value: status,
  label: prepStatusLabel[status],
}));

interface LessonPrepPanelProps {
  lesson: TemplateLesson;
  /** Draft version and `library.edit`: the status and checklist can be changed here. */
  editable: boolean;
}

/**
 * The lesson's preparation block: who prepares it and by when (set on the
 * assign page, read-only here), its prep status and the checklist of things
 * to get ready. Saved as one PATCH so a half-edited checklist never lands.
 */
export function LessonPrepPanel({ lesson, editable }: LessonPrepPanelProps) {
  return (
    <section
      aria-label="Chuẩn bị tài liệu"
      className="flex flex-col gap-4 rounded-[var(--radius-lg)] border border-line-200 bg-white p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <h2 className="font-display text-[17px] font-extrabold text-ink-900">Chuẩn bị tài liệu</h2>
        <AssignmentSummary assigneeId={lesson.assignee_id} dueDate={lesson.due_date} />
      </div>
      {editable ? <PrepEditor lesson={lesson} /> : <PrepReadOnly lesson={lesson} />}
    </section>
  );
}

function AssignmentSummary({
  assigneeId,
  dueDate,
}: {
  assigneeId: string | null;
  dueDate: string | null;
}) {
  const { has } = useCenterContext();
  const directory = useMemberDirectory(assigneeId !== null && has("members.list"));
  const assigneeName = directory.data?.find(
    (entry) => entry.teacher_id === assigneeId,
  )?.display_name;

  return (
    <dl className="flex flex-wrap gap-x-4 gap-y-1 text-[13px] text-ink-500">
      <div className="flex gap-1.5">
        <dt>Người phụ trách:</dt>
        <dd className="font-bold text-ink-700">
          {assigneeId === null ? "Chưa phân công" : (assigneeName ?? "Đã phân công")}
        </dd>
      </div>
      <div className="flex gap-1.5">
        <dt>Hạn:</dt>
        <dd className="font-bold text-ink-700">{dueDate ? formatDayMonth(dueDate) : "—"}</dd>
      </div>
    </dl>
  );
}

function PrepReadOnly({ lesson }: { lesson: TemplateLesson }) {
  return (
    <div className="flex flex-col gap-3">
      <div>
        <HvBadge variant={prepStatusVariant[lesson.prep_status]} dot>
          {prepStatusLabel[lesson.prep_status]}
        </HvBadge>
      </div>
      {lesson.checklist.length === 0 ? (
        <p className="text-[13px] text-ink-500">Chưa có việc nào</p>
      ) : (
        <ul className="flex flex-col gap-1.5 text-[14px]">
          {lesson.checklist.map((item, index) => (
            <li key={`${index}-${item.label}`} className="flex items-center gap-2">
              <input type="checkbox" checked={item.done} disabled aria-label={item.label} />
              <span className={item.done ? "text-ink-500 line-through" : "text-ink-900"}>
                {item.label}
              </span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function PrepEditor({ lesson }: { lesson: TemplateLesson }) {
  const [status, setStatus] = useState<PrepStatus>(lesson.prep_status);
  const [checklist, setChecklist] = useState<ChecklistItem[]>(lesson.checklist);
  const [draftLabel, setDraftLabel] = useState("");
  const mutation = useUpdateLessonPrep(lesson.id, lesson.version_id);
  // What the server last told us, so `save` can tell "the user touched this"
  // from "this still holds the value the page opened with". Sending a field
  // nobody edited would resend a value that may already be stale — e.g. a
  // board drag changed `prep_status` elsewhere while this panel was open.
  const [statusBaseline, setStatusBaseline] = useState(lesson.prep_status);
  const [checklistBaseline, setChecklistBaseline] = useState(lesson.checklist);

  const full = checklist.length >= MAX_CHECKLIST;
  const dirty = status !== statusBaseline || checklist !== checklistBaseline;

  function addItem() {
    const label = draftLabel.trim().slice(0, MAX_LABEL);
    if (!label || full) return;
    setChecklist((items) => [...items, { label, done: false }]);
    setDraftLabel("");
  }

  function onDraftKeyDown(event: KeyboardEvent<HTMLInputElement>) {
    if (event.key === "Enter") {
      event.preventDefault();
      addItem();
    }
  }

  function toggle(index: number, done: boolean) {
    setChecklist((items) => items.map((item, i) => (i === index ? { ...item, done } : item)));
  }

  function remove(index: number) {
    setChecklist((items) => items.filter((_, i) => i !== index));
  }

  function save() {
    const payload: PrepInput = {};
    if (status !== statusBaseline) {
      payload.prep_status = status;
    }
    if (checklist !== checklistBaseline) {
      payload.checklist = checklist;
    }
    if (payload.prep_status === undefined && payload.checklist === undefined) {
      return;
    }
    mutation.mutate(payload, {
      onSuccess: (saved) => {
        setStatus(saved.prep_status);
        setChecklist(saved.checklist);
        setStatusBaseline(saved.prep_status);
        setChecklistBaseline(saved.checklist);
        hvToast("Đã lưu trạng thái chuẩn bị");
      },
      onError: () => {
        hvToast("Không lưu được trạng thái chuẩn bị, vui lòng thử lại.", { variant: "danger" });
      },
    });
  }

  return (
    <div className="flex flex-col gap-4">
      <HvSegmented
        aria-label="Trạng thái chuẩn bị"
        options={statusOptions}
        value={status}
        onValueChange={setStatus}
      />

      <div className="flex flex-col gap-2">
        <p className="font-display text-[12px] font-extrabold uppercase tracking-[0.4px] text-ink-500">
          Việc cần làm ({checklist.filter((item) => item.done).length}/{checklist.length})
        </p>
        {checklist.length === 0 ? (
          <p className="text-[13px] text-ink-500">Chưa có việc nào</p>
        ) : (
          <ul className="flex flex-col gap-1.5">
            {checklist.map((item, index) => {
              const id = `prep-item-${lesson.id}-${index}`;
              return (
                <li key={id} className="flex items-center gap-2 text-[14px]">
                  <input
                    id={id}
                    type="checkbox"
                    checked={item.done}
                    onChange={(event) => toggle(index, event.target.checked)}
                    className="size-4 accent-mint-600"
                  />
                  <label
                    htmlFor={id}
                    className={
                      item.done ? "flex-1 text-ink-500 line-through" : "flex-1 text-ink-900"
                    }
                  >
                    {item.label}
                  </label>
                  <button
                    type="button"
                    aria-label={`Xoá ${item.label}`}
                    onClick={() => remove(index)}
                    className="inline-flex size-7 items-center justify-center rounded-full text-ink-400 hover:bg-cream-100 hover:text-coral-600"
                  >
                    <XIcon aria-hidden className="size-4" />
                  </button>
                </li>
              );
            })}
          </ul>
        )}
        <div className="flex items-center gap-2">
          <Input
            aria-label="Thêm việc cần làm"
            placeholder="VD: Soạn slide…"
            value={draftLabel}
            maxLength={MAX_LABEL}
            disabled={full}
            onChange={(event) => setDraftLabel(event.target.value)}
            onKeyDown={onDraftKeyDown}
          />
          <HvButton
            type="button"
            variant="secondary"
            size="sm"
            onClick={addItem}
            disabled={full || draftLabel.trim() === ""}
          >
            <PlusIcon aria-hidden className="size-4" />
            Thêm
          </HvButton>
        </div>
        {full ? (
          <p className="text-[12px] text-ink-500">Tối đa {MAX_CHECKLIST} việc mỗi buổi.</p>
        ) : null}
      </div>

      <div className="flex justify-end">
        <HvButton type="button" onClick={save} disabled={mutation.isPending || !dirty}>
          {mutation.isPending ? "Đang lưu…" : "Lưu chuẩn bị"}
        </HvButton>
      </div>
    </div>
  );
}
