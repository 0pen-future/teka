import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { libraryHandlers, resetLibraryStore } from "@/features/library/__tests__/library-handlers";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { CourseDetailPage } from "../pages/course-detail-page";
import {
  coursesHandlers,
  courseToan6,
  courseVan9,
  getCoursesStore,
  resetCoursesStore,
} from "./courses-handlers";
import { pathsHandlers, resetPathsStore } from "./paths-handlers";

function renderPage(route = `/courses/${courseToan6.id}`) {
  return renderWithProviders(<CourseDetailPage />, {
    route,
    path: "/courses/:id",
    extraRoutes: [
      { path: "/courses", element: <div>courses-list-stub</div> },
      { path: "/classes/:id", element: <div>class-detail-stub</div> },
    ],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

/** The course's own audit trail: one change by the owner, one failed write that is not a change. */
const courseLogs = [
  {
    id: "b1000000-0000-4000-8000-000000000001",
    occurred_at: "2026-09-10T08:00:00Z",
    actor_user_id: "aa000000-0000-4000-8000-000000000001",
    actor_name: "Cô Lan",
    actor_role: "owner",
    action: "course.set_tuition_packs",
    method: "PUT",
    path: `/api/v1/courses/${courseToan6.id}/tuition-packs`,
    entity_type: "course",
    entity_id: courseToan6.id,
    status_code: 200,
    ip: "203.0.113.10",
    user_agent: "Mozilla/5.0",
    metadata: null,
  },
  {
    id: "b1000000-0000-4000-8000-000000000002",
    occurred_at: "2026-09-09T08:00:00Z",
    actor_user_id: "aa000000-0000-4000-8000-000000000001",
    actor_name: "Cô Lan",
    actor_role: "owner",
    action: "course.update",
    method: "PUT",
    path: `/api/v1/courses/${courseToan6.id}`,
    entity_type: "course",
    entity_id: courseToan6.id,
    status_code: 422,
    ip: "203.0.113.10",
    user_agent: "Mozilla/5.0",
    metadata: null,
  },
];

const auditHandler = http.get(`${API_URL}/audit-logs`, ({ request }) => {
  const entityId = new URL(request.url).searchParams.get("entity_id");
  const items = courseLogs.filter((log) => log.entity_id === entityId);
  return HttpResponse.json(ok({ items, next_cursor: "" }));
});

function stored(id = courseToan6.id) {
  return getCoursesStore().courses.find((course) => course.id === id)!;
}

beforeEach(() => {
  resetCoursesStore();
  resetLibraryStore();
  resetPathsStore();
  // Course handlers go last so their `GET /classes` wins; the library
  // handlers serve the template, versions and lessons, the path handlers
  // the stage and path the course sits in.
  server.use(auditHandler, ...libraryHandlers, ...pathsHandlers, ...coursesHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CourseDetailPage", () => {
  it("renders the header card with code, status and where the course sits", async () => {
    const user = userEvent.setup();
    renderPage();
    expect(await screen.findByRole("heading", { name: "Toán 6 nền tảng" })).toBeInTheDocument();
    expect(screen.getByText("TOAN-6")).toBeInTheDocument();
    expect(screen.getByText("Đang hoạt động")).toBeInTheDocument();
    expect(await screen.findByText("Lộ trình Toán THCS → Nền tảng")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sửa khóa" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Dừng hoạt động" })).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "← Danh sách khóa học" }));
    expect(await screen.findByText("courses-list-stub")).toBeInTheDocument();
  });

  it("shows the eight tabs with Thông tin chung selected", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    const tabs = screen.getAllByRole("tab").map((tab) => tab.textContent);
    expect(tabs).toEqual([
      "Thông tin chung",
      "Chương trình mẫu",
      "Lớp học",
      "Báo cáo",
      "Học viên",
      "Bài tập",
      "Đánh giá",
      "Công việc",
    ]);
    expect(screen.getByRole("tab", { name: "Thông tin chung" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("lists the general facts, the change history and the setup rows on the info tab", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    const info = screen.getByRole("tabpanel");
    expect(within(info).getByText("90 phút")).toBeInTheDocument();
    expect(within(info).getByText("180.000đ")).toBeInTheDocument();
    expect(await within(info).findByText("Nền tảng")).toBeInTheDocument();
    expect(within(info).getByText("Lộ trình Toán THCS")).toBeInTheDocument();

    expect(await within(info).findByText("cập nhật gói học phí")).toBeInTheDocument();
    expect(within(info).getByText("Cô Lan")).toBeInTheDocument();
    expect(within(info).queryByText("cập nhật thông tin khóa")).not.toBeInTheDocument();

    expect(
      within(info).getByRole("button", { name: /Chương trình học.*Toán 6 cơ bản · v1/ }),
    ).toBeInTheDocument();
    expect(
      await within(info).findByRole("button", {
        name: /Điểm thành phần buổi học.*1 mục đang áp dụng/,
      }),
    ).toBeInTheDocument();
    expect(within(info).getByRole("button", { name: /Gói học phí.*2 gói/ })).toBeInTheDocument();
  });

  it("marks unset rows for a course without a template or packs", async () => {
    renderPage(`/courses/${courseVan9.id}`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });
    expect(screen.getByText("Nháp")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: /Chương trình học.*Chưa thiết lập/ }),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Gói học phí.*Chưa thiết lập/ })).toBeInTheDocument();
  });

  it("edits the duration and price in place and says when nothing changed", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Chỉnh sửa" }));
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));
    expect(await screen.findByText("Không có gì thay đổi")).toBeInTheDocument();

    const duration = screen.getByLabelText("Thời lượng buổi (phút)");
    await user.clear(duration);
    await user.type(duration, "120");
    const price = screen.getByLabelText("Giá / buổi (đ)");
    await user.clear(price);
    await user.type(price, "200000");
    await user.click(screen.getByRole("button", { name: "Lưu thay đổi" }));

    expect(await screen.findByText("Đã lưu — áp dụng cho lớp mở mới")).toBeInTheDocument();
    expect(stored()).toMatchObject({
      duration_min: 120,
      default_unit_price: 200000,
      default_template_version_id: courseToan6.default_template_version_id,
    });
  });

  it("hides the history from a member without audit.read", async () => {
    server.use(memberWith("courses.read", "classes.read"));
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(screen.getByText("Chỉ chủ trung tâm xem được lịch sử thay đổi.")).toBeInTheDocument();
  });

  it("shows the not-found block for an unknown course", async () => {
    renderPage("/courses/00000000-0000-4000-8000-000000000404");
    expect(await screen.findByText("Không tìm thấy khóa học")).toBeInTheDocument();
  });

  it("edits the course through the dialog and keeps the default template", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Sửa khóa" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa khóa học" });
    const name = within(dialog).getByLabelText("Tên khóa học");
    await user.clear(name);
    await user.type(name, "Toán 6 nền tảng mới");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByRole("heading", { name: "Toán 6 nền tảng mới" })).toBeInTheDocument();
    expect(stored().default_template_version_id).toBe(courseToan6.default_template_version_id);
    expect(stored().default_unit_price).toBe(180000);
  });

  it("refuses to stop a course with open classes", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Dừng hoạt động" }));
    expect(
      await screen.findByText("Khóa còn 1 lớp đang mở — kết thúc lớp trước khi dừng"),
    ).toBeInTheDocument();
    expect(stored().status).toBe("active");
  });

  it("stops a course without open classes and reactivates it", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    await user.click(screen.getByRole("button", { name: "Dừng hoạt động" }));
    expect(
      await screen.findByText("Đã dừng hoạt động — không mở lớp mới từ khóa này"),
    ).toBeInTheDocument();
    expect(stored(courseVan9.id).status).toBe("archived");

    await user.click(await screen.findByRole("button", { name: "Kích hoạt lại" }));
    expect(await screen.findByText("Đã kích hoạt lại")).toBeInTheDocument();
    expect(stored(courseVan9.id).status).toBe("active");
  });

  it("binds a published template version from setup and can clear it", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });
    expect(screen.getByRole("tab", { name: "Thông tin chung" })).toHaveAttribute(
      "aria-selected",
      "true",
    );

    const templates = screen.getByRole("combobox", { name: "Chương trình mẫu" });
    await within(templates).findByRole("option", { name: "Toán 6 cơ bản" });
    await user.selectOptions(templates, "Toán 6 cơ bản");
    await user.click(await screen.findByRole("button", { name: "Gắn v1" }));

    expect(await screen.findByText("Đã gắn chương trình mẫu")).toBeInTheDocument();
    expect(stored(courseVan9.id).default_template_version_id).toBe(
      "81000000-0000-4000-8000-000000000001",
    );

    await user.click(await screen.findByRole("button", { name: "Bỏ chương trình mẫu" }));
    expect(await screen.findByText("Đã bỏ chương trình mẫu")).toBeInTheDocument();
    expect(stored(courseVan9.id).default_template_version_id).toBeNull();
  });

  it("marks draft versions as unbindable and refuses them", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    const picker = screen.getByRole("combobox", { name: "Phiên bản mặc định cho khóa" });
    const draft = await within(picker).findByRole("option", {
      name: "v2 — Bản nháp (không thể gắn)",
    });
    expect(within(picker).getByRole("option", { name: "v1 — Đã phát hành" })).toBeInTheDocument();

    await user.selectOptions(picker, draft);
    expect(await screen.findByText("v2 còn nháp — kích hoạt trước khi gắn")).toBeInTheDocument();
    expect(stored().default_template_version_id).toBe(courseToan6.default_template_version_id);
  });

  it("reports a default version that was archived since it was chosen", async () => {
    stored().default_template = { ...stored().default_template!, status: "archived" };
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(
      screen.getByText("Phiên bản này đã lưu trữ; chọn phiên bản đã phát hành khác cho lớp mới."),
    ).toBeInTheDocument();
  });

  it("keeps the stored id but says so when the template was deleted", async () => {
    stored().default_template = null;
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(
      screen.getByText("Chương trình mẫu đã gắn không còn trong kho; chọn chương trình khác."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bỏ chương trình mẫu" })).toBeInTheDocument();
  });

  it("explains instead of showing the picker to an editor who cannot read the library", async () => {
    server.use(memberWith("courses.read", "courses.edit", "classes.read"));
    renderPage(`/courses/${courseVan9.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });
    expect(screen.queryByRole("combobox", { name: "Chương trình mẫu" })).not.toBeInTheDocument();
    expect(
      screen.getByText("Cần quyền xem Kho học liệu để chọn chương trình mẫu."),
    ).toBeInTheDocument();
  });

  it("lists the classes and the version each one runs in setup", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(await screen.findByText("Toán 6A")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Đổi phiên bản" }));
    expect(await screen.findByText("class-detail-stub")).toBeInTheDocument();
  });

  it("adds and removes tuition packs, saving them wholesale", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    // 12 × 180.000đ = 2.160.000đ, so the 2.000.000đ pack saves 7%.
    expect(screen.getByText("−7%")).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "Tên gói" }), "Gói 36 buổi");
    await user.type(screen.getByRole("spinbutton", { name: "Số buổi" }), "36");
    await user.type(screen.getByRole("spinbutton", { name: "Giá gói" }), "5400000");
    await user.click(screen.getByRole("button", { name: "+ Thêm gói" }));
    expect(await screen.findByText("Đã thêm gói")).toBeInTheDocument();

    await user.click(await screen.findByRole("button", { name: "Xóa Gói 12 buổi" }));
    expect(await screen.findByText("Đã xóa Gói 12 buổi")).toBeInTheDocument();
    expect(stored().tuition_packs.map((pack) => [pack.name, pack.sessions, pack.price])).toEqual([
      ["Gói 24 buổi", 24, 3800000],
      ["Gói 36 buổi", 36, 5400000],
    ]);
  });

  it("rejects a pack without a name before calling the API", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    await user.click(screen.getByRole("button", { name: "+ Thêm gói" }));
    expect(await screen.findByText("Điền tên gói, số buổi và giá")).toBeInTheDocument();
    expect(stored(courseVan9.id).tuition_packs).toEqual([]);
  });

  it("shows the bound version's lessons on the template tab", async () => {
    renderPage(`/courses/${courseToan6.id}?tab=template`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(await screen.findByText("Buổi 1 Số tự nhiên")).toBeInTheDocument();
    expect(screen.getByText("Buổi 2 Phân số")).toBeInTheDocument();
    expect(screen.getByText("Toán 6 cơ bản · v1 · 2 buổi")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Mở chương trình mẫu →" })).toBeInTheDocument();
  });

  it("points an unbound course's template tab at setup", async () => {
    const user = userEvent.setup();
    const { router } = renderPage(`/courses/${courseVan9.id}?tab=template`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });
    expect(screen.getByText("Khóa chưa gắn chương trình mẫu")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Mở thiết lập" }));
    expect(router.state.location.search).toBe("?tab=setup");
  });

  it("lists the attached classes on the classes tab and opens one", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseToan6.id}?tab=classes`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    const row = await screen.findByRole("row", { name: /Toán 6A/ });
    expect(within(row).getByText("0 HV")).toBeInTheDocument();
    await user.click(within(row).getByText("Toán 6A"));
    expect(await screen.findByText("class-detail-stub")).toBeInTheDocument();
  });

  it("explains the operations tabs that have no screen yet", async () => {
    renderPage(`/courses/${courseToan6.id}?tab=report`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(screen.getByText("Báo cáo — chưa có màn hình chi tiết")).toBeInTheDocument();
  });

  it("hides every editing control from a member who can only read", async () => {
    server.use(memberWith("courses.read", "classes.read"));
    renderPage(`/courses/${courseToan6.id}?tab=setup`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(screen.queryByRole("button", { name: "Sửa khóa" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Dừng hoạt động" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "+ Thêm gói" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xóa / })).not.toBeInTheDocument();
    expect(screen.getByText("Gói 12 buổi")).toBeInTheDocument();
  });
});
