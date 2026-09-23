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

import { PrepAssignPage } from "../pages/prep-assign-page";
import {
  getLibraryStore,
  lessonDraftPhanSo,
  lessonDraftSoTuNhien,
  libraryHandlers,
  resetLibraryStore,
  versionToan6Draft,
} from "./library-handlers";

function renderPage(versionId = versionToan6Draft.id) {
  return renderWithProviders(<PrepAssignPage />, {
    route: `/prep/${versionId}/assign`,
    path: "/prep/:vid/assign",
    extraRoutes: [{ path: "/prep/:vid/board", element: <div>board-stub</div> }],
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

describe("PrepAssignPage", () => {
  it("assigns a member and a due date per lesson with one full-replace PATCH", async () => {
    const user = userEvent.setup();
    renderPage();

    const row = await screen.findByRole("row", { name: /Buổi 1/ });
    expect(within(row).getByText("Số tự nhiên")).toBeInTheDocument();
    expect(within(row).getByText("1/2 việc")).toBeInTheDocument();

    await user.click(within(row).getByRole("combobox", { name: /Người phụ trách/ }));
    await user.click(await screen.findByRole("option", { name: /Thầy Minh/ }));
    await waitFor(() => {
      const lesson = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id);
      expect(lesson?.assignee_id).toBe(testSecondaryTeacher.id);
    });
    expect(await screen.findByText("Đã lưu phân công")).toBeInTheDocument();

    const due = within(row).getByLabelText("Hạn hoàn thành");
    await user.clear(due);
    await user.type(due, "2026-10-01");
    await waitFor(() => {
      const lesson = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id);
      expect(lesson?.due_date).toBe("2026-10-01");
      // The assignment endpoint replaces both fields, so the assignee must be re-sent.
      expect(lesson?.assignee_id).toBe(testSecondaryTeacher.id);
    });

    // The other lesson keeps its own state.
    const phanSo = getLibraryStore().lessons.find((l) => l.id === lessonDraftPhanSo.id);
    expect(phanSo?.assignee_id).toBeNull();
    expect(screen.getByRole("row", { name: /Buổi 2/ })).toHaveTextContent("Đang làm");
  });

  it("clears the assignee by picking the empty option", async () => {
    const user = userEvent.setup();
    getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id)!.assignee_id =
      testSecondaryTeacher.id;
    renderPage();

    const row = await screen.findByRole("row", { name: /Buổi 1/ });
    await user.click(within(row).getByRole("combobox", { name: /Người phụ trách/ }));
    await user.click(await screen.findByRole("option", { name: "Chưa phân công" }));
    await waitFor(() => {
      const lesson = getLibraryStore().lessons.find((l) => l.id === lessonDraftSoTuNhien.id);
      expect(lesson?.assignee_id).toBeNull();
    });
  });

  it("sends a member without prep.assign to the board", async () => {
    server.use(memberWith("library.read", "library.edit"));
    renderPage();

    expect(await screen.findByText("board-stub")).toBeInTheDocument();
  });
});
