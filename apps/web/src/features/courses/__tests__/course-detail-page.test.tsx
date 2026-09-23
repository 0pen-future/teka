import { screen, waitFor, within } from "@testing-library/react";
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
  classToan6A,
  coursesHandlers,
  courseToan6,
  courseVan9,
  getCoursesStore,
  resetCoursesStore,
} from "./courses-handlers";

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

async function pick(user: ReturnType<typeof userEvent.setup>, combobox: string, option: RegExp) {
  await user.click(screen.getByRole("combobox", { name: combobox }));
  await user.click(
    within(await screen.findByRole("listbox")).getByRole("option", { name: option }),
  );
}

beforeEach(() => {
  resetCoursesStore();
  resetLibraryStore();
  // Course handlers go last so their `GET /classes` wins over nothing; the
  // library handlers serve the template and version pickers.
  server.use(...libraryHandlers, ...coursesHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("CourseDetailPage", () => {
  it("renders the header and the info tab", async () => {
    renderPage();
    expect(await screen.findByRole("heading", { name: "Toán 6 nền tảng" })).toBeInTheDocument();
    expect(screen.getByText("Mã: TOAN-6")).toBeInTheDocument();
    expect(screen.getByText("Đang mở")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Danh mục khóa học" })).toHaveAttribute(
      "href",
      "/courses",
    );

    const info = screen.getByRole("tabpanel");
    expect(within(info).getByText("Toán")).toBeInTheDocument();
    expect(within(info).getByText("Lớp 6")).toBeInTheDocument();
    expect(within(info).getByText("180.000 ₫")).toBeInTheDocument();
    expect(within(info).getByText("36 buổi")).toBeInTheDocument();
    expect(within(info).getByText("90 phút")).toBeInTheDocument();
    expect(within(info).getByText("Khóa nền tảng cho học sinh mới vào lớp 6.")).toBeInTheDocument();
  });

  it("shows the not-found block for an unknown course", async () => {
    renderPage("/courses/00000000-0000-4000-8000-000000000404");
    expect(await screen.findByText("Không tìm thấy khóa học")).toBeInTheDocument();
  });

  it("edits the course through the dialog and keeps the default template", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Sửa" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa khóa học" });
    const name = within(dialog).getByLabelText("Tên khóa học");
    await user.clear(name);
    await user.type(name, "Toán 6 nền tảng mới");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByRole("heading", { name: "Toán 6 nền tảng mới" })).toBeInTheDocument();
    const stored = getCoursesStore().courses.find((course) => course.id === courseToan6.id);
    expect(stored?.default_template_version_id).toBe(courseToan6.default_template_version_id);
    expect(stored?.default_unit_price).toBe(180000);
  });

  it("swaps the default template for a published version and can clear it", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}?tab=template`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });
    expect(screen.getByText("Chưa gắn chương trình mẫu.")).toBeInTheDocument();

    await pick(user, "Chương trình mẫu", /Toán 6 cơ bản/);
    await pick(user, "Phiên bản", /v1 · Đã phát hành/);
    await user.click(screen.getByRole("button", { name: "Lưu chương trình mẫu" }));

    expect(await screen.findByText("Toán 6 cơ bản · v1 · Đã phát hành")).toBeInTheDocument();
    expect(
      getCoursesStore().courses.find((course) => course.id === courseVan9.id)
        ?.default_template_version_id,
    ).toBe("81000000-0000-4000-8000-000000000001");

    await user.click(screen.getByRole("button", { name: "Bỏ chương trình mẫu" }));
    expect(await screen.findByText("Chưa gắn chương trình mẫu.")).toBeInTheDocument();
    expect(
      getCoursesStore().courses.find((course) => course.id === courseVan9.id)
        ?.default_template_version_id,
    ).toBeNull();
  });

  it("reports a default version that was archived since it was chosen", async () => {
    const stored = getCoursesStore().courses.find((course) => course.id === courseToan6.id)!;
    stored.default_template = { ...stored.default_template!, status: "archived" };
    renderPage(`/courses/${courseToan6.id}?tab=template`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(screen.getByText("Toán 6 cơ bản · v1 · Đã lưu trữ")).toBeInTheDocument();
    expect(
      screen.getByText("Phiên bản này đã lưu trữ; chọn phiên bản đã phát hành khác cho lớp mới."),
    ).toBeInTheDocument();
  });

  it("keeps the stored id but says so when the template was deleted", async () => {
    const stored = getCoursesStore().courses.find((course) => course.id === courseToan6.id)!;
    stored.default_template = null;
    renderPage(`/courses/${courseToan6.id}?tab=template`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(
      screen.getByText("Chương trình mẫu đã gắn không còn trong kho; chọn chương trình khác."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bỏ chương trình mẫu" })).toBeInTheDocument();
  });

  it("explains instead of showing the picker to an editor who cannot read the library", async () => {
    server.use(memberWith("courses.read", "courses.edit", "classes.read"));
    renderPage(`/courses/${courseToan6.id}?tab=template`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(screen.queryByRole("combobox", { name: "Chương trình mẫu" })).not.toBeInTheDocument();
    expect(
      screen.getByText("Cần quyền xem Kho học liệu để chọn chương trình mẫu."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Bỏ chương trình mẫu" })).toBeInTheDocument();
  });

  it("labels the class counters as center-wide on the info tab", async () => {
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });
    expect(screen.getByText("Lớp đang học (toàn trung tâm)")).toBeInTheDocument();
    expect(screen.getByText("Lớp sắp mở (toàn trung tâm)")).toBeInTheDocument();
  });

  it("offers only published versions in the version picker", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}?tab=template`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    await pick(user, "Chương trình mẫu", /Văn 9 luyện thi/);
    expect(
      await screen.findByText("Chương trình này chưa có phiên bản đã phát hành."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("combobox", { name: "Phiên bản" })).not.toBeInTheDocument();
  });

  it("edits, reorders and saves the tuition packs", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseToan6.id}?tab=settings`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    const editor = screen.getByRole("region", { name: "Gói học phí" });
    expect(within(editor).getByRole("textbox", { name: "Tên gói 1" })).toHaveValue("Gói 12 buổi");
    await user.click(within(editor).getByRole("button", { name: "Thêm gói" }));
    await user.type(within(editor).getByRole("textbox", { name: "Tên gói 3" }), "Gói 36 buổi");
    await user.type(within(editor).getByRole("spinbutton", { name: "Số buổi 3" }), "36");
    await user.type(within(editor).getByRole("spinbutton", { name: "Giá 3" }), "5400000");
    await user.click(within(editor).getByRole("button", { name: "Chuyển lên gói 3" }));
    await user.click(within(editor).getByRole("button", { name: "Xoá gói 1" }));
    await user.click(within(editor).getByRole("button", { name: "Lưu gói học phí" }));

    expect(await screen.findByText("Đã lưu gói học phí")).toBeInTheDocument();
    const stored = getCoursesStore().courses.find((course) => course.id === courseToan6.id);
    expect(stored?.tuition_packs.map((pack) => [pack.name, pack.sessions, pack.price])).toEqual([
      ["Gói 36 buổi", 36, 5400000],
      ["Gói 24 buổi", 24, 3800000],
    ]);
  });

  it("rejects a pack without a name before calling the API", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}?tab=settings`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    const editor = screen.getByRole("region", { name: "Gói học phí" });
    await user.click(within(editor).getByRole("button", { name: "Thêm gói" }));
    await user.click(within(editor).getByRole("button", { name: "Lưu gói học phí" }));

    expect(await within(editor).findByText("Bắt buộc nhập tên gói")).toBeInTheDocument();
    expect(
      getCoursesStore().courses.find((course) => course.id === courseVan9.id)?.tuition_packs,
    ).toEqual([]);
  });

  it("lists the attached classes on the operations tab", async () => {
    renderPage(`/courses/${courseToan6.id}?tab=classes`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    const row = await screen.findByRole("row", { name: /Toán 6A/ });
    expect(within(row).getByRole("link", { name: "Toán 6A" })).toHaveAttribute(
      "href",
      `/classes/${classToan6A.id}`,
    );
    expect(within(row).getByText("TOAN6A")).toBeInTheDocument();
    expect(within(row).getByText("Đang học")).toBeInTheDocument();
  });

  it("archives the course after confirmation", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Ngừng tuyển" }));
    const confirm = await screen.findByRole("dialog", {
      name: /Ngừng tuyển khóa "Toán 6 nền tảng"/,
    });
    await user.click(within(confirm).getByRole("button", { name: "Ngừng tuyển" }));

    await waitFor(() =>
      expect(screen.queryByRole("button", { name: "Ngừng tuyển" })).not.toBeInTheDocument(),
    );
    expect(screen.getByText("Ngừng tuyển")).toBeInTheDocument();
    expect(getCoursesStore().courses.find((course) => course.id === courseToan6.id)?.status).toBe(
      "archived",
    );
  });

  it("refuses to delete a course that still has classes and says so", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    await user.click(screen.getByRole("button", { name: "Xoá khóa học" }));
    const confirm = await screen.findByRole("dialog", { name: /Xoá khóa "Toán 6 nền tảng"/ });
    await user.click(within(confirm).getByRole("button", { name: "Xoá khóa học" }));

    expect(
      await screen.findByText("Khóa học đang có lớp gắn vào, hãy lưu trữ thay vì xoá"),
    ).toBeInTheDocument();
    expect(getCoursesStore().courses.some((course) => course.id === courseToan6.id)).toBe(true);
  });

  it("deletes an unused course and returns to the list", async () => {
    const user = userEvent.setup();
    renderPage(`/courses/${courseVan9.id}`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    await user.click(screen.getByRole("button", { name: "Xoá khóa học" }));
    const confirm = await screen.findByRole("dialog", { name: /Xoá khóa "Văn 9 luyện thi"/ });
    await user.click(within(confirm).getByRole("button", { name: "Xoá khóa học" }));

    expect(await screen.findByText("courses-list-stub")).toBeInTheDocument();
    expect(getCoursesStore().courses.some((course) => course.id === courseVan9.id)).toBe(false);
  });

  it("hides every editing control from a member who can only read", async () => {
    server.use(memberWith("courses.read", "classes.read"));
    renderPage(`/courses/${courseToan6.id}?tab=settings`);
    await screen.findByRole("heading", { name: "Toán 6 nền tảng" });

    expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Ngừng tuyển" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Xoá khóa học" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu gói học phí" })).not.toBeInTheDocument();
    expect(screen.getByText("Gói 12 buổi")).toBeInTheDocument();
  });
});
