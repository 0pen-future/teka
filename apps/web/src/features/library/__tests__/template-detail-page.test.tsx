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

import { TemplateDetailPage } from "../pages/template-detail-page";
import {
  exerciseBai1,
  exerciseBai2,
  getLibraryStore,
  lessonDraftPhanSo,
  lessonDraftSoTuNhien,
  libraryHandlers,
  materialSlide,
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
  await user.click(screen.getByRole("radio", { name: label }));
}

async function selectTab(user: ReturnType<typeof userEvent.setup>, name: string) {
  await user.click(screen.getByRole("tab", { name }));
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TemplateDetailPage header and version picker", () => {
  it("renders the header, lands on the open draft and lists its lessons in order", async () => {
    renderPage();

    expect(await screen.findByRole("heading", { name: "Toán 6 cơ bản" })).toBeInTheDocument();
    expect(screen.getByText("Mã: TOAN6")).toBeInTheDocument();
    expect(screen.getByText("Toán · Lớp 6")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Kho học liệu" })).toHaveAttribute("href", "/library");

    const picker = screen.getByRole("radiogroup", { name: "Phiên bản" });
    expect(within(picker).getByRole("radio", { name: "v2 · Bản nháp" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
    expect(within(picker).getByRole("radio", { name: "v1 · Đã phát hành" })).toHaveAttribute(
      "aria-checked",
      "false",
    );
    expect(
      screen.getByText("Bản nháp — chỉnh sửa tự do. Kích hoạt để lớp có thể gắn."),
    ).toBeInTheDocument();

    for (const label of [
      "Buổi học",
      "Bài tập",
      "Tài liệu",
      "Nhóm bài tập",
      "Bộ điểm",
      "Nhật ký",
      "Phiên bản",
    ]) {
      expect(screen.getByRole("tab", { name: label })).toBeInTheDocument();
    }
    expect(screen.getByRole("tab", { name: "Buổi học" })).toHaveAttribute("aria-selected", "true");

    const rows = await screen.findAllByRole("row");
    expect(rows.map((row) => row.textContent)).toEqual([
      expect.stringContaining("Buổi học"),
      expect.stringContaining("Số tự nhiên"),
      expect.stringContaining("Phân số"),
    ]);
    const soTuNhien = within(rowOf("Số tự nhiên")).getAllByRole("cell");
    expect(within(rowOf("Số tự nhiên")).getByRole("link", { name: "Số tự nhiên" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}/lessons/${lessonDraftSoTuNhien.id}`,
    );
    expect(soTuNhien[3]).toHaveTextContent("90 phút");
    expect(soTuNhien[4]).toHaveTextContent("1");
    expect(soTuNhien[5]).toHaveTextContent("1");
    const phanSo = within(rowOf("Phân số")).getAllByRole("cell");
    expect(phanSo[3]).toHaveTextContent("—");
    expect(phanSo[4]).toHaveTextContent("0");
    expect(phanSo[5]).toHaveTextContent("0");

    expect(screen.getByText("2 buổi · 90 phút")).toBeInTheDocument();
  });

  it("switches version via the chip picker and updates the banner and tab content", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await pickVersion(user, /v1 · Đã phát hành/);

    expect(
      await screen.findByText(
        "Phiên bản đã phát hành (0 lớp đang gắn) — chỉ xem. Muốn sửa, tạo bản nháp mới.",
      ),
    ).toBeInTheDocument();
    expect(await screen.findByRole("row", { name: /Số tự nhiên/ })).toBeInTheDocument();
    expect(screen.getByRole("radio", { name: "v1 · Đã phát hành" })).toHaveAttribute(
      "aria-checked",
      "true",
    );
  });

  it("shows the not-found block for an unknown template", async () => {
    renderPage("/library/templates/80000000-0000-4000-8000-00000000dead");

    expect(await screen.findByText("Không tìm thấy chương trình mẫu")).toBeInTheDocument();
  });

  it("renders a template whose only version is an empty draft", async () => {
    renderPage(`/library/templates/${templateVan9.id}`);

    expect(await screen.findByRole("heading", { name: "Văn 9 luyện thi" })).toBeInTheDocument();
    expect(
      await screen.findByText("Chưa có buổi mẫu nào — thêm buổi hoặc nhập từ file."),
    ).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Buổi học/ })).toBeInTheDocument();
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
});

describe("TemplateDetailPage lessons tab authoring", () => {
  it("toggles the Bảng/Cây view", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    const toggle = screen.getByRole("radiogroup", { name: "Chế độ xem buổi học" });
    await user.click(within(toggle).getByRole("radio", { name: "Cây" }));

    expect(screen.getByText("Chưa phân đơn vị")).toBeInTheDocument();
    expect(screen.getAllByText("2 buổi · 90 phút")).toHaveLength(2);
    expect(screen.getByText("1 nội dung · 1 bài")).toBeInTheDocument();
    expect(screen.getByText("0 nội dung · 0 bài")).toBeInTheDocument();
  });

  it("filters lessons by title with a debounced search", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.type(screen.getByRole("searchbox", { name: "Tìm buổi theo tiêu đề…" }), "phân");

    await waitFor(() => {
      expect(screen.queryByRole("row", { name: /Số tự nhiên/ })).not.toBeInTheDocument();
    });
    expect(screen.getByRole("row", { name: /Phân số/ })).toBeInTheDocument();
    expect(screen.getByText("1 buổi · 0 phút")).toBeInTheDocument();
  });

  it("moves a lesson up and down through the reorder endpoint", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    expect(within(rowOf("Số tự nhiên")).getByRole("button", { name: "Chuyển lên" })).toBeDisabled();
    expect(within(rowOf("Phân số")).getByRole("button", { name: "Chuyển xuống" })).toBeDisabled();

    await user.click(within(rowOf("Phân số")).getByRole("button", { name: "Chuyển lên" }));

    await waitFor(() => {
      expect(draftPositions()).toEqual(["Phân số", "Số tự nhiên"]);
    });
    const rows = await screen.findAllByRole("row");
    expect(rows[1]).toHaveTextContent("Phân số");
  });

  it("appends a single lesson from the dropdown", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: /Buổi học/ }));
    await user.click(screen.getByRole("menuitem", { name: "Thêm một buổi" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm buổi học" });
    await user.type(within(dialog).getByLabelText("Tên buổi"), "Ôn tập chương 1");
    await user.type(within(dialog).getByLabelText("Thời lượng (phút)"), "60");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    const added = await screen.findByRole("row", { name: /Ôn tập chương 1/ });
    expect(within(added).getByText("60 phút")).toBeInTheDocument();
    expect(draftPositions()).toEqual(["Số tự nhiên", "Phân số", "Ôn tập chương 1"]);
  });

  it("appends several lessons at once with sequential titles", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: /Buổi học/ }));
    await user.click(screen.getByRole("menuitem", { name: "Thêm nhiều buổi" }));
    const dialog = await screen.findByRole("dialog", { name: "Thêm nhiều buổi" });
    await user.clear(within(dialog).getByLabelText("Số buổi"));
    await user.type(within(dialog).getByLabelText("Số buổi"), "3");
    await user.click(within(dialog).getByRole("button", { name: "Thêm" }));

    expect(await screen.findByText("Đã thêm 3 buổi")).toBeInTheDocument();
    await waitFor(() => {
      expect(draftPositions()).toEqual(["Số tự nhiên", "Phân số", "Buổi 1", "Buổi 2", "Buổi 3"]);
    });
  });

  it("duplicates a lesson through the /duplicate route", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(within(rowOf("Số tự nhiên")).getByRole("button", { name: "Nhân bản" }));

    expect(await screen.findByText("Đã nhân bản buổi học")).toBeInTheDocument();
    await waitFor(() => {
      expect(draftPositions()).toEqual(["Số tự nhiên", "Số tự nhiên (bản sao)", "Phân số"]);
    });
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

  it("clears every lesson after confirming, and hides the action once empty", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await user.click(screen.getByRole("button", { name: "Xoá tất cả" }));
    const dialog = await screen.findByRole("dialog", { name: "Xoá tất cả buổi học?" });
    await user.click(within(dialog).getByRole("button", { name: "Xoá tất cả" }));

    expect(await screen.findByText("Đã xoá tất cả buổi học")).toBeInTheDocument();
    expect(draftPositions()).toEqual([]);
    expect(
      await screen.findByText("Chưa có buổi mẫu nào — thêm buổi hoặc nhập từ file."),
    ).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Xoá tất cả" })).not.toBeInTheDocument();
  });

  it("hides authoring controls once the version is published", async () => {
    renderPage(`/library/templates/${templateToan6.id}?v=1`);
    await screen.findByRole("row", { name: /Phân số/ });

    expect(screen.queryByRole("button", { name: "Chuyển lên" })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Buổi học/ })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Xoá tất cả" })).not.toBeInTheDocument();
  });
});

describe("TemplateDetailPage aggregate tabs", () => {
  it("dedupes exercises by usage, counts lessons and supports search", async () => {
    const user = userEvent.setup();
    // A second usage of the same exercise from another lesson in the draft.
    getLibraryStore().exerciseLinks.push({
      lesson_id: lessonDraftPhanSo.id,
      exercise_id: exerciseBai1.id,
      group_id: null,
      position: 1,
    });
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await selectTab(user, "Bài tập");
    const row = await screen.findByRole("row", { name: /Bài 1: Tập hợp/ });
    expect(within(row).getByText("2 buổi")).toBeInTheDocument();
    expect(row).toHaveTextContent("BT-0001");

    await user.type(screen.getByRole("searchbox", { name: /Tìm bài tập/ }), "bài 2");
    await waitFor(() => {
      expect(screen.getByText("Không có bài tập nào khớp.")).toBeInTheDocument();
    });
  });

  it("dedupes materials by usage and counts lessons", async () => {
    const user = userEvent.setup();
    getLibraryStore().materialLinks.push({
      lesson_id: lessonDraftPhanSo.id,
      material_id: materialSlide.id,
      shared_with_students: false,
      position: 2,
    });
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await selectTab(user, "Tài liệu");
    const row = await screen.findByRole("row", { name: /Slide số tự nhiên/ });
    expect(within(row).getByText("2 buổi")).toBeInTheDocument();
  });

  it("shows the empty state on a version with no lessons", async () => {
    const user = userEvent.setup();
    renderPage(`/library/templates/${templateVan9.id}`);
    await screen.findByRole("heading", { name: "Văn 9 luyện thi" });

    await selectTab(user, "Bài tập");
    expect(await screen.findByText("Không có bài tập nào khớp.")).toBeInTheDocument();
    await selectTab(user, "Tài liệu");
    expect(await screen.findByText("Không có tài liệu nào khớp.")).toBeInTheDocument();
  });
});

describe("TemplateDetailPage exercise groups tab", () => {
  it("adds a group by pressing Enter and deletes an unused group without confirming", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await selectTab(user, "Nhóm bài tập");
    expect(await screen.findByText("Chưa có nhóm nào.")).toBeInTheDocument();

    await user.type(screen.getByRole("textbox", { name: "Tên nhóm mới…" }), "Nhóm A{Enter}");

    expect(await screen.findByText("Đã thêm nhóm bài tập")).toBeInTheDocument();
    const row = await screen.findByText("Nhóm A");
    expect(screen.getByText("Dùng trong 0 bài")).toBeInTheDocument();

    await user.click(within(row.closest("li")!).getByRole("button", { name: "Xoá" }));
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    await waitFor(() => {
      expect(screen.queryByText("Nhóm A")).not.toBeInTheDocument();
    });
  });

  it("requires confirmation to delete a group still holding exercises", async () => {
    const user = userEvent.setup();
    const store = getLibraryStore();
    const groupId = "86000000-0000-4000-8000-000000000001";
    store.exerciseGroups.push({
      id: groupId,
      version_id: versionToan6Draft.id,
      name: "Nhóm B",
      position: 1,
    });
    store.exerciseLinks.push({
      lesson_id: lessonDraftPhanSo.id,
      exercise_id: exerciseBai2.id,
      group_id: groupId,
      position: 2,
    });
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    await selectTab(user, "Nhóm bài tập");
    expect(await screen.findByText("Dùng trong 1 bài")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Xoá" }));
    const dialog = await screen.findByRole("dialog", { name: /Xoá nhóm "Nhóm B"/ });
    expect(
      within(dialog).getByText("Bài tập trong nhóm sẽ về 'Chưa phân nhóm'."),
    ).toBeInTheDocument();
    await user.click(within(dialog).getByRole("button", { name: "Xoá nhóm" }));

    expect(await screen.findByText("Đã xoá nhóm bài tập")).toBeInTheDocument();
    expect(store.exerciseGroups.some((g) => g.id === groupId)).toBe(false);
    expect(store.exerciseLinks.find((l) => l.exercise_id === exerciseBai2.id)?.group_id).toBeNull();
  });
});

describe("TemplateDetailPage score sets tab", () => {
  it("warns when the weight sum is off 100, clears once fixed, and saves the array body", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Bộ điểm");

    const scores = await screen.findByRole("region", { name: "Cơ cấu điểm" });
    const sumLine = within(scores).getByText(/Tổng trọng số:/);
    expect(sumLine).toHaveTextContent("Tổng trọng số: 1%");
    expect(sumLine).toHaveClass("text-coral-600");

    await user.clear(within(scores).getByRole("spinbutton", { name: "Trọng số 1.1" }));
    await user.type(within(scores).getByRole("spinbutton", { name: "Trọng số 1.1" }), "40");
    await user.clear(within(scores).getByRole("spinbutton", { name: "Trọng số 1.2" }));
    await user.type(within(scores).getByRole("spinbutton", { name: "Trọng số 1.2" }), "60");

    expect(within(scores).getByText("Tổng trọng số: 100%")).not.toHaveClass("text-coral-600");

    await user.click(within(scores).getByRole("button", { name: "Lưu cơ cấu điểm" }));
    expect(await screen.findByText("Đã lưu cơ cấu điểm")).toBeInTheDocument();
    expect(getLibraryStore().scoreSets[versionToan6Draft.id]).toEqual([
      {
        key: "main",
        title: "Bộ điểm",
        components: [
          { key: "hw", label: "Bài tập về nhà", max: 10, weight: 40 },
          { key: "kt", label: "Kiểm tra", max: 10, weight: 60 },
        ],
      },
    ]);
  });

  it("maps a nested API validation error onto the right group and row", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Bộ điểm");

    const scores = await screen.findByRole("region", { name: "Cơ cấu điểm" });
    await user.click(within(scores).getByRole("button", { name: "+ Thêm bộ điểm" }));
    const addRowButtons = within(scores).getAllByRole("button", {
      name: "+ Thêm điểm thành phần",
    });
    await user.click(addRowButtons[addRowButtons.length - 1]!);
    const secondGroupCode = within(scores).getByRole("textbox", { name: "Mã thành phần 2.1" });
    await user.type(secondGroupCode, "gk");
    await user.type(within(scores).getByRole("textbox", { name: "Tên thành phần 2.1" }), "Giữa kỳ");

    server.use(
      http.put(`${API_URL}/library/versions/:vid/score-set`, () =>
        HttpResponse.json(
          fail("VALIDATION_ERROR", "validation failed", {
            "1.components.0.weight": "trọng số quá lớn",
          }),
          { status: 422 },
        ),
      ),
    );
    await user.click(within(scores).getByRole("button", { name: "Lưu cơ cấu điểm" }));

    expect(await within(scores).findByText("trọng số quá lớn")).toBeInTheDocument();
  });

  it("shows the published version's score set read-only", async () => {
    const user = userEvent.setup();
    renderPage(`/library/templates/${templateToan6.id}?v=1`);
    await screen.findByRole("row", { name: /Số tự nhiên/ });
    await selectTab(user, "Bộ điểm");

    const scores = await screen.findByRole("region", { name: "Cơ cấu điểm" });
    expect(within(scores).getByText("Kiểm tra")).toBeInTheDocument();
    expect(
      within(scores).queryByRole("button", { name: "Lưu cơ cấu điểm" }),
    ).not.toBeInTheDocument();
  });
});

describe("TemplateDetailPage log fields tab", () => {
  it("adds a student field from the restricted new-row kinds", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Nhật ký");

    const fields = await screen.findByRole("region", { name: "Trường nhật ký" });
    await user.click(within(fields).getByRole("button", { name: "Thêm trường" }));
    await user.type(
      within(fields).getByRole("textbox", { name: "Nhãn trường 2" }),
      "Học sinh vắng",
    );
    await user.click(within(fields).getByRole("combobox", { name: /Loại trường 2/ }));
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).getAllByRole("option")).toHaveLength(4);
    await user.click(within(listbox).getByRole("option", { name: "Chọn học sinh" }));
    await user.click(within(fields).getByRole("button", { name: "Lưu trường nhật ký" }));

    expect(await screen.findByText("Đã lưu trường nhật ký")).toBeInTheDocument();
    expect(
      getLibraryStore()
        .logFields.filter((f) => f.version_id === versionToan6Draft.id)
        .map((f) => [f.label, f.kind]),
    ).toEqual([
      ["Ghi chú buổi học", "text"],
      ["Học sinh vắng", "student"],
    ]);
  });

  it("keeps a legacy select row editable without offering select to new rows", async () => {
    const user = userEvent.setup();
    const store = getLibraryStore();
    store.logFields.push({
      id: "85000000-0000-4000-8000-000000000003",
      version_id: versionToan6Draft.id,
      position: 2,
      label: "Mức độ tập trung",
      kind: "select",
      options: ["Tốt", "Khá"],
      required: true,
    });
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Nhật ký");

    const fields = await screen.findByRole("region", { name: "Trường nhật ký" });
    expect(within(fields).getByRole("combobox", { name: "Loại trường 2" })).toHaveTextContent(
      "Chọn một",
    );
    expect(within(fields).getByRole("textbox", { name: "Tuỳ chọn trường 2" })).toHaveValue(
      "Tốt, Khá",
    );

    await user.click(within(fields).getByRole("combobox", { name: "Loại trường 1" }));
    const listbox = await screen.findByRole("listbox");
    expect(within(listbox).queryByRole("option", { name: "Chọn một" })).not.toBeInTheDocument();
    expect(within(listbox).getAllByRole("option")).toHaveLength(4);
  });

  it("shows the published version's log fields read-only", async () => {
    const user = userEvent.setup();
    renderPage(`/library/templates/${templateToan6.id}?v=1`);
    await screen.findByRole("row", { name: /Số tự nhiên/ });
    await selectTab(user, "Nhật ký");

    const fields = await screen.findByRole("region", { name: "Trường nhật ký" });
    expect(within(fields).getByText("Mức độ tập trung")).toBeInTheDocument();
    expect(within(fields).getByText("Chọn một")).toBeInTheDocument();
    expect(within(fields).getByText("Tốt, Khá")).toBeInTheDocument();
    expect(within(fields).queryByRole("textbox")).not.toBeInTheDocument();
    expect(
      within(fields).queryByRole("button", { name: "Lưu trường nhật ký" }),
    ).not.toBeInTheDocument();
  });
});

describe("TemplateDetailPage versions tab", () => {
  it("lists every version, selects one on row click, and shows Chưa có lớp nào gắn", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Phiên bản");

    expect(screen.getAllByText("Chưa có lớp nào gắn")).toHaveLength(2);
    await user.click(screen.getByRole("button", { name: "Chọn v1" }));

    await waitFor(() => {
      expect(screen.getByRole("radio", { name: "v1 · Đã phát hành" })).toHaveAttribute(
        "aria-checked",
        "true",
      );
    });
  });

  it("shows class chips linking to their class when a version has bound classes", async () => {
    const user = userEvent.setup();
    getLibraryStore().classLinks.push(
      { version_id: versionToan6Published.id, class_id: "class-1", class_name: "Lớp A" },
      { version_id: versionToan6Published.id, class_id: "class-2", class_name: "Lớp B" },
    );
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Phiên bản");

    expect(await screen.findByRole("link", { name: "Lớp A" })).toHaveAttribute(
      "href",
      "/classes/class-1",
    );
    expect(screen.getByRole("link", { name: "Lớp B" })).toHaveAttribute("href", "/classes/class-2");
  });

  it("publishes the draft from Kích hoạt and stops the published version from Ngừng", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });
    await selectTab(user, "Phiên bản");

    const draftRow = screen.getByRole("button", { name: "Chọn v2" });
    await user.click(within(draftRow).getByRole("button", { name: "Kích hoạt" }));
    expect(await screen.findByText("Đã kích hoạt v2")).toBeInTheDocument();
    expect(getLibraryStore().versions.find((v) => v.id === versionToan6Draft.id)?.status).toBe(
      "published",
    );

    const publishedRow = screen.getByRole("button", { name: "Chọn v1" });
    await user.click(within(publishedRow).getByRole("button", { name: "Ngừng" }));
    const dialog = await screen.findByRole("dialog", { name: "Ngừng phiên bản v1?" });
    await user.click(within(dialog).getByRole("button", { name: "Ngừng" }));

    expect(await screen.findByText("Đã ngừng v1")).toBeInTheDocument();
    expect(getLibraryStore().versions.find((v) => v.id === versionToan6Published.id)?.status).toBe(
      "archived",
    );
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

    for (const name of [/Kích hoạt/, /Buổi học/, "Sửa", "Xoá chương trình", "Chuyển lên"]) {
      expect(screen.queryByRole("button", { name })).not.toBeInTheDocument();
    }
    expect(screen.getByRole("radiogroup", { name: "Phiên bản" })).toBeInTheDocument();
  });

  it("authors but never publishes with library.edit alone", async () => {
    server.use(memberWith("library.read", "library.edit"));
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    expect(screen.getByRole("button", { name: /Buổi học/ })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Sửa" })).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Chuyển lên" }).length).toBeGreaterThan(0);
    expect(screen.queryByRole("button", { name: "Kích hoạt" })).not.toBeInTheDocument();
  });

  it("publishes with library.publish, which carries the read", async () => {
    server.use(memberWith("library.publish"));
    renderPage();
    await screen.findByRole("row", { name: /Phân số/ });

    expect(screen.getByRole("button", { name: "Kích hoạt" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Buổi học/ })).not.toBeInTheDocument();
  });
});
