import { z } from "zod";

import { MAX_SCORE_SET_COMPONENTS, findDuplicateIndexes } from "../lib/score-set-components";

/**
 * `grading.ScoreSetResponse` — one center-wide bộ điểm: a name plus an
 * ordered list of column names (order = position, index-based, mirroring
 * `apps/api/internal/features/grading`).
 */
export const scoreSetSchema = z.object({
  id: z.string(),
  name: z.string(),
  components: z.array(z.string()),
});

export type ScoreSet = z.infer<typeof scoreSetSchema>;

/**
 * Component-name rules mirror the server (1..10 rows, each non-blank and
 * ≤50 chars, unique case-insensitively) so most edits round-trip in one
 * write instead of bouncing on a 422.
 */
const componentNamesField = z
  .array(z.string().trim().min(1, "Tên cột điểm không được để trống").max(50, "Tối đa 50 ký tự"))
  .min(1, "Cần ít nhất 1 cột điểm")
  .max(MAX_SCORE_SET_COMPONENTS, `Tối đa ${MAX_SCORE_SET_COMPONENTS} cột điểm`)
  .superRefine((names, ctx) => {
    for (const index of findDuplicateIndexes(names)) {
      ctx.addIssue({
        code: "custom",
        path: [index],
        message: "Tên cột điểm bị trùng",
      });
    }
  });

/** `grading.ScoreSetRequest` — create/update body for one bộ điểm. */
export const scoreSetInputSchema = z.object({
  name: z.string().trim().min(1, "Bắt buộc nhập tên bộ điểm").max(100, "Tối đa 100 ký tự"),
  components: componentNamesField,
});

export type ScoreSetInput = z.infer<typeof scoreSetInputSchema>;

/**
 * One configured score column for a class. The `teaching` feature reads the
 * same wire shape from `GET /classes/:id/score-components` for the
 * classbook/gradebook grid; this copy stays scoped to the `center` config
 * screen so the two feature modules keep no cross-feature import for it.
 */
export const classScoreComponentSchema = z.object({
  id: z.string(),
  name: z.string(),
  position: z.number().int(),
});

export type ClassScoreComponent = z.infer<typeof classScoreComponentSchema>;

/**
 * `grading.ClassScoreComponentsResponse` — a class's currently assigned
 * columns in position order. This is a point-in-time snapshot copied onto
 * the class at assign time, not a live reference back to a `ScoreSet` id —
 * editing or deleting a bộ điểm afterward never changes a class that already
 * used it.
 */
export const classScoreComponentsSchema = z.object({
  class_id: z.string(),
  components: z.array(classScoreComponentSchema),
});

export type ClassScoreComponents = z.infer<typeof classScoreComponentsSchema>;
