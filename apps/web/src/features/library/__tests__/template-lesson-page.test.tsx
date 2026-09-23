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

import { TemplateLessonPage } from "../pages/template-lesson-page";
import {
  getLibraryStore,
  lessonDraftSoTuNhien,
  lessonPublishedSoTuNhien,
  libraryHandlers,
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
});
