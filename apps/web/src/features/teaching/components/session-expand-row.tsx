import * as React from "react";
import { useCallback, useEffect, useImperativeHandle, useState } from "react";

import { useSessionRoster } from "@/features/attendance";

import {
  HvButton,
  HvIcon,
  HvScoreInput,
  HvStateBlock,
  hvToast,
  parseScoreInput,
} from "@/components/hv";
import { cn, formatSessionDate } from "@/lib/utils";

import type { SessionDerived } from "../lib/classbook-stats";
import { useClassMarks } from "../hooks/use-class-marks";
import { useClassScoreComponents } from "../hooks/use-component-scores";
import { useClassTeaching } from "../hooks/use-class-teaching";
import { useSaveMarks, useSaveSessionNote } from "../hooks/use-teaching-mutations";
import { lessonPlanKey } from "../lib/teaching-store";
import type { MarkEntryInput } from "../schemas/teaching-schemas";
import { PlanStatusPill } from "./plan-status-pill";
import { PlanSummary } from "./plan-summary";
import { ScoreEntryByStudent, type ScoreEntryHandle } from "./score-entry-by-student";

interface SessionExpandRowProps {
  centerId: string | null;
  classId: string;
  classTitle: string;
  derived: SessionDerived;
  /**
   * Whether the viewer may edit this session's note/scores. The API's write
   * gate is owner-or-giao_vien-only, so a hoc_vu/tro_giang class-staff
   * grantee gets a read-only row — rendering the inputs would only
   * manufacture 403s.
   */
  canWrite: boolean;
  /** Close request from the footer button; the page routes it through the unsaved guard. */
  onClose: () => void;
  /** Fires with the unsaved field count (score cells + note) whenever it changes. */
  onDirtyChange?: (dirtyCount: number, invalidCount: number) => void;
}

export interface SessionExpandRowHandle {
  /** Send unsaved scores and note now; resolves `true` once nothing is left. */
  flush: () => Promise<boolean>;
  /** Drop unsaved scores and note, including a pending autosave. */
  discard: () => void;
}

const blockLabelClassName = "text-[12px] font-extrabold tracking-[0.3px] text-ink-400";

function Kbd({ children }: { children: React.ReactNode }) {
  return (
    <kbd className="rounded-[6px] border border-line-300 bg-cream-200 px-1.5 py-px font-mono text-[11px] text-ink-500">
      {children}
    </kbd>
  );
}

/**
 * The open session's detail, rendered inside the ledger as a row spanning
 * every column: whole-class note, giáo án read view, and per-student scores
 * side by side. Notes and scores read from the month batch and save through
 * optimistic mutations, so a save reflects in the same render; the page keys
 * this component by session id so drafts reset when the selection changes.
 */
