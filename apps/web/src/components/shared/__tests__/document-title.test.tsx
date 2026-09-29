import { render, waitFor } from "@testing-library/react";
import { createMemoryRouter, Outlet, RouterProvider, type RouteObject } from "react-router";
import { afterEach, describe, expect, it } from "vitest";

import { RouteDocumentTitle } from "../document-title";

const routes: RouteObject[] = [
  {
    element: (
      <>
        <RouteDocumentTitle />
        <Outlet />
      </>
    ),
    children: [
      {
        path: "sessions",
        handle: { title: "Điểm danh" },
        element: <Outlet />,
        children: [{ path: ":id/attendance", handle: { title: "Điểm danh buổi học" } }],
      },
      { path: "untitled" },
    ],
  },
];

function renderAt(path: string) {
  const router = createMemoryRouter(routes, { initialEntries: [path] });
  render(<RouterProvider router={router} />);
  return router;
}

describe("RouteDocumentTitle", () => {
  afterEach(() => {
    document.title = "";
  });

  it("titles the tab from the matched route", async () => {
    renderAt("/sessions");
    await waitFor(() => expect(document.title).toBe("Điểm danh · Teka"));
  });

  it("prefers the deepest route's title over its parent's", async () => {
    renderAt("/sessions/abc/attendance");
    await waitFor(() => expect(document.title).toBe("Điểm danh buổi học · Teka"));
  });

  it("falls back to the app name when no route has a title", async () => {
    renderAt("/untitled");
    await waitFor(() => expect(document.title).toBe("Teka"));
  });

  it("follows navigation", async () => {
    const router = renderAt("/untitled");
    await router.navigate("/sessions");
    await waitFor(() => expect(document.title).toBe("Điểm danh · Teka"));
  });
});
