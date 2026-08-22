import { useMemo } from "react";
import { useQuery } from "@tanstack/react-query";

import { getCurriculum, listPlans } from "../api/teaching-api";
import type { Curriculum, LessonPlan } from "../lib/teaching-store";
import { lessonPlanKey } from "../lib/teaching-store";
import type { PlanResponse } from "../schemas/teaching-schemas";
import { teachingKeys } from "./teaching-keys";

export interface ClassTeaching {
  /** Undefined until the class saves one — components branch on that, not on `lessons.length`. */
  curriculum: Curriculum | undefined;
  /** `lessonPlanKey(classId, lessonIndex)` → giáo án; a missing key reads as status `"none"`. */
  lessonPlans: Record<string, LessonPlan>;
  pending: boolean;
}

/** Wire plan → the store-shaped `LessonPlan` the components were built against. */
export function toLessonPlan(plan: PlanResponse): LessonPlan {
  return {
    goal: plan.goal,
    activities: plan.activities,
    homework: plan.homework,
    fileName: plan.file_name ?? undefined,
    status: plan.status,
    redoNote: plan.redo_note ?? undefined,
    ownerComment: plan.owner_comment ?? undefined,
    submittedBy: plan.submitted_by_name ?? undefined,
  };
}

/**
 * A class's curriculum and giáo án map in the store-shaped types the
 * classbook components consume (`../lib/teaching-store`). The server has
 * no "no curriculum yet" 404 — it answers the empty default — so the adapter
 * maps an empty lesson list back to `undefined` to keep the components'
 * "chưa có chương trình" branches rendering identically.
 */
export function useClassTeaching(classId: string | undefined): ClassTeaching {
  const curriculumQuery = useQuery({
    queryKey: teachingKeys.curriculum(classId ?? ""),
    queryFn: () => getCurriculum(classId!),
    enabled: Boolean(classId),
  });
  const plansQuery = useQuery({
    queryKey: teachingKeys.plans(classId ?? ""),
    queryFn: () => listPlans(classId!),
    enabled: Boolean(classId),
  });

  const curriculum = useMemo<Curriculum | undefined>(() => {
    const data = curriculumQuery.data;
    if (!data || data.lessons.length === 0) {
      return undefined;
    }
    return { lessons: data.lessons, currentIndex: data.current_index };
  }, [curriculumQuery.data]);

  const lessonPlans = useMemo<Record<string, LessonPlan>>(() => {
    const map: Record<string, LessonPlan> = {};
    for (const plan of plansQuery.data ?? []) {
      map[lessonPlanKey(plan.class_id, plan.lesson_index)] = toLessonPlan(plan);
    }
    return map;
  }, [plansQuery.data]);

  return {
    curriculum,
    lessonPlans,
    pending: curriculumQuery.isPending || plansQuery.isPending,
  };
}
