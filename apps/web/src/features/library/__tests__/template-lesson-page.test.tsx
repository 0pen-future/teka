import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, listMeta, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import {
  renderWithProviders,
  signInAs,
  testPrimaryTeacher,
  testSecondaryTeacher,
} from "@/test/utils";

import { TemplateLessonPage } from "../pages/template-lesson-page";
import {
  exerciseBai1,
  exerciseBai2,
  getLibraryStore,
  lessonDraftSoTuNhien,
  lessonPublishedSoTuNhien,
  libraryHandlers,
  materialSlide,
  materialVideo,
  resetLibraryStore,
  templateToan6,
  versionToan6Draft,
} from "./library-handlers";

function renderPage(lessonId = lessonDraftSoTuNhien.id) {
  return renderWithProviders(<TemplateLessonPage />, {
    route: `/library/templates/${templateToan6.id}/lessons/${lessonId}`,
    path: "/library/templates/:id/lessons/:lessonId",
    extraRoutes: [{ path: "/library/templates/:id", element: <div>template-stub</div> }],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TemplateLessonPage", () => {
  it("prefills the draft lesson form and saves the edited fields", async () => {
    const user = userEvent.setup();
    renderPage();

    expect(
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" }),
    ).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Toán 6 cơ bản" })).toHaveAttribute(
      "href",
      `/library/templates/${templateToan6.id}`,
    );
    expect(screen.getByText("v2 · Bản nháp")).toBeInTheDocument();
    expect(screen.getByLabelText("Mục tiêu")).toHaveValue("Nhận biết tập N");
    expect(screen.getByLabelText("Thời lượng (phút)")).toHaveValue("90");
    expect(screen.getByLabelText("Bài tập về nhà")).toHaveValue("Bài 1-5 trang 10");

    await user.clear(screen.getByLabelText("Thời lượng (phút)"));
    await user.type(screen.getByLabelText("Thời lượng (phút)"), "120");
    await user.clear(screen.getByLabelText("Bài tập về nhà"));
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Đã lưu buổi học")).toBeInTheDocument();
    await waitFor(() => {
      expect(getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id)).toMatchObject(
        { duration_min: 120, homework_note: null, objectives: "Nhận biết tập N" },
      );
    });
  });

  it("rejects a non-numeric duration before calling the API", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByLabelText("Thời lượng (phút)");

    await user.clear(screen.getByLabelText("Thời lượng (phút)"));
    await user.type(screen.getByLabelText("Thời lượng (phút)"), "abc");
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText("Nhập số phút nguyên")).toBeInTheDocument();
    expect(
      getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id)?.duration_min,
    ).toBe(90);
  });

  it("renames the lesson and refreshes the heading", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByLabelText("Tên buổi");

    await user.clear(screen.getByLabelText("Tên buổi"));
    await user.type(screen.getByLabelText("Tên buổi"), "Tập hợp số tự nhiên");
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(
      await screen.findByRole("heading", { name: "Buổi 1 · Tập hợp số tự nhiên" }),
    ).toBeInTheDocument();
  });

  it("shows a published lesson read-only with the locked notice", async () => {
    renderPage(lessonPublishedSoTuNhien.id);

    expect(
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" }),
    ).toBeInTheDocument();
    expect(await screen.findByText(/Phiên bản v1 đã phát hành/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Mục tiêu" })).not.toBeInTheDocument();
    const objectives = screen.getByRole("group", { name: "Mục tiêu" });
    expect(within(objectives).getByText("Chưa có")).toBeInTheDocument();
    expect(
      within(screen.getByRole("group", { name: "Thời lượng" })).getByText("90 phút"),
    ).toBeInTheDocument();
  });

  it("is read-only for a member without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage();

    expect(
      await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" }),
    ).toBeInTheDocument();
    expect(await screen.findByText("Bạn không có quyền soạn buổi học mẫu.")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("group", { name: "Mục tiêu" })).getByText("Nhận biết tập N"),
    ).toBeInTheDocument();
  });

  it("shows the not-found block for an unknown lesson", async () => {
    renderPage("82000000-0000-4000-8000-00000000dead");

    expect(await screen.findByText("Không tìm thấy buổi học mẫu")).toBeInTheDocument();
  });

  it("locks the form when the save is refused because the version was published meanwhile", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });

    // Someone else published the draft between the page load and the save.
    server.use(
      http.put(`${API_URL}/library/lessons/:lid`, () => {
        const version = getLibraryStore().versions.find((v) => v.id === versionToan6Draft.id);
        if (version) version.status = "published";
        return HttpResponse.json(fail("VERSION_LOCKED", "Phiên bản đã phát hành"), { status: 409 });
      }),
    );
    await user.type(screen.getByRole("textbox", { name: "Mục tiêu" }), " thêm");
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    expect(await screen.findByText(/Phiên bản v2 đã phát hành/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(screen.queryByRole("textbox", { name: "Mục tiêu" })).not.toBeInTheDocument();
  });

  it("attaches materials with the share flag and exercises, saving each block separately", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("heading", { name: "Buổi 1 · Số tự nhiên" });

    const materials = await screen.findByRole("region", { name: "Học liệu" });
    expect(within(materials).getByRole("checkbox", { name: "Slide số tự nhiên" })).toBeChecked();
    expect(
      within(materials).getByRole("checkbox", { name: "Chia sẻ Slide số tự nhiên với học viên" }),
    ).toBeChecked();
    expect(within(materials).getByRole("checkbox", { name: "Video phân số" })).not.toBeChecked();
    expect(
      within(materials).queryByRole("checkbox", { name: "Chia sẻ Video phân số với học viên" }),
    ).not.toBeInTheDocument();

    await user.click(within(materials).getByRole("checkbox", { name: "Video phân số" }));
    await user.click(
      within(materials).getByRole("checkbox", { name: "Chia sẻ Slide số tự nhiên với học viên" }),
    );
    await user.click(within(materials).getByRole("button", { name: "Lưu học liệu" }));

    expect(await screen.findByText("Đã lưu học liệu của buổi")).toBeInTheDocument();
    expect(
      getLibraryStore()
        .materialLinks.filter((l) => l.lesson_id === lessonDraftSoTuNhien.id)
        .map((l) => [l.material_id, l.shared_with_students, l.position]),
    ).toEqual([
      [materialSlide.id, false, 1],
      [materialVideo.id, false, 2],
    ]);

    const exercises = screen.getByRole("region", { name: "Bài tập" });
    expect(within(exercises).getByRole("checkbox", { name: "Bài 1: Tập hợp" })).toBeChecked();
    await user.click(within(exercises).getByRole("checkbox", { name: "Bài 1: Tập hợp" }));
    await user.click(within(exercises).getByRole("checkbox", { name: "Bài 2: So sánh phân số" }));
    await user.click(within(exercises).getByRole("button", { name: "Lưu bài tập" }));

    expect(await screen.findByText("Đã lưu bài tập của buổi")).toBeInTheDocument();
    expect(
      getLibraryStore()
        .exerciseLinks.filter((l) => l.lesson_id === lessonDraftSoTuNhien.id)
        .map((l) => l.exercise_id),
    ).toEqual([exerciseBai2.id]);
    expect(getLibraryStore().exerciseLinks.some((l) => l.exercise_id === exerciseBai1.id)).toBe(
      false,
    );
  });

  it("narrows the material picker with the search box", async () => {
    const user = userEvent.setup();
    renderPage();
    const materials = await screen.findByRole("region", { name: "Học liệu" });
    await within(materials).findByRole("checkbox", { name: "Video phân số" });

    await user.type(within(materials).getByRole("searchbox", { name: "Tìm học liệu" }), "slide");

    await waitFor(() => {
      expect(
        within(materials).queryByRole("checkbox", { name: "Video phân số" }),
      ).not.toBeInTheDocument();
    });
    expect(within(materials).getByRole("checkbox", { name: "Slide số tự nhiên" })).toBeChecked();
  });

  it("keeps an attached material the catalog page no longer lists so it can be unticked", async () => {
    const user = userEvent.setup();
    server.use(
      http.get(`${API_URL}/library/materials`, () =>
        HttpResponse.json(ok([materialVideo], listMeta(1, 1, 100))),
      ),
    );
    renderPage();
    const materials = await screen.findByRole("region", { name: "Học liệu" });
    await within(materials).findByRole("checkbox", { name: "Video phân số" });

    const attached = within(materials).getByRole("checkbox", { name: "Slide số tự nhiên" });
    expect(attached).toBeChecked();
    await user.click(attached);
    await user.click(within(materials).getByRole("button", { name: "Lưu học liệu" }));

    expect(await screen.findByText("Đã lưu học liệu của buổi")).toBeInTheDocument();
    expect(
      getLibraryStore().materialLinks.filter((l) => l.lesson_id === lessonDraftSoTuNhien.id),
    ).toEqual([]);
  });

  it("lists the attachments read-only on a published lesson", async () => {
    renderPage(lessonPublishedSoTuNhien.id);
    await screen.findByText(/Phiên bản v1 đã phát hành/);

    const materials = await screen.findByRole("region", { name: "Học liệu" });
    expect(within(materials).getByText("Slide số tự nhiên")).toBeInTheDocument();
    expect(within(materials).getByText("Tài liệu")).toBeInTheDocument();
    expect(within(materials).queryByText("Chia sẻ HV")).not.toBeInTheDocument();
    expect(within(materials).queryByRole("checkbox")).not.toBeInTheDocument();
    expect(
      within(materials).queryByRole("button", { name: "Lưu học liệu" }),
    ).not.toBeInTheDocument();
    expect(
      within(screen.getByRole("region", { name: "Bài tập" })).getByText("Chưa gắn bài tập."),
    ).toBeInTheDocument();
  });

  it("shows the shared badge for a reader without library.edit", async () => {
    server.use(memberWith("library.read"));
    signInAs(testSecondaryTeacher);
    renderPage();
    await screen.findByText("Bạn không có quyền soạn buổi học mẫu.");

    const materials = await screen.findByRole("region", { name: "Học liệu" });
    expect(within(materials).getByText("Slide số tự nhiên")).toBeInTheDocument();
    expect(within(materials).getByText("Chia sẻ HV")).toBeInTheDocument();
    expect(within(materials).queryByRole("checkbox")).not.toBeInTheDocument();
  });

  it("edits the preparation status and checklist and saves them as one block", async () => {
    const user = userEvent.setup();
    renderPage();

    const prep = await screen.findByRole("region", { name: "Chuẩn bị tài liệu" });
    expect(within(prep).getByRole("radio", { name: "Cần làm" })).toBeChecked();
    expect(within(prep).getByRole("checkbox", { name: "Soạn slide" })).toBeChecked();
    expect(within(prep).getByRole("checkbox", { name: "In phiếu bài tập" })).not.toBeChecked();

    await user.click(within(prep).getByRole("radio", { name: "Đang làm" }));
    await user.click(within(prep).getByRole("checkbox", { name: "In phiếu bài tập" }));
    await user.type(within(prep).getByLabelText("Thêm việc cần làm"), "Chuẩn bị đề{Enter}");
    expect(within(prep).getByRole("checkbox", { name: "Chuẩn bị đề" })).not.toBeChecked();
    await user.click(within(prep).getByRole("button", { name: "Xoá Soạn slide" }));
    await user.click(within(prep).getByRole("button", { name: "Lưu chuẩn bị" }));

    expect(await screen.findByText("Đã lưu trạng thái chuẩn bị")).toBeInTheDocument();
    const saved = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id);
    expect(saved?.prep_status).toBe("doing");
    expect(saved?.checklist).toEqual([
      { label: "In phiếu bài tập", done: true },
      { label: "Chuẩn bị đề", done: false },
    ]);
  });

  it("shows the preparation block read-only on a published lesson", async () => {
    renderPage(lessonPublishedSoTuNhien.id);

    const prep = await screen.findByRole("region", { name: "Chuẩn bị tài liệu" });
    expect(within(prep).getByText("Cần làm")).toBeInTheDocument();
    expect(within(prep).getByText("Chưa có việc nào")).toBeInTheDocument();
    expect(within(prep).queryByRole("button", { name: "Lưu chuẩn bị" })).not.toBeInTheDocument();
  });

  it("shows the assignee and due date on the preparation block", async () => {
    const row = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id)!;
    row.assignee_id = testSecondaryTeacher.id;
    row.due_date = "2026-10-01";
    renderPage();

    const prep = await screen.findByRole("region", { name: "Chuẩn bị tài liệu" });
    expect(within(prep).getByText("Thầy Minh")).toBeInTheDocument();
    expect(within(prep).getByText("01/10")).toBeInTheDocument();
  });
});
