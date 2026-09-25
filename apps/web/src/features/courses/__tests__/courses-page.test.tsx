import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
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
  server.use(...coursesHandlers, ...pathsHandlers);
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
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "ly-8");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Lý 8 nâng cao");
    await user.type(within(dialog).getByLabelText("Môn học"), "Vật lý");
    const price = within(dialog).getByLabelText("Đơn giá / buổi (đ)");
    await user.clear(price);
    await user.type(price, "150000");
    await user.type(within(dialog).getByLabelText("Tổng số buổi"), "24");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await screen.findByText("course-detail-stub")).toBeInTheDocument();
    const created = getCoursesStore().courses.find((course) => course.code === "LY-8");
    expect(created).toMatchObject({
      name: "Lý 8 nâng cao",
      subject: "Vật lý",
      level: null,
      status: "draft",
      default_unit_price: 150000,
      total_sessions: 24,
      duration_min: null,
      default_template_version_id: null,
    });
  });

  it("puts a duplicate code on the code field", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "+ Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "TOAN-6");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(
      await within(dialog).findByText("Mã khóa học đã được dùng trong trung tâm"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Mã khóa học")).toHaveAttribute("aria-invalid", "true");
  });

  it("edits a course from its row without opening the detail page", async () => {
    const user = userEvent.setup();
    renderPage();
    const toan = await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(within(toan).getByRole("button", { name: "Sửa Toán 6 nền tảng" }));
    expect(await screen.findByRole("dialog", { name: "Sửa khóa học" })).toBeInTheDocument();
    expect(screen.queryByText("course-detail-stub")).not.toBeInTheDocument();
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
