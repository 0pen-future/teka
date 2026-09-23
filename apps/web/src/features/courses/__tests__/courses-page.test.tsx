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
  server.use(...coursesHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CoursesPage", () => {
  it("lists every course with code, subject, template and class counts, linking to the detail page", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Danh mục khóa học" })).toBeInTheDocument();

    const toan = await screen.findByRole("row", { name: /Toán 6 nền tảng/ });
    expect(within(toan).getByRole("link", { name: "Toán 6 nền tảng" })).toHaveAttribute(
      "href",
      `/courses/${courseToan6.id}`,
    );
    expect(within(toan).getByText("TOAN-6")).toBeInTheDocument();
    expect(within(toan).getByText("Toán")).toBeInTheDocument();
    expect(within(toan).getByText("Lớp 6")).toBeInTheDocument();
    expect(within(toan).getByText("Toán 6 cơ bản · v1")).toBeInTheDocument();
    expect(within(toan).getByText("1 đang học")).toBeInTheDocument();
    expect(within(toan).getByText("Đang mở")).toBeInTheDocument();

    const van = screen.getByRole("row", { name: /Văn 9 luyện thi/ });
    expect(within(van).getByText("Đang soạn")).toBeInTheDocument();
    expect(within(van).getAllByText("—")).toHaveLength(2);
    expect(within(van).getByText("Chưa có lớp")).toBeInTheDocument();
  });

  it("filters by status through the query string", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("tab", { name: "Đang soạn" }));

    await waitFor(() =>
      expect(screen.queryByRole("row", { name: /Toán 6 nền tảng/ })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("row", { name: /Văn 9 luyện thi/ })).toBeInTheDocument();
    expect(router.state.location.search).toBe("?status=draft");
  });

  it("searches by name after the debounce", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm khóa học" }), "văn");

    await waitFor(() =>
      expect(screen.queryByRole("row", { name: /Toán 6 nền tảng/ })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("row", { name: /Văn 9 luyện thi/ })).toBeInTheDocument();
  });

  it("creates a course from the dialog and navigates to it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });

    await user.click(screen.getByRole("button", { name: "Tạo khóa học" }));
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

    await user.click(screen.getByRole("button", { name: "Tạo khóa học" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo khóa học" });
    await user.type(within(dialog).getByLabelText("Mã khóa học"), "TOAN-6");
    await user.type(within(dialog).getByLabelText("Tên khóa học"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(
      await within(dialog).findByText("Mã khóa học đã được dùng trong trung tâm"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Mã khóa học")).toHaveAttribute("aria-invalid", "true");
  });

  it("hides the create button from a member who can only read", async () => {
    server.use(memberWith("courses.read"));
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 nền tảng/ });
    expect(screen.queryByRole("button", { name: "Tạo khóa học" })).not.toBeInTheDocument();
  });

  it("shows the empty block when no course matches the filter", async () => {
    renderPage("/courses?status=archived");
    expect(await screen.findByText("Không có khóa học nào ở trạng thái này.")).toBeInTheDocument();
  });
});
