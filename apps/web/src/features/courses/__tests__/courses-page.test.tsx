import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { libraryHandlers, resetLibraryStore } from "@/features/library/__tests__/library-handlers";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CoursesPage } from "../pages/courses-page";
import {
  coursesHandlers,
  courseToan6,
  getCoursesStore,
  resetCoursesStore,
} from "./courses-handlers";
import { pathsHandlers, resetPathsStore } from "./paths-handlers";

function renderPage(route = "/courses") {
  return renderWithProviders(<CoursesPage />, {
    route,
    path: "/courses",
    extraRoutes: [{ path: "/courses/:id", element: <div>course-detail-stub</div> }],
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
  resetLibraryStore();
  // The library handlers serve the dialog's template and version pickers.
  server.use(...coursesHandlers, ...pathsHandlers, ...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CoursesPage", () => {
  it("lists every course with code, stage, duration, price, template and status, opening the detail page", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    expect(await screen.findByRole("heading", { name: "Khóa học" })).toBeInTheDocument();

    const toan = await screen.findByRole("row", { name: /Toán 6 nền tảng/ });
    expect(within(toan).getByText("TOAN-6")).toBeInTheDocument();
    expect(await within(toan).findByText("Nền tảng")).toBeInTheDocument();
    expect(within(toan).getByText("90 phút")).toBeInTheDocument();
    expect(within(toan).getByText("1 lớp")).toBeInTheDocument();
    expect(within(toan).getByText("Toán 6 cơ bản · v1")).toBeInTheDocument();
    expect(within(toan).getByText("Đang hoạt động")).toBeInTheDocument();

    const van = screen.getByRole("row", { name: /Văn 9 luyện thi/ });
    expect(within(van).getByText("Nháp")).toBeInTheDocument();
    expect(within(van).getByText("Chưa gắn")).toBeInTheDocument();
    expect(within(van).getAllByText("—")).toHaveLength(3);

    await user.click(within(toan).getByText("Toán 6 nền tảng"));
    expect(await screen.findByText("course-detail-stub")).toBeInTheDocument();
    expect(router.state.location.pathname).toBe(`/courses/${courseToan6.id}`);
  });

  it("filters by status through the query string, with counts on each chip", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    expect(screen.getByRole("radio", { name: /Tất cả/ })).toHaveTextContent("2");
    await user.click(screen.getByRole("radio", { name: /Đang hoạt động/ }));

    await waitFor(() =>
      expect(screen.queryByRole("row", { name: /Văn 9 luyện thi/ })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("row", { name: /Toán 6 nền tảng/ })).toBeInTheDocument();
    expect(router.state.location.search).toBe("?status=active");
  });

  it("searches by name or code as you type", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm khóa học" }), "van-9");

    await waitFor(() =>
      expect(screen.queryByRole("row", { name: /Toán 6 nền tảng/ })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("row", { name: /Văn 9 luyện thi/ })).toBeInTheDocument();
  });

  it("creates a course from the dialog and navigates to it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "+ Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    expect(
      within(dialog).getByText(
        "Mã không đổi sau khi tạo. Lịch dạy, giáo viên, học phí thuộc lớp — không thuộc khóa.",
      ),
    ).toBeInTheDocument();
    expect(within(dialog).getByText("Viết hoa, không dấu")).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();

    await user.type(within(dialog).getByLabelText("Mã khóa học"), "ly-8");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Lý 8 nâng cao");
    await user.type(within(dialog).getByLabelText("Thời lượng buổi (phút)"), "90");
    const price = within(dialog).getByLabelText("Giá / buổi (đ)");
    await user.clear(price);
    await user.type(price, "150000");
    await user.type(within(dialog).getByLabelText("Môn học"), "Vật lý");
    await user.type(within(dialog).getByLabelText("Tổng số buổi"), "24");
    await user.click(within(dialog).getByRole("button", { name: "Tạo mới" }));

    expect(await screen.findByText("course-detail-stub")).toBeInTheDocument();
    const created = getCoursesStore().courses.find((course) => course.code === "LY-8");
    expect(created).toMatchObject({
      name: "Lý 8 nâng cao",
      subject: "Vật lý",
      level: null,
      status: "draft",
      default_unit_price: 150000,
      total_sessions: 24,
      duration_min: 90,
      default_template_version_id: null,
    });
  });

  it("binds a template's newest published version when creating", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "+ Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "TOAN-6B");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Toán 6 buổi tối");
    expect(within(dialog).getByRole("combobox", { name: "Phiên bản mặc định" })).toBeDisabled();

    await user.click(within(dialog).getByRole("combobox", { name: "Chương trình mẫu" }));
    await user.click(await screen.findByRole("option", { name: /Toán 6 cơ bản/ }));
    // Only the published version is offered; the open draft cannot be bound.
    const version = within(dialog).getByRole("combobox", { name: "Phiên bản mặc định" });
    await waitFor(() => expect(version).toHaveTextContent("v1 — Đã phát hành"));
    await user.click(version);
    expect(screen.queryByRole("option", { name: /v2/ })).not.toBeInTheDocument();
    await user.keyboard("{Escape}");

    await user.click(within(dialog).getByRole("button", { name: "Tạo mới" }));
    expect(await screen.findByText("course-detail-stub")).toBeInTheDocument();
    expect(
      getCoursesStore().courses.find((course) => course.code === "TOAN-6B")
        ?.default_template_version_id,
    ).toBe("81000000-0000-4000-8000-000000000001");
  });

  it("refuses a template that has no published version yet", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "+ Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "VAN-9B");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Văn 9 buổi tối");
    await user.click(within(dialog).getByRole("combobox", { name: "Chương trình mẫu" }));
    await user.click(await screen.findByRole("option", { name: /Văn 9 luyện thi/ }));
    await waitFor(() =>
      expect(
        within(dialog).getByRole("combobox", { name: "Phiên bản mặc định" }),
      ).toHaveTextContent("Chưa có phiên bản đã phát hành"),
    );
    await user.click(within(dialog).getByRole("button", { name: "Tạo mới" }));

    expect(
      await within(dialog).findByText("Chọn phiên bản mặc định cho chương trình mẫu"),
    ).toBeInTheDocument();
    expect(getCoursesStore().courses.some((course) => course.code === "VAN-9B")).toBe(false);
  });

  it("puts a duplicate code on the code field", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "+ Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "TOAN-6");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo mới" }));

    expect(
      await within(dialog).findByText("Mã khóa học đã được dùng trong trung tâm"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Mã khóa học")).toHaveAttribute("aria-invalid", "true");
  });

  it("edits a course from its row with the code locked, without opening the detail page", async () => {
    const user = userEvent.setup();
    renderPage();
    const toan = await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(within(toan).getByRole("button", { name: "Sửa Toán 6 nền tảng" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa khóa học" });
    expect(screen.queryByText("course-detail-stub")).not.toBeInTheDocument();
    const code = within(dialog).getByLabelText("Mã khóa học");
    expect(code).toHaveValue("TOAN-6");
    expect(code).toHaveAttribute("readonly");
    expect(within(dialog).getByText("Không đổi được mã sau khi tạo")).toBeInTheDocument();
    expect(within(dialog).getByText("Lớp đang chạy giữ phiên bản cũ")).toBeInTheDocument();
    expect(
      await within(dialog).findByRole("combobox", { name: "Phiên bản mặc định" }),
    ).toHaveTextContent("v1 — Đã phát hành");

    const duration = within(dialog).getByLabelText("Thời lượng buổi (phút)");
    await user.clear(duration);
    await user.type(duration, "120");
    await user.click(within(dialog).getByRole("button", { name: "Lưu thay đổi" }));

    expect(await screen.findByText("Đã lưu khóa học")).toBeInTheDocument();
    const saved = getCoursesStore().courses.find((course) => course.id === courseToan6.id);
    expect(saved).toMatchObject({
      code: "TOAN-6",
      duration_min: 120,
      default_template_version_id: courseToan6.default_template_version_id,
    });
  });

  it("deletes a course from the edit dialog after confirming", async () => {
    const user = userEvent.setup();
    renderPage();
    const van = await screen.findByRole("row", { name: /Văn 9 luyện thi/ });

    await user.click(within(van).getByRole("button", { name: "Sửa Văn 9 luyện thi" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa khóa học" });
    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));
    const confirm = await screen.findByRole("dialog", { name: 'Xoá khóa "Văn 9 luyện thi"?' });

    // Cancelling the confirmation returns to the edit form.
    await user.click(within(confirm).getByRole("button", { name: "Hủy" }));
    expect(screen.getByRole("dialog", { name: "Sửa khóa học" })).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: /^Xoá khóa/ })).getByRole("button", {
        name: "Xoá khóa",
      }),
    );

    expect(await screen.findByText("Đã xoá khóa VAN-9")).toBeInTheDocument();
    await waitFor(() =>
      expect(screen.queryByRole("row", { name: /Văn 9 luyện thi/ })).not.toBeInTheDocument(),
    );
    expect(screen.queryByRole("dialog", { name: "Sửa khóa học" })).not.toBeInTheDocument();
  });

  it("reports the server's refusal to delete a course with classes", async () => {
    const user = userEvent.setup();
    renderPage();
    const toan = await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(within(toan).getByRole("button", { name: "Sửa Toán 6 nền tảng" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa khóa học" });
    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));
    await user.click(
      within(await screen.findByRole("dialog", { name: /^Xoá khóa/ })).getByRole("button", {
        name: "Xoá khóa",
      }),
    );

    expect(
      await screen.findByText("Khóa học đang có lớp gắn vào, hãy lưu trữ thay vì xoá"),
    ).toBeInTheDocument();
    expect(getCoursesStore().courses.some((course) => course.id === courseToan6.id)).toBe(true);
    expect(screen.getByRole("dialog", { name: "Sửa khóa học" })).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: /^Xoá khóa/ })).not.toBeInTheDocument();
  });

  it("hides the create and edit buttons from a member who can only read", async () => {
    server.use(memberWith("courses.read"));
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });
    expect(screen.queryByRole("button", { name: "+ Tạo khóa học" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Sửa / })).not.toBeInTheDocument();
  });

  it("says so inside the table when no course matches the filter", async () => {
    renderPage("/courses?status=archived");
    expect(await screen.findByText("Không có khóa nào khớp.")).toBeInTheDocument();
  });
});
