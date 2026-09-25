import { useMemo } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { mapApiError } from "@/features/tasks";
import {
  asColumnId,
  asTaskId,
  kanbanReducer,
  type KanbanBoard,
  type KanbanColumn,
  type KanbanDataSource,
  type KanbanTask,
} from "@/lib/kanban";

import {
  getBoard,
  listAssignees,
  updateLessonAssignment,
  updateLessonPrep,
} from "../api/library-api";
import { prepStatusLabel } from "../lib/library-labels";
import {
  PREP_STATUSES,
  type AssignmentInput,
  type BoardCard,
  type PrepBoard,
  type PrepInput,
  type PrepStatus,
  type ProgramTemplate,
  type TemplateVersion,
} from "../schemas/library-schemas";
import { assigneesKeys, lessonsKeys, templatesKeys, versionsKeys } from "./library-keys";

/**
 * Assign-picker source for the prep assignment page: the center's live
 * members via `prep.assign` (`GET /library/assignees`), not the
 * `members.list`-gated member directory `useMemberDirectory` uses elsewhere.
 */
export function useAssignees(enabled = true) {
  return useQuery({
    queryKey: assigneesKeys.all,
    queryFn: listAssignees,
    staleTime: 60_000,
    enabled,
  });
}

/* eslint-disable @typescript-eslint/only-throw-error -- the data source
 * rejects with the headless lib's plain `KanbanError` union, not an `Error`
 * instance, exactly like the tasks adapter (see use-tasks-data-source.ts). */

/**
 * One lesson as a board card: the lib's `KanbanTask` fields plus what the
 * card shows. `columnId` is the prep status; `position` is the lesson's
 * position in the version, so a column lists its lessons in syllabus order.
 */
export interface PrepCard extends KanbanTask {
  title: string;
  lessonPosition: number;
  assigneeId: string | null;
  assigneeName: string | null;
  dueDate: string | null;
  checklistDone: number;
  checklistTotal: number;
}

export interface PrepBoardData {
  template: ProgramTemplate;
  version: TemplateVersion;
  board: KanbanBoard<PrepCard>;
}

/** The four fixed columns, in `PREP_STATUSES` order; only "done" counts as finished. */
export const PREP_COLUMNS: KanbanColumn[] = PREP_STATUSES.map((status, order) => ({
  id: asColumnId(status),
  name: prepStatusLabel[status],
  order,
  isDone: status === "done",
}));

function toCard(card: BoardCard): PrepCard {
  return {
    id: asTaskId(card.id),
    columnId: asColumnId(card.prep_status),
    position: card.position,
    title: card.title,
    lessonPosition: card.position,
    assigneeId: card.assignee_id,
    assigneeName: card.assignee_name,
    dueDate: card.due_date,
    checklistDone: card.checklist_done,
    checklistTotal: card.checklist_total,
  };
}

function toBoardData(response: PrepBoard): PrepBoardData {
  return {
    template: response.template,
    version: response.version,
    board: {
      columns: PREP_COLUMNS,
      tasks: response.columns.flatMap((column) => column.lessons.map(toCard)),
    },
  };
}

export function usePrepBoard(versionId: string | undefined) {
  return useQuery({
    queryKey: versionsKeys.board(versionId ?? ""),
    queryFn: async () => toBoardData(await getBoard(versionId ?? "")),
    enabled: Boolean(versionId),
  });
}

/**
 * Refetch everything a prep write can change: the board, the lesson rows
 * (status/checklist/assignee live on them) and the template lists, whose
 * prep summary counts done lessons and assignees.
 */
function usePrepInvalidation(versionId: string) {
  const queryClient = useQueryClient();
  return () => {
    void queryClient.invalidateQueries({ queryKey: versionsKeys.board(versionId) });
    void queryClient.invalidateQueries({ queryKey: lessonsKeys.list(versionId) });
    void queryClient.invalidateQueries({ queryKey: lessonsKeys.details() });
    void queryClient.invalidateQueries({ queryKey: templatesKeys.lists() });
  };
}

/**
 * `KanbanDataSource` over the preparation board. Only `moveTask` is real:
 * the columns are the fixed prep statuses and lessons are created on the
 * template page, so every other port method rejects as forbidden. A move
 * applies optimistically to the cached board and PATCHes the lesson's
 * status; the settled refetch is what puts a refused card back.
 */
export function usePrepDataSource(versionId: string): KanbanDataSource<PrepCard> {
  const queryClient = useQueryClient();
  const invalidate = usePrepInvalidation(versionId);

  return useMemo<KanbanDataSource<PrepCard>>(() => {
    const boardKey = versionsKeys.board(versionId);
    const forbidden = (): Promise<never> =>
      // eslint-disable-next-line @typescript-eslint/prefer-promise-reject-errors -- the lib's plain `KanbanError` union
      Promise.reject({ kind: "forbidden" } as const);

    return {
      loadBoard: async () => toBoardData(await getBoard(versionId)).board,
      createColumn: forbidden,
      updateColumn: forbidden,
      reorderColumns: forbidden,
      deleteColumn: forbidden,
      createTask: forbidden,
      updateTask: forbidden,
      deleteTask: forbidden,
      moveTask: async (taskId, columnId) => {
        const cached = queryClient.getQueryData<PrepBoardData>(boardKey);
        const card = cached?.board.tasks.find((task) => task.id === taskId);
        if (!cached || !card) {
          throw { kind: "not-found", entity: "task" } as const;
        }
        // The column lists lessons in syllabus order, so the card keeps its
        // own position rather than taking the drop index.
        queryClient.setQueryData<PrepBoardData>(boardKey, {
          ...cached,
          board: kanbanReducer(cached.board, {
            type: "tasks/moved",
            taskId,
            columnId,
            position: card.lessonPosition,
          }),
        });
        try {
          const lesson = await updateLessonPrep(String(taskId), {
            prep_status: columnId as string as PrepStatus,
          });
          return {
            ...card,
            columnId: asColumnId(lesson.prep_status),
            checklistDone: lesson.checklist.filter((item) => item.done).length,
            checklistTotal: lesson.checklist.length,
          };
        } catch (error) {
          throw mapApiError(error, "task");
        } finally {
          invalidate();
        }
      },
    };
    // `invalidate` closes over a stable query client; only the version matters.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [queryClient, versionId]);
}

export function useUpdateLessonPrep(lessonId: string, versionId: string) {
  const invalidate = usePrepInvalidation(versionId);
  return useMutation({
    mutationFn: (input: PrepInput) => updateLessonPrep(lessonId, input),
    onSettled: invalidate,
  });
}

export function useUpdateLessonAssignment(lessonId: string, versionId: string) {
  const invalidate = usePrepInvalidation(versionId);
  return useMutation({
    mutationFn: (input: AssignmentInput) => updateLessonAssignment(lessonId, input),
    onSettled: invalidate,
  });
}