export const SessionExpandRow = React.forwardRef<SessionExpandRowHandle, SessionExpandRowProps>(
  function SessionExpandRow(
    { centerId, classId, classTitle, derived, canWrite, onClose, onDirtyChange },
    ref,
  ) {
    const { session } = derived;
    // The sessions list is month-scoped, so this key always joins the
    // classbook page's batch read instead of spawning a second cache entry.
    const month = session.session_date.slice(0, 7);
    const { curriculum, lessonPlans } = useClassTeaching(classId);
    const classMarks = useClassMarks(classId, month);
    const saveNoteMutation = useSaveSessionNote(classId, month);
    const saveMarksMutation = useSaveMarks(classId, month);
    const rosterQuery = useSessionRoster(session.id);
    // Empty (or still loading) components means the plain general-score list
    // stays — the per-student entry only replaces it once the class is
    // actually configured with components.
    const scoreComponentsQuery = useClassScoreComponents(classId);
    const components = scoreComponentsQuery.data?.components ?? [];
    const hasScoreComponents = components.length > 0;

    const [noteDraft, setNoteDraft] = useState<string | null>(null);
    const [scoreDraft, setScoreDraft] = useState<Record<string, string>>({});
    // Students whose typed text cannot be read as a score; saving stays
    // blocked until every one of them is fixed or cleared.
    const [invalidIds, setInvalidIds] = useState<Set<string>>(() => new Set());
    // Component-score draft bookkeeping lives in `ScoreEntryByStudent`; the
    // row only mirrors its unsaved count for the footer and the page guard.
    const entryRef = React.useRef<ScoreEntryHandle>(null);
    const [scoresDirtyCount, setScoresDirtyCount] = useState(0);
    const [scoresInvalidCount, setScoresInvalidCount] = useState(0);
    const handleEntryDirtyChange = useCallback((dirty: number, invalid: number) => {
      setScoresDirtyCount(dirty);
      setScoresInvalidCount(invalid);
    }, []);

    const sessionLabel = formatSessionDate(session.session_date);
    const held = session.status === "held";

    const storedNote = classMarks.sessionNotes[session.id]?.text ?? "";
    const noteValue = noteDraft ?? storedNote;
    const noteDirty = noteDraft !== null && noteDraft !== storedNote;

    const storedScores = classMarks.sessionScores[session.id] ?? {};
    const generalDirtyCount = Object.keys(scoreDraft).length;
    const scoresDirty = generalDirtyCount > 0;
    const hasInvalidScores = invalidIds.size > 0;

    // The note counts as one unsaved field so the page guard covers it too.
    const dirtyCount =
      (hasScoreComponents ? scoresDirtyCount : generalDirtyCount) + (noteDirty ? 1 : 0);
    const invalidCount = hasScoreComponents ? scoresInvalidCount : invalidIds.size;

    useEffect(() => {
      onDirtyChange?.(dirtyCount, invalidCount);
      // A row that vanishes outside the guarded navigation (a refetch dropped
      // the session) must not leave a phantom count behind on the page.
      return () => onDirtyChange?.(0, 0);
    }, [dirtyCount, invalidCount, onDirtyChange]);

    useEffect(() => {
      if (dirtyCount === 0) return;
      const warn = (event: BeforeUnloadEvent) => {
        event.preventDefault();
      };
      window.addEventListener("beforeunload", warn);
      return () => window.removeEventListener("beforeunload", warn);
    }, [dirtyCount]);

    const plan =
      derived.lessonIndex === null
        ? undefined
        : lessonPlans[lessonPlanKey(classId, derived.lessonIndex)];
    const lessonTitle =
      derived.lessonIndex === null ? undefined : curriculum?.lessons[derived.lessonIndex];

    // Draft reset and the success toast wait for the server: on failure the
    // mutation's onError reverts the cache and toasts, and the still-held
    // draft keeps the user's text editable for a retry instead of silently
    // discarding it. Resolves `false` while something is still unsaved.
    async function saveNoteNow(): Promise<boolean> {
      if (!noteDirty || noteDraft === null) return true;
      if (!centerId) return false;
      try {
        await saveNoteMutation.mutateAsync({ sessionId: session.id, body: noteDraft });
      } catch {
        return false;
      }
      setNoteDraft(null);
      hvToast(`Đã lưu nhận xét buổi ${sessionLabel} — ${classTitle}`);
      return true;
    }

    function commitScore(studentId: string, parsed: ReturnType<typeof parseScoreInput>) {
      setInvalidIds((current) => {
        const invalid = parsed === "invalid";
        if (invalid === current.has(studentId)) return current;
        const next = new Set(current);
        if (invalid) next.add(studentId);
        else next.delete(studentId);
        return next;
      });
    }

    async function saveGeneralScores(): Promise<boolean> {
      if (!scoresDirty) return true;
      if (!centerId || hasInvalidScores) return false;
      const entries: MarkEntryInput[] = [];
      for (const [studentId, raw] of Object.entries(scoreDraft)) {
        const score = parseScoreInput(raw);
        if (typeof score === "number") {
          // Score-only entry: the tri-state batch leaves the student's
          // personal note untouched because the key is simply absent.
          entries.push({ student_id: studentId, score });
        }
      }
      if (entries.length === 0) {
        // Nothing parseable to send — just drop the unusable draft.
        setScoreDraft({});
        return true;
      }
      try {
        await saveMarksMutation.mutateAsync({ sessionId: session.id, entries });
      } catch {
        return false;
      }
      setScoreDraft({});
      hvToast(`Đã lưu điểm ${entries.length} học sinh — buổi ${sessionLabel}`);
      return true;
    }

    // Rebuilt every render on purpose: `flush` must see the current drafts.
    // The by-student block owns component drafts; the row owns the general
    // scores and the note, and "Lưu và đóng" has to cover all of them.
    useImperativeHandle(ref, () => ({
      flush: async () => {
        const scoresSaved = entryRef.current
          ? await entryRef.current.flush()
          : await saveGeneralScores();
        const noteSaved = await saveNoteNow();
        return scoresSaved && noteSaved;
      },
      discard: () => {
        entryRef.current?.discard();
        setNoteDraft(null);
        setScoreDraft({});
        setInvalidIds(new Set());
        setScoresDirtyCount(0);
        setScoresInvalidCount(0);
      },
    }));

    const noteBlock = (
      <div>
        {canWrite ? (
          <>
            <label htmlFor={`session-note-${session.id}`} className={blockLabelClassName}>
              NHẬN XÉT CHUNG CỦA BUỔI
            </label>
            <textarea
              id={`session-note-${session.id}`}
              rows={5}
              value={noteValue}
              onChange={(event) => setNoteDraft(event.target.value)}
              placeholder="Không khí lớp, phần yếu cần lưu ý cho họp tuần…"
              className="mt-1.5 w-full resize-y rounded-[14px] border-2 border-line-200 bg-white px-3 py-2.5 text-[13.5px] outline-none focus:border-mint-400"
            />
            <div className="mt-2 flex items-center gap-2.5">
              <HvButton
                type="button"
                variant="primary"
                size="sm"
                onClick={() => void saveNoteNow()}
                disabled={!noteDirty || saveNoteMutation.isPending}
              >
                Lưu nhận xét
              </HvButton>
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
        ) : (
          <>
            <div className={blockLabelClassName}>NHẬN XÉT CHUNG CỦA BUỔI</div>
            <p className="mt-1.5 text-[13.5px] text-ink-700">
              {storedNote.trim() || "Chưa có nhận xét."}
            </p>
          </>
        )}
      </div>
    );

    const planBlock = (
      <div>
        {derived.lessonIndex === null ? (
          <>
            <div className={blockLabelClassName}>GIÁO ÁN</div>
            <p className="mt-1.5 text-[13px] text-ink-500">Buổi hủy — không có giáo án.</p>
          </>
        ) : (
          <>
            <div className="flex flex-wrap items-center gap-2">
              <div className={blockLabelClassName}>
                GIÁO ÁN · BÀI {derived.lessonIndex + 1}
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
        )}
      </div>
    );

    const scoresBlock = (
      <div role="group" aria-label="Điểm buổi">
        <div className={blockLabelClassName}>
          ĐIỂM BUỔI{hasScoreComponents ? ` · ${components.length} ĐẦU ĐIỂM` : ""}
        </div>
        <div className="mt-1.5">
          {hasScoreComponents ? (
            <ScoreEntryByStudent
              ref={entryRef}
              sessionId={session.id}
              held={held}
              rosterRows={rosterQuery.data?.rows ?? []}
              rosterPending={rosterQuery.isPending}
              rosterError={rosterQuery.isError}
              sessionLabel={sessionLabel}
              canWrite={canWrite}
              onDirtyChange={handleEntryDirtyChange}
            />
          ) : (
            <>
              <div className="mb-1.5 text-[12px] text-ink-400">
                Chấm điểm kiểm tra cuối buổi (0–10) rồi bấm lưu.
              </div>
              {rosterQuery.isPending ? (
                <HvStateBlock state="loading" compact title="Đang tải danh sách học sinh…" />
              ) : rosterQuery.isError ? (
                <HvStateBlock state="error" compact title="Không tải được danh sách học sinh." />
              ) : (
                <div className="flex max-h-[280px] flex-col gap-1 overflow-y-auto">
                  {rosterQuery.data.rows.map((row) => {
                    const absent = row.status === "absent" || row.status === "excused";
                    const editable = held && row.status === "present" && canWrite;
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
                          <div className="w-24 shrink-0">
                            <HvScoreInput
                              size="sm"
                              aria-label={`Điểm ${row.student_name}`}
                              value={scoreDraft[row.student_id] ?? stored?.toString() ?? ""}
                              state={
                                invalidIds.has(row.student_id)
                                  ? "invalid"
                                  : row.student_id in scoreDraft
                                    ? "dirty"
                                    : "idle"
                              }
                              onChange={(raw) =>
                                setScoreDraft((draft) => ({ ...draft, [row.student_id]: raw }))
                              }
                              onCommit={(parsed) => commitScore(row.student_id, parsed)}
                            />
                          </div>
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
              {canWrite ? (
                <div className="mt-2.5 flex items-center gap-2.5">
                  <HvButton
                    type="button"
                    variant="primary"
                    size="sm"
                    onClick={() => void saveGeneralScores()}
                    disabled={!scoresDirty || hasInvalidScores || saveMarksMutation.isPending}
                  >
                    Lưu điểm buổi
                  </HvButton>
                  <span className="text-[12.5px] font-bold text-sun-600">
                    {scoresDirty ? "Chưa lưu" : ""}
                  </span>
                </div>
              ) : null}
            </>
          )}
        </div>
      </div>
    );

    // The section is not interactive itself; it only relays Escape from the
    // focusable fields inside it, which the a11y rule cannot tell apart.
    return (
      // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions
      <section
        aria-label={`Chi tiết buổi ${sessionLabel}`}
        onKeyDown={(event) => {
          // Escape anywhere in the row closes it (through the page guard).
          // Portalled dialogs bubble through React but live outside this DOM
          // subtree — their Escape stays theirs.
          if (event.key !== "Escape" || event.defaultPrevented) return;
          if (!event.currentTarget.contains(event.target as Node)) return;
          event.preventDefault();
          onClose();
        }}
        className="border-y-[1.5px] border-mint-200 bg-cream-50"
      >
        <div className="grid grid-cols-1 min-[900px]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_minmax(0,1.2fr)]">
          <div className="border-b-[1.5px] border-line-100 px-4 py-3.5 min-[900px]:border-r-[1.5px] min-[900px]:border-b-0">
            {noteBlock}
          </div>
          <div className="border-b-[1.5px] border-line-100 px-4 py-3.5 min-[900px]:border-r-[1.5px] min-[900px]:border-b-0">
            {planBlock}
          </div>
          <div className="px-4 py-3.5">{scoresBlock}</div>
        </div>
        <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-line-100 bg-white px-4 py-2.5 text-[12px] text-ink-400">
          <span className="hidden items-center gap-1.5 sm:inline-flex">
            Di chuyển <Kbd>↑</Kbd> <Kbd>↓</Kbd> · mở/đóng <Kbd>Enter</Kbd>
          </span>
          {/* The by-student block announces its own count in its footer. */}
          {!hasScoreComponents && dirtyCount > 0 ? (
            <span role="status" className="font-bold text-sun-600">
              {dirtyCount} ô chưa lưu
            </span>
          ) : null}
          <HvButton
            type="button"
            variant="ghost"
            size="sm"
            aria-label="Đóng chi tiết buổi"
            icon={<HvIcon name="x" size={16} />}
            onClick={onClose}
            className="ml-auto"
          >
            Đóng
          </HvButton>
        </div>
      </section>
    );
  },
);
