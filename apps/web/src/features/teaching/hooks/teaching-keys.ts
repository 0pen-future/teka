/**
 * Query-key factory for the teaching endpoints, shared by the read hooks and
 * the mutations' cache writes/invalidations (mirrors `sessionsKeys` in
 * `@/features/attendance`).
 */
export const teachingKeys = {
  all: ["teaching"] as const,
  curriculum: (classId: string) => [...teachingKeys.all, "curriculum", classId] as const,
  plans: (classId: string) => [...teachingKeys.all, "lesson-plans", classId] as const,
  classMarks: (classId: string) => [...teachingKeys.all, "marks", classId] as const,
  marks: (classId: string, month: string) => [...teachingKeys.classMarks(classId), month] as const,
  reviewQueue: () => [...teachingKeys.all, "review-queue"] as const,
};
