import { useSearchParams } from "react-router";
import { z } from "zod";

import { classPhases, type ClassPhase } from "../schemas/roster-schemas";

/** The status chips: every phase plus "all" and the recruiting flag. */
export const classListViews = ["all", ...classPhases, "recruiting"] as const;
export type ClassListView = (typeof classListViews)[number];

export function isClassPhaseView(view: ClassListView): view is ClassPhase {
  return (classPhases as readonly string[]).includes(view);
}

const weekdayValues = ["", "0", "1", "2", "3", "4", "5", "6"] as const;
const shiftValues = ["", "morning", "afternoon", "evening"] as const;

// A malformed value degrades to "no filter" rather than a 422 from the list
// query; the API only ever sees values that passed these enums.
const classListUrlStateSchema = z.object({
  view: z.enum(classListViews).catch("all").default("all"),
  q: z.string().catch("").default(""),
  weekday: z.enum(weekdayValues).catch("").default(""),
  shift: z.enum(shiftValues).catch("").default(""),
});

export type ClassListUrlValues = z.infer<typeof classListUrlStateSchema>;

export interface ClassListUrlState extends ClassListUrlValues {
  /** Merges `partial` into the current state; a key at its default is dropped from the URL. */
  set: (partial: Partial<ClassListUrlValues>) => void;
}

const DEFAULTS: ClassListUrlValues = { view: "all", q: "", weekday: "", shift: "" };

/**
 * Class-list filter state kept in the URL (same shape as the task board's
 * `useBoardUrlState`): a reload, a shared link or the back button reproduce
 * the same view, and an unfiltered list keeps a clean `/classes` URL.
 */
export function useClassListUrlState(): ClassListUrlState {
  const [searchParams, setSearchParams] = useSearchParams();

  const parsed = classListUrlStateSchema.parse({
    view: searchParams.get("view") ?? undefined,
    q: searchParams.get("q") ?? undefined,
    weekday: searchParams.get("weekday") ?? undefined,
    shift: searchParams.get("shift") ?? undefined,
  });

  const set = (partial: Partial<ClassListUrlValues>): void => {
    const next = new URLSearchParams(searchParams);
    const nextValues: ClassListUrlValues = { ...parsed, ...partial };
    (Object.keys(nextValues) as (keyof ClassListUrlValues)[]).forEach((key) => {
      if (nextValues[key] === DEFAULTS[key]) {
        next.delete(key);
      } else {
        next.set(key, nextValues[key]);
      }
    });
    setSearchParams(next, { replace: true });
  };

  return { ...parsed, set };
}
