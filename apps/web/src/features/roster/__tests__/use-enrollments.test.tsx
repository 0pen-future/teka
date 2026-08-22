import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { signInAs, testPrimaryTeacher } from "@/test/utils";

import { useCreateEnrollment, useEndEnrollment } from "../hooks/use-enrollments";
import { studentsKeys } from "../hooks/roster-keys";
import {
  classWithSchedule,
  enrollmentActive,
  resetRosterStore,
  rosterHandlers,
  studentOnlyChild,
} from "./roster-handlers";

beforeEach(() => {
  resetRosterStore();
  server.use(...rosterHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

/**
 * The roster screen navigates to the class tab right after enrolling, and
 * the app-wide 30s staleTime means a cached students list for that tab
 * would be served as-is — so enrollment mutations must mark every students
 * list stale or the just-enrolled student stays missing from the roster.
 */
describe("enrollment mutations", () => {
  const classListKey = studentsKeys.list({ class_id: classWithSchedule.id, per_page: 50 });

  function setup() {
    const queryClient = new QueryClient({
      defaultOptions: {
        queries: { retry: false, staleTime: 30_000 },
        mutations: { retry: false },
      },
    });
    queryClient.setQueryData(classListKey, {
      items: [],
      meta: { page: 1, per_page: 50, total: 0, total_pages: 1 },
    });
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    return { queryClient, wrapper };
  }

  it("creating an enrollment invalidates cached students lists", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useCreateEnrollment(), { wrapper });

    await result.current.mutateAsync({
      student_id: studentOnlyChild.id,
      class_id: classWithSchedule.id,
    });

    expect(queryClient.getQueryState(classListKey)?.isInvalidated).toBe(true);
  });

  it("ending an enrollment invalidates cached students lists", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useEndEnrollment(), { wrapper });

    await result.current.mutateAsync({ id: enrollmentActive.id });

    expect(queryClient.getQueryState(classListKey)?.isInvalidated).toBe(true);
  });
});
