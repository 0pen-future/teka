import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { PrepBoardPage } from "../pages/prep-board-page";
import {
  getLibraryStore,
  lessonDraftSoTuNhien,
  libraryHandlers,
  resetLibraryStore,
  templateToan6,
  versionToan6Draft,
  versionToan6Published,
} from "./library-handlers";

function renderPage(versionId = versionToan6Draft.id) {
  return renderWithProviders(<PrepBoardPage />, {
    route: `/prep/${versionId}/board`,
    path: "/prep/:vid/board",
    extraRoutes: [
      { path: "/library/templates/:id/lessons/:lessonId", element: <div>lesson-stub</div> },
    ],
  });
}

function memberWith(...permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function cardOf(text: string) {
  const card = screen.getByText(text).closest('[role="option"]');
  expect(card).not.toBeNull();
  return card as HTMLElement;
}

beforeEach(() => {
  resetLibraryStore();
  server.use(...libraryHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("PrepBoardPage", () => {
  it("renders the four status columns with the draft's lessons as cards", async () => {
    renderPage();

    const todo = await screen.findByRole("listbox", { name: "Cần làm" });
    expect(screen.getByRole("listbox", { name: "Đang làm" })).toBeInTheDocument();
    expect(screen.getByRole("listbox", { name: "Chờ duyệt" })).toBeInTheDocument();
    expect(screen.getByRole("listbox", { name: "Hoàn thành" })).toBeInTheDocument();

    const card = within(todo).getByRole("option", { name: /Buổi 1 · Số tự nhiên/ });
    expect(within(card).getByText("1/2 việc")).toBeInTheDocument();
    expect(within(card).getByText("Chưa phân công")).toBeInTheDocument();
    expect(
      within(screen.getByRole("listbox", { name: "Đang làm" })).getByText("Buổi 2 · Phân số"),
    ).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: /Toán 6 cơ bản/ })).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Phân công" })).toHaveAttribute(
      "href",
      `/prep/${versionToan6Draft.id}/assign`,
    );
  });

  it("moves a lesson through the card menu and announces it", async () => {
    const user = userEvent.setup();
    renderPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = cardOf("Buổi 1 · Số tự nhiên");
    await user.click(within(card).getByRole("button", { name: "Thao tác" }));
    await user.click(await screen.findByRole("menuitem", { name: "Chờ duyệt" }));

    await waitFor(() => {
      expect(
        within(screen.getByRole("listbox", { name: "Chờ duyệt" })).getByText(
          "Buổi 1 · Số tự nhiên",
        ),
      ).toBeInTheDocument();
    });
    expect(screen.getByText('Đã chuyển "Số tự nhiên" sang cột Chờ duyệt.')).toBeInTheDocument();
    await waitFor(() => {
      const row = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id);
      expect(row?.prep_status).toBe("review");
    });
  });

  it("moves a lesson to the next column with `]` and announces it in Vietnamese", async () => {
    renderPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    fireEvent.keyDown(cardOf("Buổi 1 · Số tự nhiên"), { key: "]" });

    await waitFor(() =>
      expect(screen.getByText("Đã chuyển buổi sang cột Đang làm.")).toBeInTheDocument(),
    );
    expect(
      within(screen.getByRole("listbox", { name: "Đang làm" })).getByText("Buổi 1 · Số tự nhiên"),
    ).toBeInTheDocument();
  });

  it("opens the lesson page from the card menu", async () => {
    const user = userEvent.setup();
    const { router } = renderPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = cardOf("Buổi 1 · Số tự nhiên");
    await user.click(within(card).getByRole("button", { name: "Thao tác" }));
    await user.click(await screen.findByRole("menuitem", { name: "Mở chi tiết" }));

    await waitFor(() =>
      expect(router.state.location.pathname).toBe(
        `/library/templates/${templateToan6.id}/lessons/${lessonDraftSoTuNhien.id}`,
      ),
    );
  });

  it("keeps a published version read-only: no move targets, a locked notice", async () => {
    const user = userEvent.setup();
    renderPage(versionToan6Published.id);

    const todo = await screen.findByRole("listbox", { name: "Cần làm" });
    expect(screen.getByText(/đã phát hành, trạng thái chuẩn bị được khoá/)).toBeInTheDocument();
    const card = within(todo).getByRole("option", { name: /Buổi 1 · Số tự nhiên/ });
    await user.click(within(card).getByRole("button", { name: "Thao tác" }));
    expect(await screen.findByRole("menuitem", { name: "Mở chi tiết" })).toBeInTheDocument();
    expect(screen.queryByRole("menuitem", { name: "Đang làm" })).not.toBeInTheDocument();
  });

  it("hides moving and the assign link from a member holding only library.read", async () => {
    server.use(memberWith("library.read"));
    renderPage();

    const todo = await screen.findByRole("listbox", { name: "Cần làm" });
    expect(screen.getByText("Bạn không có quyền đổi trạng thái chuẩn bị.")).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: "Phân công" })).not.toBeInTheDocument();

    fireEvent.keyDown(within(todo).getByRole("option", { name: /Buổi 1/ }), { key: "]" });
    await waitFor(() => expect(screen.getByText("Không chuyển được buổi.")).toBeInTheDocument());
    expect(within(todo).getByText("Buổi 1 · Số tự nhiên")).toBeInTheDocument();
  });
});
