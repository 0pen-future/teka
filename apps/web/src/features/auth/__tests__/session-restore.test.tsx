import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { describe, expect, it } from "vitest";

import { SessionRestore, useAuthStore } from "@/features/auth";
import { API_URL, makeSession, ok, primaryTeacher } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders } from "@/test/utils";

describe("SessionRestore", () => {
  it("restores the session from the refresh cookie before rendering", async () => {
    server.use(
      http.post(`${API_URL}/auth/refresh`, () =>
        HttpResponse.json(ok(makeSession(primaryTeacher))),
      ),
    );

    renderWithProviders(
      <SessionRestore>
        <p>App content</p>
      </SessionRestore>,
    );

    expect(await screen.findByText("App content")).toBeInTheDocument();
    expect(useAuthStore.getState().user?.phone).toBe(primaryTeacher.phone);
    expect(useAuthStore.getState().accessToken).toBe("test-access-token");
  });

  it("renders signed-out when there is no session to restore", async () => {
    // Default handler answers 401: a fresh visitor without a refresh cookie.
    renderWithProviders(
      <SessionRestore>
        <p>App content</p>
      </SessionRestore>,
    );

    expect(await screen.findByText("App content")).toBeInTheDocument();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().accessToken).toBeNull();
  });

  it("skips the refresh attempt entirely on the public statement route", () => {
    let refreshRequested = false;
    server.use(
      http.post(`${API_URL}/auth/refresh`, () => {
        refreshRequested = true;
        return HttpResponse.json(ok(makeSession(primaryTeacher)));
      }),
    );
    const originalLocation = window.location.pathname;
    window.history.pushState({}, "", "/s/some-token");

    try {
      renderWithProviders(
        <SessionRestore>
          <p>App content</p>
        </SessionRestore>,
      );

      // No restoring spinner is shown at all — content renders immediately.
      expect(screen.getByText("App content")).toBeInTheDocument();
      expect(refreshRequested).toBe(false);
    } finally {
      window.history.pushState({}, "", originalLocation);
    }
  });
});
