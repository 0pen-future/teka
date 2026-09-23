import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

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
  getLibraryStore,
  libraryHandlers,
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

  it("keeps the later-phase tabs disabled and marks the templates tab selected", async () => {
    renderPage();
    await screen.findByRole("row", { name: /Toán 6 cơ bản/ });

    expect(screen.getByRole("tab", { name: "Chương trình mẫu" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByRole("tab", { name: "Học liệu" })).toBeDisabled();
    expect(screen.getByRole("tab", { name: "Bài tập" })).toBeDisabled();
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
