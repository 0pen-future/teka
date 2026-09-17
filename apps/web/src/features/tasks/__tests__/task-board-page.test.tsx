import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { TaskBoardPage } from "../pages/task-board-page";
import { resetTasksStore, tasksHandlers } from "./tasks-handlers";

// The board tests only open the form; the editor itself (TipTap and
// ProseMirror, the feature's largest chunk) is covered by its own test
// files, so a plain textarea stands in for it here.
vi.mock("../components/task-description-editor", () => ({
  DESCRIPTION_MAX_CHARS: 2000,
  TaskDescriptionEditor: ({
    id,
    value,
    onChange,
    labelId,
  }: {
    id: string;
    value: string;
    onChange: (html: string) => void;
    labelId: string;
  }) => (
    <textarea
      id={id}
      aria-labelledby={labelId}
      value={value}
      onChange={(event) => onChange(event.target.value)}
    />
  ),
}));

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

    // dnd-kit mounts its own (silenced) live region, so select ours by text.
    await waitFor(() =>
      expect(screen.getByText("Đã chuyển việc sang cột Hoàn thành.")).toBeInTheDocument(),
    );
    expect(
      within(screen.getByRole("listbox", { name: "Hoàn thành" })).getByText(
        "Soạn đề kiểm tra giữa kỳ",
      ),
    ).toBeInTheDocument();
  });

  it("still announces a keyboard move after the page has announced a menu move", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    // A menu move fills the page's own live region first…
    const first = screen.getByText("Sắp lịch dạy bù").closest('[role="option"]');
    await user.click(within(first as HTMLElement).getByRole("button", { name: "Chuyển cột" }));
    await user.click(await screen.findByRole("menuitem", { name: "Hoàn thành" }));
    expect(
      await screen.findByText(/Đã chuyển "Sắp lịch dạy bù" sang cột Hoàn thành/),
    ).toBeInTheDocument();

    // …and the lib's `]` outcome must still be announced, not shadowed by it.
    const second = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    fireEvent.keyDown(second as HTMLElement, { key: "]" });
    expect(await screen.findByText("Đã chuyển việc sang cột Hoàn thành.")).toBeInTheDocument();
    expect(screen.getByText(/Đã chuyển "Sắp lịch dạy bù" sang cột Hoàn thành/)).toBeInTheDocument();
  });

  it("reloads the server order and toasts when the server rejects a move", async () => {
    const user = userEvent.setup();
    server.use(
      http.post(`${API_URL}/tasks/:id/move`, () =>
        HttpResponse.json(
          fail("VALIDATION_ERROR", "Dữ liệu không hợp lệ", {
            after_task_id: "phải là việc đang nằm trong cột đích",
          }),
          { status: 422 },
        ),
      ),
    );
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    await user.click(within(card as HTMLElement).getByRole("button", { name: "Chuyển cột" }));
    await user.click(await screen.findByRole("menuitem", { name: "Hoàn thành" }));

    // Sighted users get the server's field message as a danger toast; the
    // live region carries the generic copy for everyone else.
    expect(await screen.findByText("phải là việc đang nằm trong cột đích")).toBeInTheDocument();
    expect(screen.getByText("Không chuyển được việc.")).toBeInTheDocument();
    await waitFor(() =>
      expect(
        within(screen.getByRole("listbox", { name: "Cần làm" })).getByText(
          "Soạn đề kiểm tra giữa kỳ",
        ),
      ).toBeInTheDocument(),
    );
  });

  it("names the failed action in the toast when the server gives no reason", async () => {
    const user = userEvent.setup();
    server.use(
      http.post(`${API_URL}/tasks/:id/move`, () =>
        HttpResponse.json(fail("INTERNAL", "Lỗi hệ thống"), { status: 500 }),
      ),
    );
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    await user.click(within(card as HTMLElement).getByRole("button", { name: "Chuyển cột" }));
    await user.click(await screen.findByRole("menuitem", { name: "Hoàn thành" }));

    expect(
      await screen.findByText("Không chuyển được việc, vui lòng thử lại."),
    ).toBeInTheDocument();
  });

  it("keeps the lib's option semantics on cards instead of dnd-kit's button role", async () => {
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    const todoColumn = await screen.findByRole("listbox", { name: "Cần làm" });

    const options = within(todoColumn).getAllByRole("option");
    expect(options.length).toBeGreaterThan(0);
    for (const option of options) {
      expect(option).not.toHaveAttribute("aria-roledescription");
      expect(option).not.toHaveAttribute("aria-describedby");
      expect(option).not.toHaveAttribute("aria-pressed");
    }
    // Roving tabindex: exactly one card in the column is in the tab order.
    expect(options.filter((option) => option.tabIndex === 0)).toHaveLength(1);
  });

  it("marks a card as dragging once the mouse moves past the activation distance", async () => {
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    expect(card).not.toBeNull();
    fireEvent.mouseDown(card as HTMLElement, { button: 0, clientX: 10, clientY: 10 });
    fireEvent.mouseMove(document, { clientX: 10, clientY: 30 });

    await waitFor(() => expect(card).toHaveAttribute("data-dragging", "true"));
    expect(screen.getByRole("listbox", { name: "Cần làm" }).closest("[data-over]")).not.toBeNull();

    fireEvent.mouseUp(document, { clientX: 10, clientY: 30 });
    await waitFor(() => expect(card).not.toHaveAttribute("data-dragging"));
  });

  it("does not start a drag for a caller without tasks.edit", async () => {
    server.use(memberCenterMe(["tasks.list"]));
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    const card = screen.getByText("Soạn đề kiểm tra giữa kỳ").closest('[role="option"]');
    expect(card).not.toBeNull();
    fireEvent.mouseDown(card as HTMLElement, { button: 0, clientX: 10, clientY: 10 });
    fireEvent.mouseMove(document, { clientX: 10, clientY: 40 });
    fireEvent.mouseUp(document, { clientX: 10, clientY: 40 });

    expect(card).not.toHaveAttribute("data-dragging");
    expect(within(card as HTMLElement).getByRole("button", { name: "Chuyển cột" })).toBeDisabled();
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
    // Read-only shows the description as rendered HTML, not an editor.
    expect(within(dialog).queryByRole("textbox", { name: "Mô tả" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("link", { name: "bảng học phí" })).toHaveAttribute(
      "href",
      "https://teka.vn/hoc-phi",
    );
    expect(dialog.querySelector("script")).toBeNull();
    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Đóng" })).toBeInTheDocument();
  });
});
