import { useQuery } from "@tanstack/react-query";
import { waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { apiClient } from "@/lib/api/client";
import { API_URL, fail, makeTeacher } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import {
  renderWithProviders,
  signInAs,
  testPrimaryTeacher,
  testSecondaryTeacher,
} from "@/test/utils";

/** A query hook standing in for real feature code — its only job is to sit
 * in the cache under a key SessionCacheReset must clear. */
function Probe() {
  useQuery({
    queryKey: ["probe"],
    queryFn: () => apiClient.get<unknown>("/probe").then((res) => res.data),
  });
  return null;
}

describe("SessionCacheReset", () => {
  it("clears the cache when the session ends via clearSession (logout, dead refresh)", async () => {
    signInAs(testPrimaryTeacher);
    const { queryClient } = renderWithProviders(<p>content</p>);
    queryClient.setQueryData(["seed"], 1);

    useAuthStore.getState().clearSession();

    await waitFor(() => expect(queryClient.getQueryCache().getAll()).toHaveLength(0));
  });

  it("clears the cache when the signed-in identity changes on the same tab", async () => {
    signInAs(testPrimaryTeacher);
    const { queryClient } = renderWithProviders(<p>content</p>);
    queryClient.setQueryData(["seed"], 1);

    useAuthStore.getState().setSession(testSecondaryTeacher, "second-access-token");

    await waitFor(() => expect(queryClient.getQueryCache().getAll()).toHaveLength(0));
  });

  it("keeps the cache when the profile updates under the same id", async () => {
    signInAs(testPrimaryTeacher);
    const { queryClient } = renderWithProviders(<p>content</p>);
    queryClient.setQueryData(["seed"], 1);

    useAuthStore.getState().setUser({ ...testPrimaryTeacher, full_name: "Cô Lan (đổi tên)" });

    await waitFor(() => expect(queryClient.getQueryCache().getAll()).toHaveLength(1));
    expect(queryClient.getQueryData(["seed"])).toBe(1);
  });

  it("keeps the cache on an access-token rotation", async () => {
    signInAs(testPrimaryTeacher);
    const { queryClient } = renderWithProviders(<p>content</p>);
    queryClient.setQueryData(["seed"], 1);

    useAuthStore.getState().setAccessToken("rotated-access-token");

    await waitFor(() => expect(queryClient.getQueryCache().getAll()).toHaveLength(1));
    expect(queryClient.getQueryData(["seed"])).toBe(1);
  });

  it("does not clear on mount when the tab is already signed in", async () => {
    signInAs(testPrimaryTeacher);
    const { queryClient } = renderWithProviders(<p>content</p>);
    queryClient.setQueryData(["seed"], 1);

    await waitFor(() => expect(queryClient.getQueryData(["seed"])).toBe(1));
    expect(queryClient.getQueryCache().getAll()).toHaveLength(1);
  });

  it("clears the cache end-to-end when a query 401s and the refresh also dies", async () => {
    server.use(
      http.get(`${API_URL}/probe`, () =>
        HttpResponse.json(fail("UNAUTHORIZED", "unauthorized"), { status: 401 }),
      ),
      http.post(`${API_URL}/auth/refresh`, () =>
        HttpResponse.json(fail("UNAUTHORIZED", "session expired"), { status: 401 }),
      ),
    );
    signInAs(makeTeacher());
    const { queryClient } = renderWithProviders(<Probe />);
    queryClient.setQueryData(["seed"], 1);

    await waitFor(() => expect(queryClient.getQueryCache().getAll()).toHaveLength(0));
    expect(useAuthStore.getState().user).toBeNull();
  });
});
