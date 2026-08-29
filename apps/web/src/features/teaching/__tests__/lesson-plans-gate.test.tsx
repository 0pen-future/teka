import { screen, waitFor } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { LessonPlansPage } from "../pages/lesson-plans-page";

function renderPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<LessonPlansPage />, {
    route: "/lesson-plans",
    path: "/lesson-plans",
    extraRoutes: [{ path: "/classbook", element: <div>classbook fallback</div> }],
  });
}

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("lesson plans owner gate", () => {
  it("renders the page for an owner", async () => {
    // Default /centers/me handler is owner-shaped.
    renderPage();
    expect(
      await screen.findByRole("heading", { level: 1, name: "Duyệt giáo án" }),
    ).toBeInTheDocument();
  });

  it("redirects to /classbook when /centers/me fails instead of blanking", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    const { router } = renderPage();
    await waitFor(() => expect(router.state.location.pathname).toBe("/classbook"));
  });

  it("renders the queue for a member granted teaching.review_queue", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(
          ok({
            center_name: "Trung Tâm Bình Minh",
            permissions: ["teaching.review_queue"],
          }),
        ),
      ),
    );
    renderPage();
    expect(
      await screen.findByRole("heading", { level: 1, name: "Duyệt giáo án" }),
    ).toBeInTheDocument();
  });

  it("redirects a non-owner to /classbook", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
      ),
    );
    const { router } = renderPage();
    await waitFor(() => expect(router.state.location.pathname).toBe("/classbook"));
    expect(screen.getByText("classbook fallback")).toBeInTheDocument();
  });
});
