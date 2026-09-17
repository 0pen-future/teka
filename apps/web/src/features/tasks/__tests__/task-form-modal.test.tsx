import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useAuthStore } from "@/features/auth";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";
import { mockViewport } from "@/test/viewport";

import { TaskBoardPage } from "../pages/task-board-page";
import {
  ownTaskId,
  resetTasksStore,
  seedTaskDescription,
  taskWriteRequests,
  tasksHandlers,
} from "./tasks-handlers";

function renderBoardPage() {
  return renderWithProviders(<TaskBoardPage />, { route: "/tasks", path: "/tasks" });
}

async function openTask(user: ReturnType<typeof userEvent.setup>, title: string) {
  signInAs(testPrimaryTeacher);
  renderBoardPage();
  await screen.findByRole("listbox", { name: "Cần làm" });
  await user.click(screen.getByText(title));
  const dialog = await screen.findByRole("dialog", { name: "Chi tiết công việc" });
  const textbox = await within(dialog).findByRole("textbox", { name: "Mô tả" });
  return { dialog, textbox };
}

beforeEach(() => {
  mockViewport(1024);
  resetTasksStore();
  server.use(...tasksHandlers);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

describe("TaskFormModal description", () => {
  it("loads the stored HTML into the editor and sends the edited HTML back", async () => {
    const user = userEvent.setup();
    const { dialog, textbox } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    expect(textbox.querySelector("strong")).toHaveTextContent("3 phần");
    expect(textbox.querySelectorAll("li")).toHaveLength(2);

    await user.click(textbox);
    await user.keyboard("{Control>}a{/Control}Đề mới");
    await user.keyboard("{Control>}a{/Control}");
    await user.click(within(dialog).getByRole("button", { name: "Đậm" }));
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(taskWriteRequests.at(-1)?.description).toBe("<p><strong>Đề mới</strong></p>");
  });

  it("wraps a legacy plain-text description as a paragraph with line breaks before editing", async () => {
    const user = userEvent.setup();
    const { textbox } = await openTask(user, "Sắp lịch dạy bù");

    expect(textbox.innerHTML).toBe("<p>Hỏi lớp 6A &amp; 7B<br>báo lại trước thứ 6</p>");
  });

  it("sends an empty description when the editor is cleared", async () => {
    const user = userEvent.setup();
    const { dialog, textbox } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.click(textbox);
    await user.keyboard("{Control>}a{/Control}{Backspace}");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(taskWriteRequests.at(-1)?.description).toBe("");
  });

  it("rejects more than 2000 characters of text with a field error wired to the textbox", async () => {
    const user = userEvent.setup();
    // Typing 2001 keys through ProseMirror is too slow under jsdom; the
    // stored row carries the overlong text instead.
    seedTaskDescription(ownTaskId, `<p>${"a".repeat(2001)}</p>`);
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    expect(within(dialog).getByText(/\/2000$/)).toHaveTextContent("2001/2000");

    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    const error = await within(dialog).findByText("Mô tả tối đa 2000 ký tự");
    expect(error).toHaveAttribute("id", "task-description-error");
    await waitFor(() =>
      expect(within(dialog).getByRole("textbox", { name: "Mô tả" })).toHaveAttribute(
        "aria-describedby",
        "task-description-error",
      ),
    );
    expect(within(dialog).getByRole("textbox", { name: "Mô tả" })).toHaveAttribute(
      "aria-invalid",
      "true",
    );
    expect(taskWriteRequests).toHaveLength(0);
  });

  it("shows the description as sanitized HTML on the card and in the read-only modal", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    // Card preview is text only, block boundaries become spaces.
    expect(screen.getByText("Đề gồm 3 phần: Trắc nghiệm Tự luận")).toHaveClass("line-clamp-2");
    expect(screen.getByText("Hỏi lớp 6A & 7B báo lại trước thứ 6")).toBeInTheDocument();

    await user.click(screen.getByText("Kiểm tra học phí tháng 9"));
    const dialog = await screen.findByRole("dialog", { name: "Chi tiết công việc" });
    const link = within(dialog).getByRole("link", { name: "bảng học phí" });
    expect(link).toHaveAttribute("rel", "nofollow noreferrer noopener");
    expect(link).toHaveAttribute("target", "_blank");
    expect(dialog.querySelector("script")).toBeNull();
  });

  it("creates a task with an empty description when nothing is typed", async () => {
    const user = userEvent.setup();
    signInAs(testPrimaryTeacher);
    renderBoardPage();
    await screen.findByRole("listbox", { name: "Cần làm" });

    await user.click(screen.getByRole("button", { name: "Thêm việc vào Cần làm" }));
    const dialog = await screen.findByRole("dialog", { name: "Tạo công việc" });
    await within(dialog).findByRole("textbox", { name: "Mô tả" });
    await user.type(within(dialog).getByLabelText("Tiêu đề"), "Việc không mô tả");
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(taskWriteRequests.at(-1)).toMatchObject({ title: "Việc không mô tả", description: "" });
  });
});
