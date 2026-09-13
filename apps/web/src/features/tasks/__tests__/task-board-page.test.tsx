import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { TaskBoardPage } from "../pages/task-board-page";
import { resetTasksStore, tasksHandlers } from "./tasks-handlers";

function memberCenterMe(permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
}

function renderBoardPage() {
  return renderWithProviders(<TaskBoardPage />, {
    route: "/tasks",
    path: "/tasks",
    extraRoutes: [{ path: "/", element: <div>Trang tổng quan</div> }],
  });
}

beforeEach(() => {
  mockViewport(1024);
  resetTasksStore();
  server.use(...tasksHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TaskBoardPage", () => {
  it("redirects a caller without tasks.list to the dashboard", async () => {
    server.use(memberCenterMe(["tasks.create"]));
    signInAs(testPrimaryTeacher);
    const { router } = renderBoardPage();

    await waitFor(() => expect(router.state.location.pathname).toBe("/"));
    expect(await screen.findByText("Trang tổng quan")).toBeInTheDocument();
  });

  it("renders the board with its columns and tasks for the owner", async () => {
    signInAs(testPrimaryTeacher);
    renderBoardPage();

    expect(await screen.findByRole("listbox", { name: "Cần làm" })).toBeInTheDocument();
    const doneColumn = screen.getByRole("listbox", { name: "Hoàn thành" });
    expect(doneColumn).toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(3);
    expect(screen.getByText("Soạn đề kiểm tra giữa kỳ")).toBeInTheDocument();
  });

  it("hides the scope switch and board settings button for a member limited to tasks.list", async () => {
    server.use(memberCenterMe(["tasks.list"]));
    signInAs(testPrimaryTeacher);
    renderBoardPage();

    await screen.findByRole("listbox", { name: "Cần làm" });
    expect(
      screen.queryByRole("radiogroup", { name: "Phạm vi bảng công việc" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Cấu hình cột" })).not.toBeInTheDocument();
  });

  it("shows the scope switch and settings button for a member with view_all and manage_board", async () => {
    server.use(memberCenterMe(["tasks.list", "tasks.view_all", "tasks.manage_board"]));
    signInAs(testPrimaryTeacher);
    renderBoardPage();

    await screen.findByRole("listbox", { name: "Cần làm" });
    expect(screen.getByRole("radiogroup", { name: "Phạm vi bảng công việc" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cấu hình cột" })).toBeInTheDocument();
  });

  it("creates a task through the column '+' button and the form modal", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    await user.click(screen.getByRole("button", { name: "Thêm việc vào Cần làm" }));
    await screen.findByRole("dialog", { name: "Tạo công việc" });

    await user.type(screen.getByLabelText("Tiêu đề"), "Việc mới cho tuần này");
    await user.click(screen.getByRole("button", { name: "Lưu" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(await screen.findByText("Việc mới cho tuần này")).toBeInTheDocument();
  });

  it("moves a task to another column through its move menu", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const todoColumn = screen.getByRole("listbox", { name: "Cần làm" });
    const card = within(todoColumn)
      .getByText("Soạn đề kiểm tra giữa kỳ")
      .closest('[role="option"]');
    expect(card).not.toBeNull();
    await user.click(within(card as HTMLElement).getByRole("button", { name: "Chuyển cột" }));
    await user.click(await screen.findByRole("menuitem", { name: "Hoàn thành" }));

    await waitFor(() => {
      const doneColumn = screen.getByRole("listbox", { name: "Hoàn thành" });
      expect(within(doneColumn).getByText("Soạn đề kiểm tra giữa kỳ")).toBeInTheDocument();
    });
    expect(
      within(screen.getByRole("listbox", { name: "Cần làm" })).queryByText(
        "Soạn đề kiểm tra giữa kỳ",
      ),
    ).not.toBeInTheDocument();
  });

  it("announces the move in Vietnamese in the aria-live region after a keyboard `[`/`]` move", async () => {
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    expect(card).not.toBeNull();
    fireEvent.keyDown(card as HTMLElement, { key: "]" });

    await waitFor(() =>
      expect(screen.getByRole("status")).toHaveTextContent("Đã chuyển việc sang cột Hoàn thành."),
    );
  });

  it("deletes a task from the form modal after confirming", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    await user.click(screen.getByText("Soạn đề kiểm tra giữa kỳ"));
    const dialog = await screen.findByRole("dialog", { name: "Chi tiết công việc" });
    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));

    const confirmDialog = await screen.findByRole("dialog", { name: "Xoá công việc này?" });
    await user.click(within(confirmDialog).getByRole("button", { name: "Xoá" }));

    await waitFor(() => {
      expect(screen.queryByText("Soạn đề kiểm tra giữa kỳ")).not.toBeInTheDocument();
    });
  });

  it("shows a read-only form with no delete button for a task the caller did not create", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    await user.click(screen.getByText("Kiểm tra học phí tháng 9"));
    const dialog = await screen.findByRole("dialog", { name: "Chi tiết công việc" });

    expect(within(dialog).getByLabelText("Tiêu đề")).toBeDisabled();
    expect(within(dialog).getByLabelText("Mô tả")).toBeDisabled();
    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Đóng" })).toBeInTheDocument();
  });
});
