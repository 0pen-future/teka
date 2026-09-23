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

import { TemplateDetailPage } from "../pages/template-detail-page";
import {
  getLibraryStore,
  lessonDraftPhanSo,
  lessonDraftSoTuNhien,
  libraryHandlers,
  resetLibraryStore,
  templateToan6,
  templateVan9,
  versionToan6Draft,
  versionToan6Published,
} from "./library-handlers";

function renderPage(route = `/library/templates/${templateToan6.id}`) {
  return renderWithProviders(<TemplateDetailPage />, {
    route,
    path: "/library/templates/:id",
    extraRoutes: [
      { path: "/library", element: <div>library-stub</div> },
      { path: "/library/templates/:id/lessons/:lessonId", element: <div>lesson-stub</div> },
    ],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function rowOf(name: string) {
  return screen.getByRole("row", { name: new RegExp(name) });
}

function draftPositions(): string[] {
  return getLibraryStore()
    .lessons.filter((l) => l.version_id === versionToan6Draft.id)
    .sort((a, b) => a.position - b.position)
    .map((l) => l.title);
}

async function pickVersion(user: ReturnType<typeof userEvent.setup>, label: RegExp) {
  await user.click(screen.getByRole("combobox", { name: "Phiên bản" }));
  await user.click(within(await screen.findByRole("listbox")).getByRole("option", { name: label }));
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TemplateDetailPage as the owner", () => {
  it("renders the header, lands on the open draft and lists its lessons in order", async () => {
    renderPage();

    expect(await screen.findByRole("heading", { name: "Toán 6 cơ bản" })).toBeInTheDocument();
    expect(screen.getByText("Mã: TOAN6")).toBeInTheDocument();
    expect(screen.getByText("Toán · Lớp 6")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Kho học liệu" })).toHaveAttribute("href", "/library");

    expect(await screen.findByRole("combobox", { name: "Phiên bản" })).toHaveTextContent(
      "v2 · Bản nháp",
    );
    expect(screen.getByText("Thêm buổi ôn tập")).toBeInTheDocument();
    const rows = await screen.findAllByRole("row");
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("Tên buổi"),
      expect.stringContaining("Số tự nhiên"),
      expect.stringContaining("Phân số"),
    ]);
    expect(within(rowOf("Số tự nhiên")).getByRole("link", { name: "Số tự nhiên" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}/lessons/${lessonDraftSoTuNhien.id}`,
    );
    expect(within(rowOf("Số tự nhiên")).getByText("Có")).toBeInTheDocument();
    expect(within(rowOf("Phân số")).getByText("—")).toBeInTheDocument();
  });

  it("moves a lesson up and down through the reorder endpoint", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    // The first row cannot move up, the last cannot move down.
    expect(within(rowOf("Số tự nhiên")).getByRole("button", { name: "Chuyển lên" })).toBeDisabled();
    expect(within(rowOf("Phân số")).getByRole("button", { name: "Chuyển xuống" })).toBeDisabled();

    await user.click(within(rowOf("Phân số")).getByRole("button", { name: "Chuyển lên" }));

    await waitFor(() => {
      expect(draftPositions()).toEqual(["Phân số", "Số tự nhiên"]);
    });
    const rows = await screen.findAllByRole("row");
    expect(rows[1]).toHaveTextContent("Phân số");
  });

  it("appends a lesson from the dialog", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: "Thêm buổi học" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm buổi học" });
    await user.type(within(dialog).getByLabelText("Tên buổi"), "Ôn tập chương 1");
    await user.type(within(dialog).getByLabelText("Thời lượng (phút)"), "60");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    const added = await screen.findByRole("row", { name: /Ôn tập chương 1/ });
    expect(within(added).getByText("60 phút")).toBeInTheDocument();
    expect(draftPositions()).toEqual(["Số tự nhiên", "Phân số", "Ôn tập chương 1"]);
  });

  it("removes a lesson after confirming", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(within(rowOf("Số tự nhiên")).getByRole("button", { name: "Xoá" }));
    const dialog = await screen.findByRole("dialog", { name: /Xoá buổi "Số tự nhiên"/ });
    await user.click(within(dialog).getByRole("button", { name: "Xoá buổi" }));

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Số tự nhiên/ })).not.toBeInTheDocument();
    });
    expect(draftPositions()).toEqual(["Phân số"]);
    expect(getLibraryStore().lessons.find((l) => l.id === lessonDraftPhanSo.id)?.position).toBe(1);
  });

  it("publishes the draft, which locks the lessons and offers a new draft", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: "Phát hành" }));
    const dialog = await screen.findByRole("dialog", { name: /Phát hành v2/ });
    await user.click(within(dialog).getByRole("button", { name: "Phát hành" }));

    expect(await screen.findByText("Đã phát hành v2")).toBeInTheDocument();
    expect(getLibraryStore().versions.find((v) => v.id === versionToan6Draft.id)?.status).toBe(
      "published",
    );
    expect(await screen.findByText(/Phiên bản này đã phát hành/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm buổi học" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Chuyển lên" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tạo bản nháp mới" })).toBeInTheDocument();
  });

  it("opens a new draft that copies the published lessons", async () => {
    const user = userEvent.setup();
    // No open draft: only the published v1 remains.
    const store = getLibraryStore();
    store.versions = store.versions.filter((v) => v.id !== versionToan6Draft.id);
    store.lessons = store.lessons.filter((l) => l.version_id !== versionToan6Draft.id);
    renderPage();
    expect(await screen.findByRole("combobox", { name: "Phiên bản" })).toHaveTextContent(
      "v1 · Đã phát hành",
    );

    await user.click(screen.getByRole("button", { name: "Tạo bản nháp mới" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo bản nháp mới" });
    await user.type(within(dialog).getByLabelText("Ghi chú thay đổi"), "Sửa buổi 2");
    await user.click(within(dialog).getByRole("button", { name: "Tạo bản nháp" }));

    expect(await screen.findByText("Đã tạo bản nháp v2")).toBeInTheDocument();
    await waitFor(() => {
      expect(screen.getByRole("combobox", { name: "Phiên bản" })).toHaveTextContent(
        "v2 · Bản nháp",
      );
    });
    expect(await screen.findByRole("row", { name: /Số tự nhiên/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thêm buổi học" })).toBeInTheDocument();
    const draft = getLibraryStore().versions.find(
      (v) => v.template_id === templateToan6.id && v.status === "draft",
    );
    expect(draft?.changelog).toBe("Sửa buổi 2");
  });

  it("archives the published version from the picker", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await pickVersion(user, /v1 · Đã phát hành/);
    expect(await screen.findByText(/Phiên bản này đã phát hành/)).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Lưu trữ" }));
    const dialog = await screen.findByRole("dialog", { name: /Lưu trữ v1/ });
    await user.click(within(dialog).getByRole("button", { name: "Lưu trữ" }));

    expect(await screen.findByText("Đã lưu trữ v1")).toBeInTheDocument();
    expect(getLibraryStore().versions.find((v) => v.id === versionToan6Published.id)?.status).toBe(
      "archived",
    );
  });

  it("surfaces the API conflict when the version changed under the caller", async () => {
    const user = userEvent.setup();
    server.use(
      http.post(`${API_URL}/library/versions/:vid/publish`, () =>
        HttpResponse.json(
          {
            success: false,
            error: { code: "VERSION_NOT_DRAFT", message: "đã có người phát hành" },
          },
          { status: 409 },
        ),
      ),
    );
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: "Phát hành" }));
    const dialog = await screen.findByRole("dialog", { name: /Phát hành v2/ });
    await user.click(within(dialog).getByRole("button", { name: "Phát hành" }));

    expect(await screen.findByText("đã có người phát hành")).toBeInTheDocument();
  });

  it("edits the template's own fields from the dialog", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("button", { name: "Sửa" }));
    const dialog = await screen.findByRole("dialog", { name: "Sửa chương trình mẫu" });
    expect(within(dialog).getByLabelText("Mã chương trình")).toHaveValue("TOAN6");
    await user.clear(within(dialog).getByLabelText("Tên chương trình"));
    await user.type(within(dialog).getByLabelText("Tên chương trình"), "Toán 6 nâng cao");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    expect(await screen.findByRole("heading", { name: "Toán 6 nâng cao" })).toBeInTheDocument();
    expect(getLibraryStore().templates.find((t) => t.id === templateToan6.id)?.name).toBe(
      "Toán 6 nâng cao",
    );
  });

  it("deletes the template after confirming and returns to the library", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Toán 6 cơ bản" });

    await user.click(screen.getByRole("button", { name: "Xoá chương trình" }));
    const dialog = await screen.findByRole("dialog", { name: /Xoá chương trình "Toán 6 cơ bản"/ });
    await user.click(within(dialog).getByRole("button", { name: "Xoá chương trình" }));

    expect(await screen.findByText("library-stub")).toBeInTheDocument();
    expect(getLibraryStore().templates.some((t) => t.id === templateToan6.id)).toBe(false);
  });

  it("shows the not-found block for an unknown template", async () => {
    renderPage("/library/templates/80000000-0000-4000-8000-00000000dead");

    expect(await screen.findByText("Không tìm thấy chương trình mẫu")).toBeInTheDocument();
  });

  it("renders a template whose only version is an empty draft", async () => {
    renderPage(`/library/templates/${templateVan9.id}`);

    expect(await screen.findByRole("heading", { name: "Văn 9 luyện thi" })).toBeInTheDocument();
    expect(await screen.findByText("Phiên bản này chưa có buổi học nào.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Thêm buổi học" })).toBeInTheDocument();
  });
});

describe("TemplateDetailPage as a member", () => {
  beforeEach(() => {
    signInAs(testSecondaryTeacher);
  });

  it("is read-only with library.read alone", async () => {
    server.use(memberWith("library.read"));
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    for (const name of ["Phát hành", "Thêm buổi học", "Sửa", "Xoá chương trình", "Chuyển lên"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(screen.getByRole("combobox", { name: "Phiên bản" })).toBeInTheDocument();
  });

  it("authors but never publishes with library.edit alone", async () => {
    server.use(memberWith("library.read", "library.edit"));
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    expect(screen.getByRole("button", { name: "Thêm buổi học" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sửa" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Chuyển lên" }).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Phát hành" })).not.toBeInTheDocument();
  });

  it("publishes with library.publish, which carries the read", async () => {
    server.use(memberWith("library.publish"));
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    expect(screen.getByRole("button", { name: "Phát hành" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Thêm buổi học" })).not.toBeInTheDocument();
  });
});
