import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { LearningPathsPage } from "../pages/learning-paths-page";
import { resetCoursesStore } from "./courses-handlers";
import { getPathsStore, pathsHandlers, pathToan, resetPathsStore } from "./paths-handlers";

function renderPage(route = "/paths") {
  return renderWithProviders(<LearningPathsPage />, {
    route,
    path: "/paths",
    extraRoutes: [{ path: "/paths/:id", element: <div>path-detail-stub</div> }],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

beforeEach(() => {
  resetCoursesStore();
  resetPathsStore();
  server.use(...pathsHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("LearningPathsPage", () => {
  it("lists every path as a card with code, status and counts, linking to the detail page", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Lộ trình học" })).toBeInTheDocument();

    const toan = await screen.findByRole("article", { name: "Lộ trình Toán THCS" });
    expect(within(toan).getByRole("link", { name: "Lộ trình Toán THCS" })).toHaveAttribute(
      "href",
      `/paths/${pathToan.id}`,
    );
    expect(within(toan).getByText("LT-TOAN")).toBeInTheDocument();
    expect(within(toan).getByText("Đang dùng")).toBeInTheDocument();
    expect(within(toan).getByText("2 giai đoạn · 1 khóa học")).toBeInTheDocument();
    expect(within(toan).getByText("Từ nền tảng lớp 6 tới ôn thi vào 10.")).toBeInTheDocument();

    const van = screen.getByRole("article", { name: "Lộ trình Văn luyện thi" });
    expect(within(van).getByText("Đang soạn")).toBeInTheDocument();
    expect(within(van).getByText("Chưa có giai đoạn")).toBeInTheDocument();
  });

  it("filters by status through the query string", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByRole("article", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("tab", { name: "Đang soạn" }));

    await waitFor(() =>
      expect(screen.queryByRole("article", { name: "Lộ trình Toán THCS" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("article", { name: "Lộ trình Văn luyện thi" })).toBeInTheDocument();
    expect(router.state.location.search).toBe("?status=draft");
  });

  it("searches by name after the debounce", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Lộ trình Toán THCS" });

    await user.type(screen.getByRole("searchbox", { name: "Tìm lộ trình" }), "văn");

    await waitFor(() =>
      expect(screen.queryByRole("article", { name: "Lộ trình Toán THCS" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("article", { name: "Lộ trình Văn luyện thi" })).toBeInTheDocument();
  });

  it("creates a path from the dialog and navigates to it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("button", { name: "Tạo lộ trình" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo lộ trình" });
    await user.type(within(dialog).getByLabelText("Mã lộ trình"), "lt-ly");
    await user.type(within(dialog).getByLabelText("Tên lộ trình"), "Lộ trình Lý THCS");
    await user.type(within(dialog).getByLabelText("Mô tả"), "Từ lớp 8 tới lớp 9.");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await screen.findByText("path-detail-stub")).toBeInTheDocument();
    const created = getPathsStore().paths.find((path) => path.code === "LT-LY");
    expect(created).toMatchObject({
      name: "Lộ trình Lý THCS",
      description: "Từ lớp 8 tới lớp 9.",
      status: "draft",
      stages: [],
    });
  });

  it("puts a duplicate code on the code field", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("button", { name: "Tạo lộ trình" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo lộ trình" });
    await user.type(within(dialog).getByLabelText("Mã lộ trình"), "LT-TOAN");
    await user.type(within(dialog).getByLabelText("Tên lộ trình"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(
      await within(dialog).findByText("Mã lộ trình đã được dùng trong trung tâm"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Mã lộ trình")).toHaveAttribute("aria-invalid", "true");
  });

  it("hides the create button from a member who can only read", async () => {
    server.use(memberWith("paths.read"));
    renderPage();
    await screen.findByRole("article", { name: "Lộ trình Toán THCS" });
    expect(screen.queryByRole("button", { name: "Tạo lộ trình" })).not.toBeInTheDocument();
  });

  it("shows the empty block when no path matches the filter", async () => {
    renderPage("/paths?status=archived");
    expect(await screen.findByText("Không có lộ trình nào ở trạng thái này.")).toBeInTheDocument();
  });
});
