import { screen } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

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

  describe("on a public auth page", () => {
    function trackRefresh() {
      const calls = { count: 0 };
      server.use(
        http.post(`${API_URL}/auth/refresh`, () => {
          calls.count += 1;
          return HttpResponse.json(ok(makeSession(primaryTeacher)));
        }),
      );
      return calls;
    }

    function renderAt(path: string) {
      window.history.pushState({}, "", path);
      renderWithProviders(
        <SessionRestore>
          <p>App content</p>
        </SessionRestore>,
      );
    }

    afterEach(() => window.history.pushState({}, "", "/"));

    it.each(["/login", "/forgot-password", "/reset-password/abc", "/invite/abc"])(
      "skips the refresh on %s for a browser that never signed in",
      (path) => {
        const calls = trackRefresh();
        renderAt(path);
        // No restoring spinner at all: content renders immediately.
        expect(screen.getByText("App content")).toBeInTheDocument();
        expect(calls.count).toBe(0);
      },
    );

    it("restores the session when this browser has signed in before", async () => {
      const calls = trackRefresh();
      window.localStorage.setItem("teka.hasSession", "1");
      renderAt("/login");
      expect(await screen.findByText("App content")).toBeInTheDocument();
      expect(calls.count).toBe(1);
      expect(useAuthStore.getState().user?.phone).toBe(primaryTeacher.phone);
    });
  });

  it("always tries to restore on a protected route, hint or not", async () => {
    let refreshRequested = false;
    server.use(
      http.post(`${API_URL}/auth/refresh`, () => {
        refreshRequested = true;
        return HttpResponse.json(ok(makeSession(primaryTeacher)));
      }),
    );
    window.history.pushState({}, "", "/classes");
    try {
      renderWithProviders(
        <SessionRestore>
          <p>App content</p>
        </SessionRestore>,
      );
      expect(await screen.findByText("App content")).toBeInTheDocument();
      expect(refreshRequested).toBe(true);
    } finally {
      window.history.pushState({}, "", "/");
    }
  });
});

describe("session hint", () => {
  it("is set on sign-in and cleared on sign-out", () => {
    useAuthStore.getState().setSession(primaryTeacher, "token");
    expect(window.localStorage.getItem("teka.hasSession")).toBe("1");
    useAuthStore.getState().clearSession();
    expect(window.localStorage.getItem("teka.hasSession")).toBeNull();
  });
});
