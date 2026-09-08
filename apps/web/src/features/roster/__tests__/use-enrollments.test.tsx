import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderHook } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { sessionsKeys } from "@/features/attendance";
import { useAuthStore } from "@/features/auth";
import { API_URL } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { signInAs, testPrimaryTeacher } from "@/test/utils";

import {
  useCreateEnrollment,
  useDeleteEnrollment,
  useEndEnrollment,
} from "../hooks/use-enrollments";
import { useImportRoster } from "../hooks/use-roster-import";
import { studentsKeys } from "../hooks/roster-keys";
import { useAnonymizeStudent } from "../hooks/use-students";
import { makeImportReport, mockImportHappyPath } from "./roster-import-handlers";
import {
  classWithSchedule,
  enrollmentActive,
  resetRosterStore,
  rosterHandlers,
  studentOnlyChild,
  studentSiblingOne,
} from "./roster-handlers";

/** Stands in for whatever session an attendance sheet has open; the id itself is arbitrary. */
const SESSION_ID = "s1";

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
 * The same mutations change what an open attendance sheet's roster is
 * showing and a session's student_count, so both must go stale too.
 */
describe("enrollment mutations", () => {
  const classListKey = studentsKeys.list({ class_id: classWithSchedule.id, per_page: 50 });
  const sessionRosterKey = sessionsKeys.roster(SESSION_ID);
  const sessionDetailKey = sessionsKeys.detail(SESSION_ID);

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
    queryClient.setQueryData(sessionRosterKey, []);
    queryClient.setQueryData(sessionDetailKey, {});
    const wrapper = ({ children }: { children: ReactNode }) => (
      <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
    );
    return { queryClient, wrapper };
  }

  function expectSessionsInvalidated(queryClient: QueryClient) {
    expect(queryClient.getQueryState(sessionRosterKey)?.isInvalidated).toBe(true);
    expect(queryClient.getQueryState(sessionDetailKey)?.isInvalidated).toBe(true);
  }

  it("creating an enrollment invalidates cached students lists and sessions", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useCreateEnrollment(), { wrapper });

    await result.current.mutateAsync({
      student_id: studentOnlyChild.id,
      class_id: classWithSchedule.id,
    });

    expect(queryClient.getQueryState(classListKey)?.isInvalidated).toBe(true);
    expectSessionsInvalidated(queryClient);
  });

  it("ending an enrollment invalidates cached students lists and sessions", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useEndEnrollment(), { wrapper });

    await result.current.mutateAsync({ id: enrollmentActive.id });

    expect(queryClient.getQueryState(classListKey)?.isInvalidated).toBe(true);
    expectSessionsInvalidated(queryClient);
  });

  it("deleting an enrollment invalidates sessions", async () => {
    server.use(
      http.delete(`${API_URL}/enrollments/:id`, () => new HttpResponse(null, { status: 204 })),
    );
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useDeleteEnrollment(), { wrapper });

    await result.current.mutateAsync(enrollmentActive.id);

    expectSessionsInvalidated(queryClient);
  });

  it("committing a roster import invalidates sessions", async () => {
    mockImportHappyPath(makeImportReport({ enrollments: { created: 1, reused: 0 } }));
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useImportRoster(), { wrapper });

    await result.current.mutateAsync({ file: new File(["fake"], "roster.xlsx"), dryRun: false });

    expectSessionsInvalidated(queryClient);
  });

  it("a dry-run roster import does not invalidate sessions", async () => {
    mockImportHappyPath(makeImportReport({ enrollments: { created: 1, reused: 0 } }));
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useImportRoster(), { wrapper });

    await result.current.mutateAsync({ file: new File(["fake"], "roster.xlsx"), dryRun: true });

    expect(queryClient.getQueryState(sessionRosterKey)?.isInvalidated).toBeFalsy();
    expect(queryClient.getQueryState(sessionDetailKey)?.isInvalidated).toBeFalsy();
  });

  it("anonymizing a student invalidates sessions", async () => {
    const { queryClient, wrapper } = setup();
    const { result } = renderHook(() => useAnonymizeStudent(), { wrapper });

    await result.current.mutateAsync(studentSiblingOne.id);

    expectSessionsInvalidated(queryClient);
  });
});
