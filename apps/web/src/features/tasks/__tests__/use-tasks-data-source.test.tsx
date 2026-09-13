import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { asColumnId, asTaskId, isKanbanError } from "@/lib/kanban";
import { API_URL, fail, ok, primaryTeacher } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { signInAs, testPrimaryTeacher } from "@/test/utils";

import { getBoard } from "../api/tasks-api";
import { tasksKeys } from "../hooks/tasks-keys";
import {
  toBoardQueryData,
  useTasksDataSource,
  type BoardQueryData,
} from "../hooks/use-tasks-data-source";
import {
  columnDoneId,
  columnTodoId,
  ownTaskId,
  resetTasksStore,
  tasksHandlers,
} from "./tasks-handlers";

beforeEach(() => {
  resetTasksStore();
  server.use(...tasksHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

function setup() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
  );
  return { queryClient, wrapper };
}

describe("useTasksDataSource", () => {
  it("maps the wire board response onto the headless lib's camelCase shape", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });

    const board = await result.current.dataSource.loadBoard();

    expect(board.columns.map((column) => column.name)).toEqual(["Cần làm", "Hoàn thành"]);
    expect(board.columns.find((column) => column.id === columnDoneId)?.isDone).toBe(true);
    const task = board.tasks.find((candidate) => candidate.id === ownTaskId);
    expect(task).toMatchObject({
      title: "Soạn đề kiểm tra giữa kỳ",
      columnId: columnTodoId,
      priority: "medium",
      assigneeId: primaryTeacher.id,
      createdBy: primaryTeacher.id,
    });
  });

  it("maps a 422 field error from the API onto a KanbanError validation result", async () => {
    const { wrapper } = setup();
    const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });

    await expect(
      result.current.createColumnMutation.mutateAsync({ name: "Cần làm", isDone: false }),
    ).rejects.toMatchObject({ kind: "validation", fields: { name: "Tên cột đã tồn tại" } });
  });

  it("maps a 403 response onto a KanbanError forbidden result", async () => {
    server.use(
      http.delete(`${API_URL}/tasks/:id`, () =>
        HttpResponse.json(fail("FORBIDDEN", "not allowed"), { status: 403 }),
      ),
    );
    const { wrapper } = setup();
    const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });

    await expect(
      result.current.deleteTaskMutation.mutateAsync(asTaskId(ownTaskId)),
    ).rejects.toEqual({
      kind: "forbidden",
    });
  });

  it.each([409, 422])(
    "collapses a %d response from the reorder endpoint into a conflict, not a field error",
    async (status) => {
      server.use(
        http.put(`${API_URL}/task-columns/order`, () =>
          HttpResponse.json(
            status === 422
              ? fail("INVALID_PERMUTATION", "stale order", { ids: "stale" })
              : fail("CONFLICT", "stale order"),
            { status },
          ),
        ),
      );
      const { wrapper } = setup();
      const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });

      const error: unknown = await result.current.reorderColumnsMutation
        .mutateAsync([asColumnId(columnTodoId), asColumnId(columnDoneId)])
        .catch((caught: unknown) => caught);

      expect(isKanbanError(error) && error.kind).toBe("conflict");
    },
  );

  it("serializes mutations sharing the kanban-board scope instead of racing them", async () => {
    const order: string[] = [];
    server.use(
      http.post(`${API_URL}/tasks`, async () => {
        order.push("create:start");
        await new Promise((resolve) => setTimeout(resolve, 30));
        order.push("create:end");
        return HttpResponse.json(
          ok({
            id: "61000000-0000-4000-8000-000000000050",
            column_id: columnTodoId,
            title: "Việc mới",
            description: "",
            priority: "none",
            due_on: null,
            assignee_id: null,
            created_by: primaryTeacher.id,
            position: 0,
            completed_at: null,
            created_at: "2026-09-13T10:00:00Z",
            updated_at: "2026-09-13T10:00:00Z",
          }),
          { status: 201 },
        );
      }),
      http.post(`${API_URL}/tasks/:id/move`, async ({ params }) => {
        order.push("move:start");
        await new Promise((resolve) => setTimeout(resolve, 5));
        order.push("move:end");
        return HttpResponse.json(
          ok({
            id: params.id as string,
            column_id: columnDoneId,
            title: "Soạn đề kiểm tra giữa kỳ",
            description: "",
            priority: "medium",
            due_on: "2026-09-25",
            assignee_id: primaryTeacher.id,
            created_by: primaryTeacher.id,
            position: 0,
            completed_at: "2026-09-13T10:00:00Z",
            created_at: "2026-09-10T08:00:00Z",
            updated_at: "2026-09-13T10:00:00Z",
          }),
        );
      }),
    );
    const { wrapper } = setup();
    const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });

    const createPromise = result.current.createTaskMutation.mutateAsync({
      title: "Việc mới",
      columnId: columnTodoId,
    });
    const movePromise = result.current.dataSource.moveTask(
      asTaskId(ownTaskId),
      asColumnId(columnDoneId),
      0,
    );

    await Promise.all([createPromise, movePromise]);

    await waitFor(() => {
      expect(order).toEqual(["create:start", "create:end", "move:start", "move:end"]);
    });
  });

  it("keeps a deleted column's tasks visible in the destination column while the delete is in flight", async () => {
    server.use(
      http.delete(`${API_URL}/task-columns/:id`, async () => {
        await new Promise((resolve) => setTimeout(resolve, 30));
        return HttpResponse.json(ok({ moved_count: 3 }));
      }),
    );
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useTasksDataSource({ scope: "mine" }), { wrapper });
    const boardKey = tasksKeys.board({ scope: "mine" });
    queryClient.setQueryData(boardKey, toBoardQueryData(await getBoard({ scope: "mine" })));

    const deletePromise = result.current.deleteColumnMutation.mutateAsync({
      columnId: asColumnId(columnTodoId),
      moveTasksTo: asColumnId(columnDoneId),
    });

    await waitFor(() => {
      const cached = queryClient.getQueryData<BoardQueryData>(boardKey);
      expect(cached?.board.columns.map((column) => column.id)).toEqual([columnDoneId]);
      expect(cached?.board.tasks.filter((task) => task.columnId === columnDoneId)).toHaveLength(3);
    });

    await deletePromise;
  });
});
