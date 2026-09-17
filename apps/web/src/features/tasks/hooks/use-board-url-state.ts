import { useSearchParams } from "react-router";
import { z } from "zod";

import { asColumnId, type ColumnId } from "@/lib/kanban";

import { BOARD_FILTERS, type BoardFilterValue } from "../schemas/task-schemas";

const boardUrlStateSchema = z.object({
  filter: z.enum(BOARD_FILTERS).catch("all").default("all"),
  // A malformed value (not a uuid) degrades to "no assignee filter" rather
  // than a 422 from the board query — `assignee` only ever reaches the API
  // once it has passed this check.
  assignee: z.uuid().catch("").default(""),
  // Comma-joined `ColumnId`s; validated loosely here (each entry passes
  // through `asColumnId` unchanged) since a stale id just never matches a
  // rendered column and quietly drops out.
  collapsed: z.string().catch("").default(""),
});

export interface BoardUrlStateValues {
  filter: BoardFilterValue;
  assignee: string;
  collapsed: Set<ColumnId>;
}

export interface BoardUrlState extends BoardUrlStateValues {
  /** Merges `partial` into the current state; a key left at its default is dropped from the URL rather than written explicitly. */
  set: (partial: Partial<BoardUrlStateValues>) => void;
}

const DEFAULTS: Record<"filter" | "assignee" | "collapsed", string> = {
  filter: "all",
  assignee: "",
  collapsed: "",
};

/** Sorted so two sessions collapsing the same columns in a different order still produce the same URL. */
function serializeCollapsed(collapsed: Set<ColumnId>): string {
  return [...collapsed].sort().join(",");
}

function parseCollapsed(raw: string): Set<ColumnId> {
  if (raw === "") return new Set();
  return new Set(raw.split(",").filter(Boolean).map(asColumnId));
}

/**
 * Board filter/assignee/collapsed-columns state, read from and written back
 * to the URL via `useSearchParams` (see D4/D11 in the plan) instead of
 * component state — a reload, shared link, or the browser back button all
 * reproduce the same view. Values at their default are dropped from the
 * querystring entirely: `set` deletes the key rather than writing "all" or
 * "" explicitly, so an unfiltered board keeps a clean `/tasks` URL.
 */
export function useBoardUrlState(): BoardUrlState {
  const [searchParams, setSearchParams] = useSearchParams();

  const parsed = boardUrlStateSchema.parse({
    filter: searchParams.get("filter") ?? undefined,
    assignee: searchParams.get("assignee") ?? undefined,
    collapsed: searchParams.get("collapsed") ?? undefined,
  });

  const set = (partial: Partial<BoardUrlStateValues>): void => {
    const next = new URLSearchParams(searchParams);
    const nextValues: Record<"filter" | "assignee" | "collapsed", string> = {
      filter: partial.filter ?? parsed.filter,
      assignee: partial.assignee ?? parsed.assignee,
      collapsed: serializeCollapsed(partial.collapsed ?? parseCollapsed(parsed.collapsed)),
    };
    (Object.keys(nextValues) as (keyof typeof nextValues)[]).forEach((key) => {
      if (nextValues[key] === DEFAULTS[key]) {
        next.delete(key);
      } else {
        next.set(key, nextValues[key]);
      }
    });
    setSearchParams(next, { replace: true });
  };

  return {
    filter: parsed.filter,
    assignee: parsed.assignee,
    collapsed: parseCollapsed(parsed.collapsed),
    set,
  };
}
