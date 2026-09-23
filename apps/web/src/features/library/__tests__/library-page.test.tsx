import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
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
  materialSlide,
  materialVideo,
  resetLibraryStore,
  templateToan6,
  versionVan9Draft,
} from "./library-handlers";

function renderPage(route = "/library") {
  return renderWithProviders(<LibraryPage />, {
    route,
    path: "/library",
    extraRoutes: [{ path: "/library/templates/:id", element: <div>template-detail-stub</div> }],
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

describe("LibraryPage templates tab", () => {
  it("lists every template with code, subject and version summary, linking to the detail page", async () => {
    renderPage();

    expect(await screen.findByRole("heading", { name: "Kho học liệu" })).toBeInTheDocument();
    const toan = await screen.findByRole("row", { name: /Toán 6 cơ bản/ });
    expect(within(toan).getByText("TOAN6")).toBeInTheDocument();
    expect(within(toan).getByText("Toán · Lớp 6")).toBeInTheDocument();
    expect(within(toan).getByText("v1 · Đã phát hành")).toBeInTheDocument();
    expect(within(toan).getByText("v2 · Bản nháp")).toBeInTheDocument();
    expect(within(toan).getByRole("link", { name: "Toán 6 cơ bản" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}`,
    );

    const van = rowOf("Văn 9 luyện thi");
    expect(within(van).getByText("Chưa phát hành")).toBeInTheDocument();
    expect(within(van).getByText("v1 · Bản nháp")).toBeInTheDocument();
  });

  it("offers all four tabs and marks the templates tab selected", async () => {
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    expect(screen.getByRole("tab", { name: "Chương trình mẫu" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    for (const name of ["Buổi học mẫu", "Học liệu", "Bài tập"]) {
      expect(screen.getByRole("tab", { name })).toBeEnabled();
    }
  });

  it("filters by name through the API's q parameter", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm chương trình mẫu" }), "văn");

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Toán 6 cơ bản/ })).not.toBeInTheDocument();
    });
    expect(await screen.findByRole("row", { name: /Văn 9 luyện thi/ })).toBeInTheDocument();
  });

  it("shows the empty state when the center has no template yet", async () => {
    getLibraryStore().templates = [];
    renderPage();

    expect(await screen.findByText("Chưa có chương trình mẫu nào.")).toBeInTheDocument();
  });

  it("creates a template from the dialog and opens its detail page", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    await user.click(screen.getByRole("button", { name: "Tạo chương trình mẫu" }));
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
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    await user.click(screen.getByRole("button", { name: "Tạo chương trình mẫu" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo chương trình mẫu" });
    await user.type(within(dialog).getByLabelText("Mã chương trình"), "toan6");
    await user.type(within(dialog).getByLabelText("Tên chương trình"), "Trùng mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(await within(dialog).findByText("mã chương trình đã tồn tại")).toBeInTheDocument();
    expect(screen.getByRole("dialog", { name: "Tạo chương trình mẫu" })).toBeInTheDocument();
  });

  it("rejects a malformed code before calling the API", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    await user.click(screen.getByRole("button", { name: "Tạo chương trình mẫu" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo chương trình mẫu" });
    await user.type(within(dialog).getByLabelText("Mã chương trình"), "a b");
    await user.type(within(dialog).getByLabelText("Tên chương trình"), "Sai mã");
    await user.click(within(dialog).getByRole("button", { name: "Tạo" }));

    expect(
      await within(dialog).findByText("Chỉ dùng chữ, số và dấu gạch ngang"),
    ).toBeInTheDocument();
    expect(getLibraryStore().templates).toHaveLength(2);
  });

  it("hides the create action from a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage();

    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });
    expect(screen.queryByRole("button", { name: "Tạo chương trình mẫu" })).not.toBeInTheDocument();
  });

  it("offers the create action to a member holding library.edit", async () => {
    server.use(memberWith("library.read", "library.edit"));
    signInAs(testSecondaryTeacher);
    renderPage();

    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });
    expect(screen.getByRole("button", { name: "Tạo chương trình mẫu" })).toBeInTheDocument();
  });
});

describe("LibraryPage lessons tab", () => {
  it("browses a template's current version lessons after picking it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    await user.click(screen.getByRole("tab", { name: "Buổi học mẫu" }));
    expect(screen.getByText("Chọn một chương trình mẫu để xem các buổi học.")).toBeInTheDocument();

    await user.click(screen.getByRole("combobox", { name: "Chọn chương trình mẫu" }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: /Toán 6 cơ bản/ }),
    );

    // The open draft is the working copy, so that is the version shown.
    expect(await screen.findByText("v2 · Bản nháp")).toBeInTheDocument();
    const first = await screen.findByRole("row", { name: /Số tự nhiên/ });
    expect(within(first).getByRole("link", { name: "Số tự nhiên" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}/lessons/82000000-0000-4000-8000-000000000003`,
    );
    expect(within(first).getByText("90 phút")).toBeInTheDocument();
    expect(within(rowOf("Phân số")).getByText("—")).toBeInTheDocument();
  });

  it("reports an empty draft without a table", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=lessons");
    await user.click(await screen.findByRole("combobox", { name: "Chọn chương trình mẫu" }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: /Văn 9 luyện thi/ }),
    );

    expect(await screen.findByText("Phiên bản này chưa có buổi học nào.")).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(getLibraryStore().lessons.some((l) => l.version_id === versionVan9Draft.id)).toBe(false);
  });
});

describe("LibraryPage materials tab", () => {
  it("lists the materials with kind, link and tags", async () => {
    renderPage("/library?tab=materials");

    const slide = await screen.findByRole("row", { name: /Slide số tự nhiên/ });
    expect(within(slide).getByText("Tài liệu")).toBeInTheDocument();
    expect(within(slide).getByRole("link", { name: /slide-so-tu-nhien/ })).toHaveAttribute(
      "href",
      materialSlide.url,
    );
    expect(within(slide).getByText("chương 1, slide")).toBeInTheDocument();
    expect(within(rowOf("Video phân số")).getByText("Video")).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Học liệu" })).toHaveAttribute("aria-selected", "true");
  });

  it("filters by title through the API's q parameter", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm học liệu" }), "video");

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Slide số tự nhiên/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Video phân số/ })).toBeInTheDocument();
  });

  it("creates a material from the dialog with a kind and comma-separated tags", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    await user.click(screen.getByRole("button", { name: "Thêm học liệu" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm học liệu" });
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Đề cương ôn tập");
    await user.click(within(dialog).getByRole("combobox", { name: /Loại/ }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: "Liên kết" }),
    );
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

  it("refuses a material link that is not http(s) before calling the API", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=materials");
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
    renderPage("/library?tab=materials");
    await screen.findByRole("row", { name: /Video phân số/ });

    await user.click(screen.getByRole("button", { name: "Sửa Video phân số" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa học liệu" });
    expect(within(dialog).getByLabelText("Đường dẫn")).toHaveValue(materialVideo.url);
    await user.clear(within(dialog).getByLabelText("Tên học liệu"));
    await user.type(within(dialog).getByLabelText("Tên học liệu"), "Video so sánh phân số");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu học liệu")).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /Video so sánh phân số/ })).toBeInTheDocument();
  });

  it("deletes a free material after confirming", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=materials");
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

  it("keeps a material that a lesson still uses and shows the conflict", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=materials");
    await screen.findByRole("row", { name: /Slide số tự nhiên/ });

    await user.click(screen.getByRole("button", { name: "Xoá Slide số tự nhiên" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá học liệu" }),
    );

    expect(await screen.findByText("học liệu đang được gắn vào buổi học")).toBeInTheDocument();
    expect(screen.getByRole("row", { name: /Slide số tự nhiên/ })).toBeInTheDocument();
    expect(getLibraryStore().materials.some((m) => m.id === materialSlide.id)).toBe(true);
  });

  it("is read-only for a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage("/library?tab=materials");

    await screen.findByRole("row", { name: /Slide số tự nhiên/ });
    expect(screen.queryByRole("button", { name: "Thêm học liệu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Sửa / })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /^Xoá / })).not.toBeInTheDocument();
  });
});

describe("LibraryPage exercises tab", () => {
  it("lists the exercises with difficulty and tags", async () => {
    renderPage("/library?tab=exercises");

    const bai1 = await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });
    expect(within(bai1).getByText("Mức 2")).toBeInTheDocument();
    expect(within(bai1).getByText("chương 1")).toBeInTheDocument();
    expect(within(rowOf("Bài 2: So sánh phân số")).getAllByText("—").length).toBeGreaterThan(0);
    expect(screen.getByRole("tab", { name: "Bài tập" })).toHaveAttribute("aria-selected", "true");
  });

  it("creates an exercise with a difficulty from the dialog", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=exercises");
    await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });

    await user.click(screen.getByRole("button", { name: "Thêm bài tập" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm bài tập" });
    await user.type(within(dialog).getByLabelText("Tên bài tập"), "Bài 3: Hỗn số");
    await user.click(within(dialog).getByRole("combobox", { name: /Độ khó/ }));
    await user.click(
      within(await screen.findByRole("listbox")).getByRole("option", { name: "Mức 4" }),
    );
    await user.type(within(dialog).getByLabelText("Mô tả"), "Đổi hỗn số ra phân số.");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await screen.findByText("Đã thêm bài tập")).toBeInTheDocument();
    const row = await screen.findByRole("row", { name: /Bài 3: Hỗn số/ });
    expect(within(row).getByText("Mức 4")).toBeInTheDocument();
    expect(getLibraryStore().exercises.find((e) => e.title === "Bài 3: Hỗn số")).toMatchObject({
      difficulty: 4,
      description: "Đổi hỗn số ra phân số.",
      tags: [],
    });
  });

  it("deletes a free exercise and refuses one in use", async () => {
    const user = userEvent.setup();
    renderPage("/library?tab=exercises");
    await screen.findByRole("row", { name: /Bài 2: So sánh phân số/ });

    await user.click(screen.getByRole("button", { name: "Xoá Bài 2: So sánh phân số" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá bài tập" }),
    );
    expect(await screen.findByText("Đã xoá bài tập")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLibraryStore().exercises.some((e) => e.id === exerciseBai2.id)).toBe(false);
    });

    server.use(
      http.delete(`${API_URL}/library/exercises/:id`, () =>
        HttpResponse.json(fail("EXERCISE_IN_USE", "bài tập đang được gắn vào buổi học"), {
          status: 409,
        }),
      ),
    );
    await user.click(screen.getByRole("button", { name: "Xoá Bài 1: Tập hợp" }));
    await user.click(
      within(await screen.findByRole("dialog")).getByRole("button", { name: "Xoá bài tập" }),
    );
    expect(await screen.findByText("bài tập đang được gắn vào buổi học")).toBeInTheDocument();
    expect(getLibraryStore().exercises.some((e) => e.id === exerciseBai1.id)).toBe(true);
  });
});
