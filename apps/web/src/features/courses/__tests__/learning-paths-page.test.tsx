import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { LearningPathsPage } from "../pages/learning-paths-page";
import { courseToan6, resetCoursesStore } from "./courses-handlers";
import { getPathsStore, pathsHandlers, pathToan, resetPathsStore } from "./paths-handlers";

function renderPage(route = "/paths") {
  return renderWithProviders(<LearningPathsPage />, {
    route,
    path: "/paths",
    extraRoutes: [{ path: "/courses/:id", element: <div>course-detail-stub</div> }],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function pathRow(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

function queryPathRow(name: string) {
  return screen.queryByRole("row", { name: new RegExp(name) });
}

function stageGroup(name: string) {
  return screen.getByRole("group", { name });
}

function toanStages() {
  return getPathsStore().paths.find((path) => path.id === pathToan.id)!.stages;
}

async function expandToan(user: ReturnType<typeof userEvent.setup>) {
  await screen.findByText("Lộ trình Toán THCS");
  await user.click(screen.getByRole("button", { name: "Quản lý chặng Lộ trình Toán THCS" }));
  await screen.findByRole("group", { name: "Chặng 1: Nền tảng" });
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
  it("lists every path as a row with status and counts, and expands its stages", async () => {
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByRole("heading", { name: "Lộ trình học" })).toBeInTheDocument();
    await screen.findByText("Lộ trình Toán THCS");

    const toan = pathRow("Lộ trình Toán THCS");
    expect(within(toan).getByText("Đang hoạt động")).toBeInTheDocument();
    // Two stages, one distinct course across them.
    const toanCells = within(toan).getAllByRole("cell");
    expect(toanCells[3]).toHaveTextContent("2");
    expect(toanCells[4]).toHaveTextContent("1");

    const van = pathRow("Lộ trình Văn luyện thi");
    expect(within(van).getByText("Nháp")).toBeInTheDocument();
    const vanCells = within(van).getAllByRole("cell");
    expect(vanCells[3]).toHaveTextContent("0");
    expect(vanCells[4]).toHaveTextContent("0");

    // The chip counts reflect the whole list, not the active filter.
    expect(screen.getByRole("radio", { name: /^Tất cả/ })).toHaveTextContent("2");
    expect(screen.getByRole("radio", { name: /^Nháp/ })).toHaveTextContent("1");

    await user.click(screen.getByRole("button", { name: "Quản lý chặng Lộ trình Toán THCS" }));
    const first = await screen.findByRole("group", { name: "Chặng 1: Nền tảng" });
    expect(within(first).getByRole("link", { name: "TOAN-6 · Toán 6 nền tảng" })).toHaveAttribute(
      "href",
      `/courses/${courseToan6.id}`,
    );
    expect(stageGroup("Chặng 2: Nâng cao")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thu gọn Lộ trình Toán THCS" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
  });

  it("filters by status through the query string", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByText("Lộ trình Toán THCS");

    await user.click(screen.getByRole("radio", { name: /^Nháp/ }));

    await waitFor(() => expect(queryPathRow("Lộ trình Toán THCS")).not.toBeInTheDocument());
    expect(pathRow("Lộ trình Văn luyện thi")).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: /^Nháp/ })).toHaveAttribute("aria-checked", "true");
    expect(router.state.location.search).toBe("?status=draft");

    await user.click(screen.getByRole("radio", { name: /^Tất cả/ }));
    expect(await screen.findByText("Lộ trình Toán THCS")).toBeInTheDocument();
    expect(router.state.location.search).toBe("");
  });

  it("creates a path from the dialog and opens its row", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByText("Lộ trình Toán THCS");

    await user.click(screen.getByRole("button", { name: "+ Tạo lộ trình" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo lộ trình" });
    await user.type(within(dialog).getByLabelText("Mã lộ trình"), "lt-ly");
    await user.type(within(dialog).getByLabelText("Tên lộ trình"), "Lộ trình Lý THCS");
    await user.type(within(dialog).getByLabelText("Mô tả"), "Từ lớp 8 tới lớp 9.");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await screen.findByText("Đã tạo lộ trình LT-LY")).toBeInTheDocument();
    expect(await screen.findByText("Lộ trình Lý THCS")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thu gọn Lộ trình Lý THCS" })).toHaveAttribute(
      "aria-expanded",
      "true",
    );
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
    await screen.findByText("Lộ trình Toán THCS");

    await user.click(screen.getByRole("button", { name: "+ Tạo lộ trình" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo lộ trình" });
    await user.type(within(dialog).getByLabelText("Mã lộ trình"), "LT-TOAN");
    await user.type(within(dialog).getByLabelText("Tên lộ trình"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(
      await within(dialog).findByText("Mã lộ trình đã được dùng trong trung tâm"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Mã lộ trình")).toHaveAttribute("aria-invalid", "true");
  });

  it("adds a stage at the end of the path through the dialog", async () => {
    const user = userEvent.setup();
    renderPage();
    await expandToan(user);

    await user.click(screen.getByRole("button", { name: "+ Thêm chặng" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm chặng" });
    await user.type(within(dialog).getByLabelText("Tên chặng"), "Ôn thi vào 10");
    await user.type(within(dialog).getByLabelText("Mục tiêu"), "Luyện đề tổng hợp.");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(
      await screen.findByRole("group", { name: "Chặng 3: Ôn thi vào 10" }),
    ).toBeInTheDocument();
    expect(toanStages()[2]).toMatchObject({
      name: "Ôn thi vào 10",
      goal: "Luyện đề tổng hợp.",
      position: 3,
    });
  });

  it("renames and deletes a stage, keeping positions dense", async () => {
    const user = userEvent.setup();
    renderPage();
    await expandToan(user);

    await user.click(
      within(stageGroup("Chặng 1: Nền tảng")).getByRole("button", { name: "Nền tảng" }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Đổi tên chặng" });
    const name = within(dialog).getByLabelText("Tên chặng");
    await user.clear(name);
    await user.type(name, "Khởi đầu");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));
    expect(await screen.findByRole("group", { name: "Chặng 1: Khởi đầu" })).toBeInTheDocument();

    await user.click(
      within(stageGroup("Chặng 1: Khởi đầu")).getByRole("button", {
        name: "Xoá chặng Khởi đầu",
      }),
    );
    const confirm = await screen.findByRole("dialog", { name: 'Xoá chặng "Khởi đầu"?' });
    await user.click(within(confirm).getByRole("button", { name: "Xoá chặng" }));

    expect(await screen.findByRole("group", { name: "Chặng 1: Nâng cao" })).toBeInTheDocument();
    expect(screen.queryByRole("group", { name: /Khởi đầu/ })).not.toBeInTheDocument();
    expect(toanStages().map((stage) => stage.position)).toEqual([1]);
  });

  it("moves a stage down with the reorder buttons", async () => {
    const user = userEvent.setup();
    renderPage();
    await expandToan(user);

    const first = stageGroup("Chặng 1: Nền tảng");
    expect(within(first).getByRole("button", { name: "Đưa Nền tảng lên" })).toBeDisabled();
    await user.click(within(first).getByRole("button", { name: "Đưa Nền tảng xuống" }));

    expect(await screen.findByRole("group", { name: "Chặng 2: Nền tảng" })).toBeInTheDocument();
    expect(stageGroup("Chặng 1: Nâng cao")).toBeInTheDocument();
    expect(toanStages().map((stage) => stage.name)).toEqual(["Nâng cao", "Nền tảng"]);
  });

  it("hides every editing control from a member who can only read", async () => {
    server.use(memberWith("paths.read"));
    const user = userEvent.setup();
    renderPage();
    await expandToan(user);

    expect(screen.queryByRole("button", { name: "+ Tạo lộ trình" })).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Sửa Lộ trình Toán THCS" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Thêm chặng" })).not.toBeInTheDocument();
    const first = stageGroup("Chặng 1: Nền tảng");
    expect(within(first).queryByRole("button")).not.toBeInTheDocument();
    // Without courses.read the course chip stays plain text.
    expect(within(first).getByText("TOAN-6 · Toán 6 nền tảng")).toBeInTheDocument();
    expect(within(first).queryByRole("link")).not.toBeInTheDocument();
  });

  it("shows the empty row when no path matches the filter", async () => {
    renderPage("/paths?status=archived");
    expect(await screen.findByText("Không có lộ trình nào ở trạng thái này.")).toBeInTheDocument();
  });
});
