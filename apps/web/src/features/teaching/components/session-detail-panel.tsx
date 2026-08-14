import { useState } from "react";

import { useSessionRoster } from "@/features/attendance";

import { hvToast } from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import type { SessionDerived } from "../lib/classbook-stats";
import { parseScoreInput } from "../lib/classbook-stats";
import { useClassMarks } from "../hooks/use-class-marks";
import { useClassTeaching } from "../hooks/use-class-teaching";
import { useSaveMarks, useSaveSessionNote } from "../hooks/use-teaching-mutations";
import { lessonPlanKey } from "../lib/teaching-store";
import type { MarkEntryInput } from "../schemas/teaching-schemas";
import { vnd } from "../lib/vnd";
import { PlanStatusPill } from "./plan-status-pill";
import { PlanSummary } from "./plan-summary";

interface SessionDetailPanelProps {
  centerId: string | null;
  classId: string;
  classTitle: string;
  derived: SessionDerived;
  onClose: () => void;
}

type DetailTab = "note" | "plan" | "scores";

const detailTabs: { id: DetailTab; label: string }[] = [
  { id: "note", label: "Nhận xét" },
  { id: "plan", label: "Giáo án" },
  { id: "scores", label: "Điểm buổi" },
];

const saveButtonActive =
  "cursor-pointer rounded-[14px] bg-mint-400 px-5 py-[9px] text-[13px] font-extrabold text-white shadow-press-mint transition-transform active:translate-y-[3px] active:shadow-none";
const saveButtonIdle =
  "cursor-default rounded-[14px] bg-cream-200 px-5 py-[9px] text-[13px] font-extrabold text-ink-400";

/**
 * Slide-in card for one session: whole-class note, giáo án read view, and
 * per-student scores. Notes and scores read from the month batch and save
 * through optimistic mutations, so a save reflects in the same render; the
 * parent keys this component by session id so drafts and the active tab
 * reset when the selection changes.
 */
