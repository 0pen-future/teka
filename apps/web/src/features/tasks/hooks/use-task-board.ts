import { keepPreviousData, useQuery } from "@tanstack/react-query";

import { getBoard, type GetBoardParams } from "../api/tasks-api";
import { tasksKeys } from "./tasks-keys";
import { toBoardQueryData } from "./use-tasks-data-source";

/**
 * `keepPreviousData` avoids a loading flash across the day boundary, when
 * `params.today` rolls over and the query key changes — the previous board
 * stays on screen until the new one resolves. The query resolves to the same
 * `BoardQueryData` shape `use-tasks-data-source.ts`'s mutations write into
 * this cache entry via `setQueryData`, since both share this query key.
 */
export function useTaskBoard(params: GetBoardParams) {
  return useQuery({
    queryKey: tasksKeys.board(params),
    queryFn: async () => toBoardQueryData(await getBoard(params)),
    placeholderData: keepPreviousData,
  });
}
