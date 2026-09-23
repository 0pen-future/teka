import { screen, within } from "@testing-library/react";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { PrepPage } from "../pages/prep-page";
import {
  getLibraryStore,
  libraryHandlers,
  resetLibraryStore,
  templateToan6,
  templateVan9,
  versionToan6Draft,
  versionVan9Draft,
} from "./library-handlers";

function renderPage() {
  return renderWithProviders(<PrepPage />, { route: "/prep", path: "/prep" });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("PrepPage", () => {
  it("lists only templates with an open draft, with progress and board links", async () => {
    // A template whose only version is published has no board to prepare.
    const store = getLibraryStore();
    store.templates.push({
      ...templateVan9,
      id: "80000000-0000-4000-8000-000000000009",
      code: "LY8",
      name: "Lý 8 nâng cao",
    });
    store.versions.push({
      ...versionVan9Draft,
      id: "81000000-0000-4000-8000-000000000009",
      template_id: "80000000-0000-4000-8000-000000000009",
      status: "published",
    });
    renderPage();

    const toan = await screen.findByRole("row", { name: /Toán 6 cơ bản/ });
    expect(within(toan).getByText("0/2 buổi")).toBeInTheDocument();
    expect(within(toan).getByRole("link", { name: "Bảng chuẩn bị" })).toHaveAttribute(
      "href",
      `/prep/${versionToan6Draft.id}/board`,
    );
    expect(within(toan).getByRole("link", { name: "Phân công" })).toHaveAttribute(
      "href",
      `/prep/${versionToan6Draft.id}/assign`,
    );
    expect(within(toan).getByRole("link", { name: "Toán 6 cơ bản" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}`,
    );
    expect(screen.getByRole("row", { name: /Văn 9 luyện thi/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /Lý 8 nâng cao/ })).not.toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Tạo chương trình" })).toHaveAttribute(
      "href",
      "/library/templates/new",
    );
  });

  it("hides the assign and create links from a member holding only library.read", async () => {
    server.use(memberWith("library.read"));
    renderPage();

    const toan = await screen.findByRole("row", { name: /Toán 6 cơ bản/ });
    expect(within(toan).getByRole("link", { name: "Bảng chuẩn bị" })).toBeInTheDocument();
    expect(within(toan).queryByRole("link", { name: "Phân công" })).not.toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Tạo chương trình" })).not.toBeInTheDocument();
  });

  it("shows the empty state when no template has a draft", async () => {
    server.use(
      http.get(`${API_URL}/library/templates`, () =>
        HttpResponse.json(ok([], { page: 1, per_page: 100, total: 0, total_pages: 0 })),
      ),
    );
    renderPage();

    expect(await screen.findByText("Chưa có bản nháp nào cần chuẩn bị")).toBeInTheDocument();
  });

  it("blocks a member without library.read", async () => {
    server.use(memberWith("tasks.list"));
    renderPage();

    expect(await screen.findByText("Bạn không có quyền xem mục này")).toBeInTheDocument();
  });
});