export function SessionDetailPanel({
  centerId,
  classId,
  classTitle,
  derived,
  onClose,
}: SessionDetailPanelProps) {
  const { session } = derived;
  // The sessions list is month-scoped, so this key always joins the
  // classbook page's batch read instead of spawning a second cache entry.
  const month = session.session_date.slice(0, 7);
  const { curriculum, lessonPlans } = useClassTeaching(classId);
  const classMarks = useClassMarks(classId, month);
  const saveNoteMutation = useSaveSessionNote(classId, month);
  const saveMarksMutation = useSaveMarks(classId, month);
  const rosterQuery = useSessionRoster(session.id);

  const [tab, setTab] = useState<DetailTab>("note");
  const [noteDraft, setNoteDraft] = useState<string | null>(null);
  const [scoreDraft, setScoreDraft] = useState<Record<string, string>>({});

  const sessionLabel = formatSessionDate(session.session_date);
  const held = session.status === "held";

  const storedNote = classMarks.sessionNotes[session.id]?.text ?? "";
  const noteValue = noteDraft ?? storedNote;
  const noteDirty = noteDraft !== null && noteDraft !== storedNote;

  const storedScores = classMarks.sessionScores[session.id] ?? {};
  const scoresDirty = Object.keys(scoreDraft).length > 0;

  const plan =
    derived.lessonIndex === null
      ? undefined
      : lessonPlans[lessonPlanKey(classId, derived.lessonIndex)];
  const lessonTitle =
    derived.lessonIndex === null ? undefined : curriculum?.lessons[derived.lessonIndex];

  const subtitle =
    session.status === "planned"
      ? "Chưa điểm danh"
      : session.status === "cancelled"
        ? "Buổi hủy — không tính phí"
        : "";

  const chips =
    held && derived.present !== null
      ? [
          `${derived.present}/${derived.eligible} có mặt`,
          `Điểm TB ${derived.average === null ? "—" : derived.average.toFixed(1)}`,
          `${(derived.net ?? 0) >= 0 ? "Lãi" : "Lỗ"} ${vnd(Math.abs(derived.net ?? 0))}`,
        ]
      : [];

  // Draft reset and the success toast wait for the server: on failure the
  // mutation's onError reverts the cache, and the still-held draft keeps the
  // user's text editable for a retry instead of silently discarding it.
  function saveNote() {
    if (!centerId || !noteDirty || noteDraft === null) {
      return;
    }
    saveNoteMutation.mutate(
      { sessionId: session.id, body: noteDraft },
      {
        onSuccess: () => {
          setNoteDraft(null);
          hvToast(`Đã lưu nhận xét buổi ${sessionLabel} — ${classTitle}`);
        },
      },
    );
  }

  function saveScores() {
    if (!centerId || !scoresDirty) {
      return;
    }
    const entries: MarkEntryInput[] = [];
    for (const [studentId, raw] of Object.entries(scoreDraft)) {
      const score = parseScoreInput(raw);
      if (score !== null) {
        // Score-only entry: the tri-state batch leaves the student's
        // personal note untouched because the key is simply absent.
        entries.push({ student_id: studentId, score });
      }
    }
    if (entries.length === 0) {
      // Nothing parseable to send — just drop the unusable draft.
      setScoreDraft({});
      return;
    }
    saveMarksMutation.mutate(
      { sessionId: session.id, entries },
      {
        onSuccess: () => {
          setScoreDraft({});
          hvToast(`Đã lưu điểm ${entries.length} học sinh — buổi ${sessionLabel}`);
        },
      },
    );
  }

  return (
    <section className="min-w-[320px] flex-1 overflow-hidden rounded-[24px] bg-white shadow-soft-lg">
      <div className="flex items-start gap-2.5 bg-mint-400 px-[18px] py-[14px] text-white">
        <div className="flex-1">
          <h3 className="font-display text-[17px] font-bold">
            Buổi {sessionLabel} — {classTitle}
          </h3>
          {subtitle ? <div className="text-[12.5px] opacity-[.92]">{subtitle}</div> : null}
          {chips.length > 0 ? (
            <div className="mt-1.5 flex flex-wrap gap-1.5">
              {chips.map((chip) => (
                <span
                  key={chip}
                  className="rounded-full bg-white/[.22] px-[11px] py-1 text-[12px] font-extrabold"
                >
                  {chip}
                </span>
              ))}
            </div>
          ) : null}
        </div>
        <button
          type="button"
          onClick={onClose}
          aria-label="Đóng chi tiết buổi"
          className="h-[26px] w-[26px] rounded-full bg-white/20 font-extrabold text-white"
        >
          ✕
        </button>
      </div>

      <div
        role="tablist"
        aria-label="Chi tiết buổi học"
        className="mx-3.5 mt-3 flex gap-1 rounded-[14px] bg-cream-200 p-1"
      >
        {detailTabs.map((item) => (
          <button
            key={item.id}
            type="button"
            role="tab"
            aria-selected={tab === item.id}
            onClick={() => setTab(item.id)}
            className={cn(
              "flex-1 rounded-xl px-1.5 py-2 text-[13px] font-extrabold focus-visible:ring-4 focus-visible:outline-none",
              tab === item.id ? "bg-white text-mint-600 shadow-soft-sm" : "text-ink-400",
            )}
          >
            {item.label}
          </button>
        ))}
      </div>

      <div className="px-[18px] pt-3.5 pb-4">
        {tab === "note" ? (
          <>
            <label
              htmlFor={`session-note-${session.id}`}
              className="block text-[12px] font-extrabold tracking-[0.3px] text-ink-400"
            >
              NHẬN XÉT CHUNG CỦA BUỔI
            </label>
            <textarea
              id={`session-note-${session.id}`}
              rows={5}
              value={noteValue}
              onChange={(event) => setNoteDraft(event.target.value)}
              placeholder="Không khí lớp, phần yếu cần lưu ý cho họp tuần…"
              className="mt-1.5 w-full resize-y rounded-[14px] border-2 border-line-200 px-3 py-2.5 text-[13.5px] outline-none focus:border-mint-400"
            />
            <div className="mt-2 flex items-center gap-2.5">
              <button
                type="button"
                onClick={saveNote}
                className={noteDirty ? saveButtonActive : saveButtonIdle}
              >
                Lưu nhận xét
              </button>
              <span
                className={cn(
                  "text-[12.5px] font-bold",
                  noteDirty ? "text-sun-600" : "text-mint-600",
                )}
              >
                {noteDirty ? "Chưa lưu" : storedNote.trim() ? "Đã lưu ✓" : ""}
              </span>
            </div>
          </>
        ) : null}

        {tab === "plan" ? (
          derived.lessonIndex === null ? (
            <p className="text-[13px] text-ink-500">Buổi hủy — không có giáo án.</p>
          ) : (
            <>
              <div className="flex items-center gap-2">
                <div className="text-[12px] font-extrabold tracking-[0.3px] text-ink-400">
                  Giáo án — Bài {derived.lessonIndex + 1}
                  {curriculum ? `/${curriculum.lessons.length}` : ""}
                </div>
                <PlanStatusPill status={plan?.status ?? "none"} />
              </div>
              {lessonTitle ? (
                <div className="mt-1 font-display text-[16px] font-bold text-ink-900">
                  {lessonTitle}
                </div>
              ) : null}
              {plan ? (
                <PlanSummary plan={plan} />
              ) : (
                <p className="mt-2 text-[13px] text-ink-500">
                  Chưa có giáo án cho buổi này — soạn ở tab Chương trình &amp; giáo án.
                </p>
              )}
            </>
          )
        ) : null}

        {tab === "scores" ? (
          <>
            <div className="mb-1.5 text-[12px] text-ink-400">
              Chấm điểm kiểm tra cuối buổi (0–10) rồi bấm lưu.
            </div>
            {rosterQuery.isPending ? (
              <p className="text-[13px] text-ink-500">Đang tải danh sách học sinh…</p>
            ) : rosterQuery.isError ? (
              <p className="text-[13px] text-coral-600">Không tải được danh sách học sinh.</p>
            ) : (
              <div className="flex max-h-[280px] flex-col gap-1 overflow-y-auto">
                {rosterQuery.data.rows.map((row) => {
                  const absent = row.status === "absent" || row.status === "excused";
                  const editable = held && row.status === "present";
                  const stored = storedScores[row.student_id];
                  return (
                    <div
                      key={row.student_id}
                      className="flex items-center gap-2.5 rounded-[10px] px-2 py-1 hover:bg-cream-100"
                    >
                      <span className="flex-1 text-[13.5px] font-bold text-ink-700">
                        {row.student_name}
                      </span>
                      {editable ? (
                        <input
                          type="number"
                          min={0}
                          max={10}
                          step={0.5}
                          aria-label={`Điểm ${row.student_name}`}
                          value={scoreDraft[row.student_id] ?? (stored?.toString() ?? "")}
                          onChange={(event) =>
                            setScoreDraft((draft) => ({
                              ...draft,
                              [row.student_id]: event.target.value,
                            }))
                          }
                          className="w-16 rounded-[10px] border-2 border-line-200 px-2 py-[5px] text-center text-[13.5px] font-extrabold text-ink-900 outline-none focus:border-mint-400"
                        />
                      ) : (
                        <span
                          className={cn(
                            "ml-auto rounded-full px-2.5 py-[3px] text-[13px] font-extrabold",
                            absent && held
                              ? "bg-coral-100 text-coral-600"
                              : "bg-cream-200 text-ink-400",
                          )}
                        >
                          {!held ? "—" : absent ? "Vắng" : "—"}
                        </span>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
            <div className="mt-2.5 flex items-center gap-2.5">
              <button
                type="button"
                onClick={saveScores}
                className={scoresDirty ? saveButtonActive : saveButtonIdle}
              >
                Lưu điểm buổi
              </button>
              <span className="text-[12.5px] font-bold text-sun-600">
                {scoresDirty ? "Chưa lưu" : ""}
              </span>
            </div>
          </>
        ) : null}
      </div>
    </section>
  );
}
