import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { signInAs, testPrimaryTeacher } from "@/test/utils";

import { coursesKeys } from "../hooks/courses-keys";
import { pathsKeys } from "../hooks/paths-keys";
import { useArchiveCourse, useDeleteCourse, useUpdateCourse } from "../hooks/use-courses";
import { useDeletePath, useReorderStages, useUpdatePath } from "../hooks/use-paths";
import { coursesHandlers, courseToan6, courseVan9, resetCoursesStore } from "./courses-handlers";
import { pathsHandlers, pathToan, resetPathsStore } from "./paths-handlers";

beforeEach(() => {
  resetCoursesStore();
  resetPathsStore();
  server.use(...coursesHandlers, ...pathsHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

/**
 * The app-wide 30s staleTime serves cached pages as-is, so anything that
 * changes what another page shows about a path must mark that cache stale:
 * a course page embeds the paths recommending it, and a path's timeline
 * embeds each course's name and status.
 */
describe("learning path cache invalidation", () => {
  const byCourseKey = pathsKeys.byCourse(courseToan6.id);
  const detailKey = pathsKeys.detail(pathToan.id);
  const listKey = pathsKeys.list({});

  function setup() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: 30_000 },
        mutations: { retry: false },
      },
    });
    queryClient.setQueryData(byCourseKey, []);
    queryClient.setQueryData(detailKey, pathToan);
    queryClient.setQueryData(listKey, { items: [], meta: {} });
    queryClient.setQueryData(coursesKeys.detail(courseToan6.id), courseToan6);
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    return { queryClient, wrapper };
  }

  function invalidated(queryClient: QueryClient, key: readonly unknown[]) {
    return queryClient.getQueryState(key)?.isInvalidated;
  }

  it("updating a path refreshes the per-course listings", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useUpdatePath(pathToan.id), { wrapper });

    await result.current.mutateAsync({
      code: pathToan.code,
      name: "Lộ trình Toán 6-9",
      description: null,
      status: "active",
    });

    expect(invalidated(queryClient, byCourseKey)).toBe(true);
  });

  it("deleting a path refreshes the per-course listings", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useDeletePath(), { wrapper });

    await result.current.mutateAsync(pathToan.id);

    expect(invalidated(queryClient, byCourseKey)).toBe(true);
  });

  it("a failed stage mutation refetches the path so the editor is not stuck on stale stages", async () => {
    server.use(
      http.put(`${API_URL}/paths/:id/stages/order`, () =>
        HttpResponse.json(fail("VALIDATION_ERROR", "validation failed"), { status: 422 }),
      ),
    );
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useReorderStages(pathToan.id), { wrapper });

    await expect(result.current.mutateAsync(["stale-stage-id"])).rejects.toThrow();

    expect(invalidated(queryClient, detailKey)).toBe(true);
  });

  it("editing a course refreshes the paths that embed its name", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useUpdateCourse(courseToan6.id), { wrapper });

    await result.current.mutateAsync({
      code: courseToan6.code,
      name: "Toán 6 cơ bản",
      subject: courseToan6.subject,
      level: courseToan6.level,
      description: courseToan6.description,
      status: "active",
      default_template_version_id: null,
      default_unit_price: courseToan6.default_unit_price,
      total_sessions: null,
      duration_min: null,
    });

    expect(invalidated(queryClient, detailKey)).toBe(true);
    expect(invalidated(queryClient, listKey)).toBe(true);
  });

  it("archiving a course refreshes the paths that show its status", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useArchiveCourse(courseToan6.id), { wrapper });

    await result.current.mutateAsync();

    expect(invalidated(queryClient, detailKey)).toBe(true);
  });

  it("deleting a course refreshes the path lists and details", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useDeleteCourse(), { wrapper });

    await result.current.mutateAsync(courseVan9.id);

    expect(invalidated(queryClient, detailKey)).toBe(true);
    expect(invalidated(queryClient, listKey)).toBe(true);
  });
});
