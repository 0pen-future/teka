import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import {
  renderWithProviders,
  signInAs,
  testPrimaryTeacher,
  testSecondaryTeacher,
} from "@/test/utils";

import { LibraryPage } from "../pages/library-page";
import {
  exerciseBai1,
  exerciseBai2,
  getLibraryStore,
  libraryHandlers,
  materialVideo,
  resetLibraryStore,
  templateToan6,
} from "./library-handlers";

function renderPage(route = "/library") {
  return renderWithProviders(<LibraryPage tab="templates" />, {
    route,
    path: "/library",
    extraRoutes: [
      { path: "/library/materials", element: <LibraryPage tab="materials" /> },
      { path: "/library/exercises", element: <LibraryPage tab="exercises" /> },
      { path: "/library/templates/:id", element: <div>template-detail-stub</div> },
    ],
  });
}

/** A member holding only the read key: browsing, never authoring. */
function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function rowOf(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("LibraryPage routing", () => {
  it("redirects the legacy ?tab=materials query to /library/materials", async () => {
    renderPage("/library?tab=materials");
    expect(await screen.findByRole("row", { name: /Slide số tự nhiên/ })).toBeInTheDocument();
  });

  it("redirects the legacy ?tab=exercises query to /library/exercises", async () => {
    renderPage("/library?tab=exercises");
    expect(await screen.findByRole("row", { name: /Bài 1: Tập hợp/ })).toBeInTheDocument();
  });

  it("drops an unknown or removed ?tab= value back to the templates route", async () => {
    renderPage("/library?tab=lessons");
    expect(await screen.findByRole("article", { name: "Toán 6 cơ bản" })).toBeInTheDocument();
  });

  it("keeps other query params (like a search term) through the legacy ?tab= redirect", async () => {
    renderPage("/library?tab=exercises&q=BT-0002");

    expect(await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Tìm bài tập" })).toHaveValue("BT-0002");
    expect(screen.queryByRole("row", { name: /Bài 1: Tập hợp/ })).not.toBeInTheDocument();
  });

  it("renders each bank directly when opened at its own route", async () => {
    renderPage("/library/materials");
    expect(await screen.findByRole("row", { name: /Slide số tự nhiên/ })).toBeInTheDocument();
    expect(screen.queryByRole("article")).not.toBeInTheDocument();
  });
});

describe("LibraryPage tabs", () => {
  it("offers all three tabs and marks the current one selected", async () => {
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    expect(screen.getByRole("tab", { name: "Chương trình mẫu" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    for (const name of ["Ngân hàng nội dung", "Ngân hàng bài tập"]) {
      expect(screen.getByRole("tab", { name })).toBeEnabled();
    }
  });

  it("navigates to the materials route when its tab is clicked", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("tab", { name: "Ngân hàng nội dung" }));

    expect(await screen.findByRole("row", { name: /Slide số tự nhiên/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Ngân hàng nội dung" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("navigates to the exercises route when its tab is clicked", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("tab", { name: "Ngân hàng bài tập" }));

    expect(await screen.findByRole("row", { name: /Bài 1: Tập hợp/ })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Ngân hàng bài tập" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });
});

describe("LibraryPage templates tab", () => {
  it("lists every template as a card with subject, counts and version chips, linking to the detail page", async () => {
    renderPage();

    expect(await screen.findByRole("heading", { name: "Kho học liệu" })).toBeInTheDocument();
    const toan = await screen.findByRole("article", { name: "Toán 6 cơ bản" });
    expect(within(toan).getByText("Toán · Lớp 6")).toBeInTheDocument();
    expect(within(toan).getByText("2 buổi mẫu · 2 phiên bản · 0 lớp đang gắn")).toBeInTheDocument();
    expect(within(toan).getByRole("button", { name: "v1" })).toBeInTheDocument();
    expect(within(toan).getByRole("button", { name: "v2" })).toBeInTheDocument();
    expect(within(toan).getByRole("link", { name: "Toán 6 cơ bản" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}`,
    );

    const van = await screen.findByRole("article", { name: "Văn 9 luyện thi" });
    expect(within(van).getByText("Ngữ văn")).toBeInTheDocument();
    expect(within(van).getByText("0 buổi mẫu · 1 phiên bản · 0 lớp đang gắn")).toBeInTheDocument();
  });

  it("navigates to the detail page with the version query when a chip is clicked", async () => {
    const user = userEvent.setup();
    renderPage();
    const toan = await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.click(within(toan).getByRole("button", { name: "v1" }));

    expect(await screen.findByText("template-detail-stub")).toBeInTheDocument();
  });

  it("filters by name or code through the API's q parameter", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.type(screen.getByRole("searchbox", { name: "Tìm chương trình mẫu" }), "văn");

    await waitFor(() => {
      expect(screen.queryByRole("article", { name: "Toán 6 cơ bản" })).not.toBeInTheDocument();
    });
    expect(await screen.findByRole("article", { name: "Văn 9 luyện thi" })).toBeInTheDocument();
  });

  it("shows the empty state when the center has no template yet", async () => {
    getLibraryStore().templates = [];
    renderPage();

    expect(await screen.findByText("Chưa có chương trình mẫu nào.")).toBeInTheDocument();
  });

  it("creates a template from the dialog and opens its detail page", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("button", { name: "+ Chương trình mẫu" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo chương trình mẫu" });
    await user.type(within(dialog).getByLabelText("Mã chương trình"), "ly7");
    await user.type(within(dialog).getByLabelText("Tên chương trình"), "Lý 7 nâng cao");
    await user.type(within(dialog).getByLabelText("Môn học"), "Vật lý");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await screen.findByText("template-detail-stub")).toBeInTheDocument();
    const created = getLibraryStore().templates.find((t) => t.code === "LY7");
    expect(created).toMatchObject({ name: "Lý 7 nâng cao", subject: "Vật lý", level: null });
  });

  it("puts a code clash on the code field and keeps the dialog open", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("article", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("button", { name: "+ Chương trình mẫu" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo chương trình mẫu" });
    await user.type(within(dialog).getByLabelText("Mã chương trình"), "toan6");
    await user.type(within(dialog).getByLabelText("Tên chương trình"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await within(dialog).findByText("mã chương trình đã tồn tại")).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Tạo chương trình mẫu" })).toBeInTheDocument();
  });

  it("hides the create action from a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage();

    await screen.findByRole("article", { name: "Toán 6 cơ bản" });
    expect(screen.queryByRole("button", { name: "+ Chương trình mẫu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Sửa" })).not.toBeInTheDocument();
  });

  it("offers the create action to a member holding library.edit", async () => {
    server.use(memberWith("library.read", "library.edit"));
    signInAs(testSecondaryTeacher);
    renderPage();

    await screen.findByRole("article", { name: "Toán 6 cơ bản" });
    expect(screen.getByRole("button", { name: "+ Chương trình mẫu" })).toBeInTheDocument();
  });
});

describe("MaterialsBank", () => {
  it("lists materials with kind, format, usage and status", async () => {
    renderPage("/library/materials");

    const slide = await screen.findByRole("row", { name: /Slide số tự nhiên/ });
    expect(within(slide).getByText("Tài liệu")).toBeInTheDocument();
    expect(within(slide).getByText("Slide bài giảng chương 1.")).toBeInTheDocument();
    expect(within(slide).getByText("PDF")).toBeInTheDocument();
    expect(within(slide).getByText("2 buổi · 1 CT mẫu")).toBeInTheDocument();
    expect(within(slide).getByText("Hoạt động")).toBeInTheDocument();

    const video = await screen.findByRole("row", { name: /Video phân số/ });
    expect(within(video).getByText("Video")).toBeInTheDocument();
    expect(within(video).getByText("Link")).toBeInTheDocument();
    expect(within(video).getByText("—")).toBeInTheDocument();
  });

  it("filters by title through the API's q parameter", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm học liệu" }), "video");

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Slide số tự nhiên/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Video phân số/ })).toBeInTheDocument();
  });

  it("toggles a material's status and the filter honors it", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Ngừng Video phân số" }));
    expect(await screen.findByText("Đã ngừng học liệu")).toBeInTheDocument();
    expect(within(rowOf("Video phân số")).getByText("Ngừng")).toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Hoạt động" }));
    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Video phân số/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Slide số tự nhiên/ })).toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Ngừng" }));
    expect(await screen.findByRole("row", { name: /Video phân số/ })).toBeInTheDocument();
    expect(screen.queryByRole("row", { name: /Slide số tự nhiên/ })).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Kích hoạt Video phân số" }));
    expect(await screen.findByText("Đã kích hoạt học liệu")).toBeInTheDocument();
  });

  it("creates a material with a required URL, a kind excluding Khác, and comma-separated tags", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    await user.click(screen.getByRole("button", { name: "Thêm học liệu" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
    expect(
      within(dialog).queryByText("Sửa nội dung này sẽ ảnh hưởng mọi buổi đang dùng"),
    ).not.toBeInTheDocument();
    await user.click(within(dialog).getByRole("combobox", { name: /Loại/ }));
    expect(
      within(await screen.findByRole("listbox")).queryByRole("option", { name: "Khác" }),
    ).not.toBeInTheDocument();
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: "Liên kết ngoài" }),
    );
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Đề cương ôn tập");
    await user.type(within(dialog).getByLabelText("Đường dẫn"), "https://example.com/de-cuong");
    await user.type(within(dialog).getByLabelText("Thẻ"), "ôn tập, , cuối kỳ ");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await screen.findByText("Đã thêm học liệu")).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /Đề cương ôn tập/ })).toBeInTheDocument();
    expect(getLibraryStore().materials.find((m) => m.title === "Đề cương ôn tập")).toMatchObject({
      kind: "link",
      url: "https://example.com/de-cuong",
      description: null,
      tags: ["ôn tập", "cuối kỳ"],
    });
  });

  it("refuses submitting a material without a URL", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Thêm học liệu" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Trang trống");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await within(dialog).findByText("Bắt buộc nhập đường dẫn")).toBeInTheDocument();
    expect(getLibraryStore().materials.some((m) => m.title === "Trang trống")).toBe(false);
  });

  it("refuses a material link that is not http(s)", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Thêm học liệu" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Trang bẩn");
    await user.type(within(dialog).getByLabelText("Đường dẫn"), "javascript:alert(1)");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(
      await within(dialog).findByText("Đường dẫn phải bắt đầu bằng http:// hoặc https://"),
    ).toBeInTheDocument();
    expect(getLibraryStore().materials.some((m) => m.title === "Trang bẩn")).toBe(false);
  });

  it("edits a material in place", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Sửa Video phân số" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa học liệu" });
    expect(
      within(dialog).getByText("Sửa nội dung này sẽ ảnh hưởng mọi buổi đang dùng"),
    ).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Đường dẫn")).toHaveValue(materialVideo.url);
    await user.clear(within(dialog).getByLabelText("Tên học liệu"));
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Video so sánh phân số");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu học liệu")).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /Video so sánh phân số/ })).toBeInTheDocument();
  });

  it("keeps Khác selectable only while editing a material that already has it", async () => {
    const user = userEvent.setup();
    getLibraryStore().materials.push({
      ...materialVideo,
      id: "83000000-0000-4000-8000-000000000009",
      title: "Ghi chú cũ",
      kind: "other",
    });
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Ghi chú cũ/ });

    await user.click(screen.getByRole("button", { name: "Sửa Ghi chú cũ" }));
    const editDialog = await screen.findByRole("dialog", { name: "Sửa học liệu" });
    expect(within(editDialog).getByRole("combobox", { name: /Loại/ })).toHaveTextContent("Khác");
    await user.click(within(editDialog).getByRole("combobox", { name: /Loại/ }));
    expect(
      within(await screen.findByRole("listbox")).getByRole("option", { name: "Khác" }),
    ).toBeInTheDocument();
    await user.keyboard("{Escape}");
    await user.click(within(editDialog).getByRole("button", { name: "Hủy" }));

    await user.click(screen.getByRole("button", { name: "Thêm học liệu" }));
    const createDialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
    await user.click(within(createDialog).getByRole("combobox", { name: /Loại/ }));
    expect(
      within(await screen.findByRole("listbox")).queryByRole("option", { name: "Khác" }),
    ).not.toBeInTheDocument();
  });

  it("deletes a free material after confirming", async () => {
    const user = userEvent.setup();
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Xoá Video phân số" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá học liệu" }),
    );

    expect(await screen.findByText("Đã xoá học liệu")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Video phân số/ })).not.toBeInTheDocument();
    });
    expect(getLibraryStore().materials.some((m) => m.id === materialVideo.id)).toBe(false);
  });

  it("disables Xoá for a material a lesson still uses, with a tooltip explaining why", async () => {
    renderPage("/library/materials");
    const slide = await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    const deleteButton = within(slide).getByRole("button", { name: "Xoá Slide số tự nhiên" });
    expect(deleteButton).toBeDisabled();
    expect(deleteButton).toHaveAttribute(
      "title",
      "Học liệu đang gắn vào buổi học mẫu, ngừng hoạt động thay vì xoá.",
    );
  });

  it("shows the do-not-hard-delete footnote", async () => {
    renderPage("/library/materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    expect(
      screen.getByText(/Không xóa cứng nội dung còn được phiên bản đang hoạt động tham chiếu/),
    ).toBeInTheDocument();
  });

  it("is read-only for a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage("/library/materials");

    await screen.findByRole("row", { name: /Slide số tự nhiên/ });
    expect(screen.queryByRole("button", { name: "Thêm học liệu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Sửa / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xoá / })).not.toBeInTheDocument();
  });
});

describe("ExercisesBank", () => {
  it("lists exercises with code, tags, skill/level and usage", async () => {
    renderPage("/library/exercises");

    const bai1 = await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });
    expect(within(bai1).getByText("BT-0001")).toBeInTheDocument();
    expect(within(bai1).getByText("chương 1")).toBeInTheDocument();
    expect(within(bai1).getByText("1 buổi · 1 CT mẫu")).toBeInTheDocument();

    const bai2 = await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ });
    expect(within(bai2).getAllByText("—").length).toBeGreaterThanOrEqual(2);
  });

  it("filters by title or code through the API's q parameter", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm bài tập" }), "BT-0002");

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Bài 1: Tập hợp/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Bài 2: So sánh phân số/ })).toBeInTheDocument();
  });

  it("seeds the search box from an initial ?q= so the lesson page's link lands filtered", async () => {
    renderPage("/library/exercises?q=BT-0002");

    expect(await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ })).toBeInTheDocument();
    expect(screen.getByRole("searchbox", { name: "Tìm bài tập" })).toHaveValue("BT-0002");
    expect(screen.queryByRole("row", { name: /Bài 1: Tập hợp/ })).not.toBeInTheDocument();
  });

  it("copies the exercise code to the clipboard and confirms with a toast", async () => {
    const user = userEvent.setup();
    const writeText = vi.fn().mockResolvedValue(undefined);
    Object.defineProperty(navigator, "clipboard", { value: { writeText }, configurable: true });
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.click(screen.getByRole("button", { name: "Sao chép mã BT-0001" }));

    expect(writeText).toHaveBeenCalledWith("BT-0001");
    expect(await screen.findByText("Đã sao chép mã")).toBeInTheDocument();
  });

  it("toggles an exercise's status and the filter honors it", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ });

    await user.click(screen.getByRole("button", { name: "Ngừng Bài 2: So sánh phân số" }));
    expect(await screen.findByText("Đã ngừng bài tập")).toBeInTheDocument();

    await user.click(screen.getByRole("radio", { name: "Hoạt động" }));
    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Bài 2: So sánh phân số/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Bài 1: Tập hợp/ })).toBeInTheDocument();
  });

  it("creates an exercise with a code, skill and level from the dialog", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.click(screen.getByRole("button", { name: "Thêm bài tập" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm bài tập" });
    await user.type(within(dialog).getByLabelText("Tên bài tập"), "Bài 3: Hỗn số");
    await user.type(within(dialog).getByLabelText("Mã"), "bt-hon-so");
    await user.type(within(dialog).getByLabelText("Kỹ năng"), "Tính toán");
    await user.type(within(dialog).getByLabelText("Cấp độ"), "Nâng cao");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await screen.findByText("Đã thêm bài tập")).toBeInTheDocument();
    const row = await screen.findByRole("row", { name: /Bài 3: Hỗn số/ });
    expect(within(row).getByText("Tính toán")).toBeInTheDocument();
    expect(within(row).getByText("Nâng cao")).toBeInTheDocument();
    expect(getLibraryStore().exercises.find((e) => e.title === "Bài 3: Hỗn số")).toMatchObject({
      code: "BT-HON-SO",
      skill: "Tính toán",
      level: "Nâng cao",
    });
  });

  it("rejects a malformed exercise code before calling the API", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.click(screen.getByRole("button", { name: "Thêm bài tập" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm bài tập" });
    await user.type(within(dialog).getByLabelText("Tên bài tập"), "Bài lỗi mã");
    await user.type(within(dialog).getByLabelText("Mã"), "bt lỗi");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(
      await within(dialog).findByText("Chỉ dùng chữ, số và dấu gạch ngang"),
    ).toBeInTheDocument();
    expect(getLibraryStore().exercises.some((e) => e.title === "Bài lỗi mã")).toBe(false);
  });

  it("rejects an exercise skill longer than the API's 50-character limit", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.click(screen.getByRole("button", { name: "Thêm bài tập" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm bài tập" });
    await user.type(within(dialog).getByLabelText("Tên bài tập"), "Bài lỗi kỹ năng");
    await user.type(within(dialog).getByLabelText("Kỹ năng"), "a".repeat(51));
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await within(dialog).findByText("Tối đa 50 ký tự")).toBeInTheDocument();
    expect(getLibraryStore().exercises.some((e) => e.title === "Bài lỗi kỹ năng")).toBe(false);
  });

  it("deletes a free exercise", async () => {
    const user = userEvent.setup();
    renderPage("/library/exercises");
    await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ });

    await user.click(screen.getByRole("button", { name: "Xoá Bài 2: So sánh phân số" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá bài tập" }),
    );
    expect(await screen.findByText("Đã xoá bài tập")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLibraryStore().exercises.some((e) => e.id === exerciseBai2.id)).toBe(false);
    });
  });

  it("disables Xoá for an exercise a lesson still uses, with a tooltip explaining why", async () => {
    renderPage("/library/exercises");
    const bai1 = await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    const deleteButton = within(bai1).getByRole("button", { name: "Xoá Bài 1: Tập hợp" });
    expect(deleteButton).toBeDisabled();
    expect(deleteButton).toHaveAttribute(
      "title",
      "Bài tập đang gắn vào buổi học mẫu, ngừng hoạt động thay vì xoá.",
    );
    expect(getLibraryStore().exercises.some((e) => e.id === exerciseBai1.id)).toBe(true);
  });

  it("is read-only for a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage("/library/exercises");

    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });
    expect(screen.queryByRole("button", { name: "Thêm bài tập" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Sửa / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xoá / })).not.toBeInTheDocument();
  });
});
