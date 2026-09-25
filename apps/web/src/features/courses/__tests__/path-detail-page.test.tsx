import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { PathDetailPage } from "../pages/path-detail-page";
import { coursesHandlers, courseToan6, resetCoursesStore } from "./courses-handlers";
import { getPathsStore, pathsHandlers, pathToan, pathVan, resetPathsStore } from "./paths-handlers";

function renderPage(route = `/paths/${pathToan.id}`) {
  return renderWithProviders(<PathDetailPage />, {
    route,
    path: "/paths/:id",
    extraRoutes: [
      { path: "/paths", element: <div>paths-list-stub</div> },
      { path: "/courses/:id", element: <div>course-detail-stub</div> },
    ],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function stageOf(path: typeof pathToan, index: number) {
  return getPathsStore().paths.find((item) => item.id === path.id)!.stages[index]!;
}

function stageRegion(name: string) {
  return screen.getByRole("region", { name });
}

beforeEach(() => {
  resetCoursesStore();
  resetPathsStore();
  // Course handlers feed the course picker; path handlers own everything else.
  server.use(...coursesHandlers, ...pathsHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("PathDetailPage", () => {
  it("renders the header, the description and the stage timeline with course chips", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Lộ trình Toán THCS" })).toBeInTheDocument();
    expect(screen.getByText("Mã: LT-TOAN")).toBeInTheDocument();
    expect(screen.getByText("Đang hoạt động")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Lộ trình học" })).toHaveAttribute("href", "/paths");
    expect(screen.getByText("Từ nền tảng lớp 6 tới ôn thi vào 10.")).toBeInTheDocument();

    const first = stageRegion("Giai đoạn 1: Nền tảng");
    expect(within(first).getByText("Nắm chắc số học và hình học cơ bản.")).toBeInTheDocument();
    expect(within(first).getByRole("link", { name: "Toán 6 nền tảng" })).toHaveAttribute(
      "href",
      `/courses/${courseToan6.id}`,
    );

    const second = stageRegion("Giai đoạn 2: Nâng cao");
    expect(within(second).getByText("Chưa gắn khóa học nào.")).toBeInTheDocument();
  });

  it("shows the not-found block for an unknown path", async () => {
    renderPage("/paths/00000000-0000-4000-8000-000000000404");
    expect(await screen.findByText("Không tìm thấy lộ trình")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Về danh sách lộ trình" })).toHaveAttribute(
      "href",
      "/paths",
    );
  });

  it("shows the empty timeline for a path with no stage", async () => {
    renderPage(`/paths/${pathVan.id}`);
    expect(await screen.findByText("Chưa có giai đoạn nào.")).toBeInTheDocument();
  });

  it("adds a stage through the dialog", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("button", { name: "Thêm giai đoạn" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm chặng" });
    await user.type(within(dialog).getByLabelText("Tên chặng"), "Ôn thi vào 10");
    await user.type(within(dialog).getByLabelText("Mục tiêu"), "Luyện đề tổng hợp.");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await screen.findByRole("region", { name: "Giai đoạn 3: Ôn thi vào 10" }));
    expect(stageOf(pathToan, 2)).toMatchObject({
      name: "Ôn thi vào 10",
      goal: "Luyện đề tổng hợp.",
      position: 3,
    });
  });

  it("edits and deletes a stage, keeping positions dense", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    await user.click(
      within(stageRegion("Giai đoạn 1: Nền tảng")).getByRole("button", {
        name: "Sửa giai đoạn Nền tảng",
      }),
    );
    const dialog = await screen.findByRole("dialog", { name: "Đổi tên chặng" });
    const name = within(dialog).getByLabelText("Tên chặng");
    await user.clear(name);
    await user.type(name, "Khởi đầu");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));
    expect(await screen.findByRole("region", { name: "Giai đoạn 1: Khởi đầu" }));

    await user.click(
      within(stageRegion("Giai đoạn 1: Khởi đầu")).getByRole("button", {
        name: "Xoá giai đoạn Khởi đầu",
      }),
    );
    const confirm = await screen.findByRole("dialog", { name: 'Xoá giai đoạn "Khởi đầu"?' });
    await user.click(within(confirm).getByRole("button", { name: "Xoá giai đoạn" }));

    expect(await screen.findByRole("region", { name: "Giai đoạn 1: Nâng cao" }));
    expect(screen.queryByRole("region", { name: /Khởi đầu/ })).not.toBeInTheDocument();
    expect(getPathsStore().paths[0]!.stages.map((stage) => stage.position)).toEqual([1]);
  });

  it("moves a stage down and up with the reorder buttons", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    // Every control names its stage, so a screen reader's button list
    // does not read as N identical "Lên"/"Xuống" entries.
    const first = stageRegion("Giai đoạn 1: Nền tảng");
    expect(within(first).getByRole("button", { name: "Đưa Nền tảng lên" })).toBeDisabled();
    await user.click(within(first).getByRole("button", { name: "Đưa Nền tảng xuống" }));

    expect(await screen.findByRole("region", { name: "Giai đoạn 2: Nền tảng" }));
    expect(stageRegion("Giai đoạn 1: Nâng cao")).toBeInTheDocument();
    expect(stageOf(pathToan, 0).name).toBe("Nâng cao");

    const moved = stageRegion("Giai đoạn 2: Nền tảng");
    expect(within(moved).getByRole("button", { name: "Đưa Nền tảng xuống" })).toBeDisabled();
    await user.click(within(moved).getByRole("button", { name: "Đưa Nền tảng lên" }));
    expect(await screen.findByRole("region", { name: "Giai đoạn 1: Nền tảng" }));
    expect(stageOf(pathToan, 0).name).toBe("Nền tảng");
  });

  it("adds an active course to a stage from the picker and removes one", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    // Only active courses are offered, and Toán 6 already sits in stage 1.
    const first = stageRegion("Giai đoạn 1: Nền tảng");
    await user.click(within(first).getByRole("combobox", { name: "Thêm khóa học vào Nền tảng" }));
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: /Toán 6/ })).not.toBeInTheDocument();
    expect(within(listbox).queryByRole("option", { name: /Văn 9/ })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");

    const second = stageRegion("Giai đoạn 2: Nâng cao");
    await user.click(within(second).getByRole("combobox", { name: "Thêm khóa học vào Nâng cao" }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: /Toán 6/ }),
    );
    expect(
      await within(stageRegion("Giai đoạn 2: Nâng cao")).findByRole("link", {
        name: "Toán 6 nền tảng",
      }),
    ).toBeInTheDocument();
    expect(stageOf(pathToan, 1).courses.map((course) => course.id)).toEqual([courseToan6.id]);

    await user.click(
      within(stageRegion("Giai đoạn 1: Nền tảng")).getByRole("button", {
        name: "Gỡ Toán 6 nền tảng",
      }),
    );
    expect(
      await within(stageRegion("Giai đoạn 1: Nền tảng")).findByText("Chưa gắn khóa học nào."),
    ).toBeInTheDocument();
    expect(stageOf(pathToan, 0).courses).toEqual([]);
  });

  it("edits the path from the dialog", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("button", { name: "Sửa lộ trình" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa lộ trình" });
    const name = within(dialog).getByLabelText("Tên lộ trình");
    await user.clear(name);
    await user.type(name, "Lộ trình Toán 6-9");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByRole("heading", { name: "Lộ trình Toán 6-9" })).toBeInTheDocument();
    expect(getPathsStore().paths[0]!.name).toBe("Lộ trình Toán 6-9");
  });

  it("deletes the path after confirmation and returns to the list", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    await user.click(screen.getByRole("button", { name: "Xoá lộ trình" }));
    const confirm = await screen.findByRole("dialog", {
      name: 'Xoá lộ trình "Lộ trình Toán THCS"?',
    });
    await user.click(within(confirm).getByRole("button", { name: "Xoá lộ trình" }));

    expect(await screen.findByText("paths-list-stub")).toBeInTheDocument();
    expect(getPathsStore().paths.map((path) => path.id)).toEqual([pathVan.id]);
  });

  it("tells an editor without courses.read that the catalog is unavailable and keeps chips plain", async () => {
    server.use(
      memberWith("paths.read", "paths.edit"),
      http.get(`${API_URL}/courses`, () =>
        HttpResponse.json(fail("FORBIDDEN", "forbidden"), { status: 403 }),
      ),
    );
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    expect(await screen.findByRole("alert")).toHaveTextContent("Không tải được danh mục khóa học.");
    const first = stageRegion("Giai đoạn 1: Nền tảng");
    expect(
      within(first).getByRole("combobox", { name: "Thêm khóa học vào Nền tảng" }),
    ).toBeDisabled();
    expect(within(first).getByText("Toán 6 nền tảng")).toBeInTheDocument();
    expect(within(first).queryByRole("link", { name: "Toán 6 nền tảng" })).not.toBeInTheDocument();
    // Removing a held course never needs the catalog.
    expect(within(first).getByRole("button", { name: "Gỡ Toán 6 nền tảng" })).toBeInTheDocument();
  });

  it("hides every editing control from a member who can only read", async () => {
    server.use(memberWith("paths.read", "courses.read"));
    renderPage();
    await screen.findByRole("heading", { name: "Lộ trình Toán THCS" });

    expect(screen.queryByRole("button", { name: "Sửa lộ trình" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Xoá lộ trình" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm giai đoạn" })).not.toBeInTheDocument();
    const first = stageRegion("Giai đoạn 1: Nền tảng");
    expect(within(first).queryByRole("button")).not.toBeInTheDocument();
    expect(within(first).queryByRole("combobox")).not.toBeInTheDocument();
    expect(within(first).getByRole("link", { name: "Toán 6 nền tảng" })).toBeInTheDocument();
  });
});
