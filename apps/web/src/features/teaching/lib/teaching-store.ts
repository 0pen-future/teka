import { z } from "zod";

/**
 * Teaching domain vocabulary shared by the classbook, records, and review
 * screens: the store-shaped types the components consume, the giáo án state
 * machine, and the composite key helpers. Persistence lives server-side
 * (`apps/api/internal/features/teaching`); the React Query hooks in
 * `../hooks` adapt the wire shapes to these types, so no component ever
 * sees a wire payload.
 */

/**
 * The prototype's fixed per-session operating cost for the LÃI/LỖ stat. The
 * backend models no cost setting, so this stays a UI constant surfaced in the
 * classbook table footnote; revisit if a real center setting ever lands.
 */
export const SESSION_COST_VND = 300_000;

export const lessonPlanStatusSchema = z.enum(["none", "draft", "pending", "approved", "redo"]);

export type LessonPlanStatus = z.infer<typeof lessonPlanStatusSchema>;

export interface LessonPlan {
  goal: string;
  activities: string[];
  homework: string;
  fileName?: string;
  status: LessonPlanStatus;
  redoNote?: string;
  ownerComment?: string;
  submittedBy?: string;
}

export interface Curriculum {
  lessons: string[];
  currentIndex: number;
}

export interface TeachingState {
  /** classId → curriculum (lesson titles + progress pointer). */
  curricula: Record<string, Curriculum>;
  /** `lessonPlanKey(classId, lessonIndex)` → giáo án. */
  lessonPlans: Record<string, LessonPlan>;
  /** sessionId → whole-class nhận xét. */
  sessionNotes: Record<string, { text: string }>;
  /** sessionId → studentId → score. */
  sessionScores: Record<string, Record<string, number>>;
  /** `personalNoteKey(sessionId, studentId)` → per-student note. */
  personalNotes: Record<string, string>;
}

export type LessonPlanAction = "save" | "submit" | "approve" | "requestRedo" | "reopen";

/**
 * The giáo án review loop, shared by the teacher (save/submit) and owner
 * (approve/requestRedo/reopen) screens so the two can never disagree on what
 * a status allows. Saving from `redo` keeps `redo` — the plan stays
 * submittable and the owner's note stays visible until resubmission.
 * Reopening from `redo` is the owner withdrawing their own request; the
 * teacher's path out of `redo` stays submit-only. The server enforces the
 * same table (`apps/api/internal/features/teaching/service.go`) and answers
 * 409 for an illegal move.
 */
const lessonPlanTransitions: Record<
  LessonPlanStatus,
  Partial<Record<LessonPlanAction, LessonPlanStatus>>
> = {
  none: { save: "draft" },
  draft: { save: "draft", submit: "pending" },
  pending: { approve: "approved", requestRedo: "redo" },
  redo: { save: "redo", submit: "pending", reopen: "pending" },
  approved: { reopen: "pending" },
};

/** Next status for a legal move, null for an illegal one — callers must not coerce. */
export function transitionLessonPlanStatus(
  status: LessonPlanStatus,
  action: LessonPlanAction,
): LessonPlanStatus | null {
  return lessonPlanTransitions[status][action] ?? null;
}

export function lessonPlanKey(classId: string, lessonIndex: number): string {
  return `${classId}#${lessonIndex}`;
}

export function personalNoteKey(sessionId: string, studentId: string): string {
  return `${sessionId}#${studentId}`;
}
