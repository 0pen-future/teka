import { useSyncExternalStore } from "react";
import { z } from "zod";

/**
 * Client-side store for teaching-only data the backend does not model:
 * curriculum, lesson plans (giáo án), session scores/notes, and per-student
 * personal notes. Deliberately device-local (accepted product trade-off for
 * the UI-first delivery): in-memory snapshots per center, persisted to
 * localStorage under `teka.teaching.<centerId>` so switching centers never
 * leaks another center's data.
 *
 * Pattern: module-scope singleton + `useSyncExternalStore` — the same
 * external-cache trade-off React Query makes, without dragging local,
 * non-server state into query caching semantics. Writes are immutable
 * replacements; snapshot identity is what re-renders subscribers.
 */

/**
 * The prototype's fixed per-session operating cost for the LÃI/LỖ stat. The
 * backend models no cost setting, so this stays a UI constant surfaced in the
 * classbook table footnote; revisit if a real center setting ever lands.
 */
export const SESSION_COST_VND = 300_000;

export const lessonPlanStatusSchema = z.enum(["none", "draft", "pending", "approved", "redo"]);

export type LessonPlanStatus = z.infer<typeof lessonPlanStatusSchema>;

const lessonPlanSchema = z.object({
  goal: z.string(),
  activities: z.array(z.string()),
  homework: z.string(),
  fileName: z.string().optional(),
  status: lessonPlanStatusSchema,
  redoNote: z.string().optional(),
  ownerComment: z.string().optional(),
  submittedBy: z.string().optional(),
});

export type LessonPlan = z.infer<typeof lessonPlanSchema>;

const curriculumSchema = z.object({
  lessons: z.array(z.string()),
  currentIndex: z.number().int().nonnegative(),
});

export type Curriculum = z.infer<typeof curriculumSchema>;

const teachingStateSchema = z.object({
  /** classId → curriculum (lesson titles + progress pointer). */
  curricula: z.record(z.string(), curriculumSchema),
  /** `lessonPlanKey(classId, lessonIndex)` → giáo án. */
  lessonPlans: z.record(z.string(), lessonPlanSchema),
  /** sessionId → whole-class nhận xét. */
  sessionNotes: z.record(z.string(), z.object({ text: z.string() })),
  /** sessionId → studentId → score. */
  sessionScores: z.record(z.string(), z.record(z.string(), z.number())),
  /** `personalNoteKey(sessionId, studentId)` → per-student note. */
  personalNotes: z.record(z.string(), z.string()),
});

export type TeachingState = z.infer<typeof teachingStateSchema>;

/**
 * Versioned persistence envelope: if later phases change the state shape,
 * bump the version and unreadable/legacy payloads fall back to empty state
 * instead of throwing — acceptable because this data is local-only and
 * non-authoritative.
 */
const envelopeSchema = z.object({ version: z.literal(1), state: teachingStateSchema });

export function lessonPlanKey(classId: string, lessonIndex: number): string {
  return `${classId}#${lessonIndex}`;
}

export function personalNoteKey(sessionId: string, studentId: string): string {
  return `${sessionId}#${studentId}`;
}

function storageKey(centerId: string): string {
  return `teka.teaching.${centerId}`;
}

function createEmptyState(): TeachingState {
  return { curricula: {}, lessonPlans: {}, sessionNotes: {}, sessionScores: {}, personalNotes: {} };
}

/** Stable snapshot for callers that have no resolved center yet. */
const NO_CENTER_STATE: TeachingState = createEmptyState();

const snapshots = new Map<string, TeachingState>();
const listeners = new Set<() => void>();

function loadState(centerId: string): TeachingState {
  try {
    const raw = localStorage.getItem(storageKey(centerId));
    if (!raw) {
      return createEmptyState();
    }
    const parsed = envelopeSchema.safeParse(JSON.parse(raw));
    return parsed.success ? parsed.data.state : createEmptyState();
  } catch {
    // JSON.parse throw or localStorage unavailable — never break the UI
    // over non-authoritative local data.
    return createEmptyState();
  }
}

export function getTeachingSnapshot(centerId: string): TeachingState {
  const cached = snapshots.get(centerId);
  if (cached) {
    return cached;
  }
  const loaded = loadState(centerId);
  snapshots.set(centerId, loaded);
  return loaded;
}

/**
 * Immutable replacement: the recipe receives the current snapshot and returns
 * the next one. Persisting failure (quota, private mode) is tolerated — the
 * in-memory snapshot still updates and subscribers still re-render.
 */
export function updateTeachingState(
  centerId: string,
  recipe: (state: TeachingState) => TeachingState,
): void {
  const next = recipe(getTeachingSnapshot(centerId));
  snapshots.set(centerId, next);
  try {
    localStorage.setItem(storageKey(centerId), JSON.stringify({ version: 1, state: next }));
  } catch {
    // Ignore persistence failure; the session keeps working in memory.
  }
  for (const listener of listeners) {
    listener();
  }
}

export function subscribeTeaching(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/** Owner nav dot + review queue badge source. */
export function countPendingPlans(state: TeachingState): number {
  return Object.values(state.lessonPlans).filter((plan) => plan.status === "pending").length;
}

/** Drops in-memory snapshots only — localStorage survives, like a reload. */
export function resetTeachingStoreForTests(): void {
  snapshots.clear();
}

/** Reactive read; `null` center (still resolving) reads as empty state. */
export function useTeachingStore(centerId: string | null): TeachingState {
  return useSyncExternalStore(subscribeTeaching, () =>
    centerId ? getTeachingSnapshot(centerId) : NO_CENTER_STATE,
  );
}

/**
 * Number-snapshot subscription for the nav dot: `Object.is`-stable, so shell
 * consumers re-render only when the count actually changes — not on every
 * store write (phases 3–5 write per keystroke).
 */
export function usePendingPlanCount(centerId: string | null): number {
  return useSyncExternalStore(subscribeTeaching, () =>
    centerId ? countPendingPlans(getTeachingSnapshot(centerId)) : 0,
  );
}
