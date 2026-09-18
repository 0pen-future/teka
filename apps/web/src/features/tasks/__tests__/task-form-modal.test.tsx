import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { useAuthStore } from "@/features/auth";
import { API_URL, ok } from "@/test/msw/handlers";
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

/** Overrides `/centers/me` for a limited member instead of the default owner mock. */
function memberCenterMe(permissions: string[]) {
  return http.get(`${API_URL}/centers/me`, () =>
    HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh", permissions })),
  );
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

/** For the read-only branches, where the body renders sanitized HTML instead of an editor. */
async function openReadOnlyTask(user: ReturnType<typeof userEvent.setup>, title: string) {
  signInAs(testPrimaryTeacher);
  renderBoardPage();
  await screen.findByRole("listbox", { name: "Cần làm" });
  await user.click(screen.getByText(title));
  const dialog = await screen.findByRole("dialog", { name: "Chi tiết công việc" });
  return { dialog };
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

describe("TaskFormModal footer, context row and delete flow", () => {
  it("shows the current column and creator as a context row under the title", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    const eyebrow = within(dialog).getByText("Cô Lan tạo 10/09/2026").parentElement;
    expect(eyebrow).toHaveTextContent(/^Cần làm\s*Cô Lan tạo 10\/09\/2026$/);
  });

  it("orders the footer as Xoá, Huỷ, Lưu", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    const footerLabels = within(dialog)
      .getAllByRole("button")
      .map((button) => button.textContent)
      .filter((label): label is string => label === "Xoá" || label === "Huỷ" || label === "Lưu");
    expect(footerLabels).toEqual(["Xoá", "Huỷ", "Lưu"]);
  });

  it("hides Xoá for a caller without tasks.delete", async () => {
    const user = userEvent.setup();
    server.use(memberCenterMe(["tasks.list", "tasks.create", "tasks.edit"]));
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();
  });

  it('shows a "Chưa lưu" badge once the form is dirty, and guards a dirty Huỷ behind a confirm dialog', async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    expect(within(dialog).queryByText("Chưa lưu")).not.toBeInTheDocument();

    await user.type(within(dialog).getByLabelText("Tiêu đề"), "!");
    expect(within(dialog).getByText("Chưa lưu")).toBeInTheDocument();

    await user.click(within(dialog).getByRole("button", { name: "Huỷ" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Bỏ thay đổi chưa lưu?" });
    await user.click(within(confirm).getByRole("button", { name: "Tiếp tục sửa" }));
    expect(screen.queryByRole("alertdialog")).not.toBeInTheDocument();

    expect(screen.getByRole("dialog", { name: "Chi tiết công việc" })).toBeInTheDocument();
    expect(within(dialog).getByLabelText("Tiêu đề")).toHaveValue("Soạn đề kiểm tra giữa kỳ!");
  });

  it("takes the form and footer out of the tab order while a confirm panel covers them", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    const save = within(dialog).getByRole("button", { name: "Lưu" });
    const title = within(dialog).getByLabelText("Tiêu đề");
    expect(save.closest("[inert]")).toBeNull();
    expect(title.closest("[inert]")).toBeNull();

    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));
    const confirm = await screen.findByRole("alertdialog", { name: "Xoá công việc này?" });
    expect(within(confirm).getByRole("button", { name: "Giữ lại" })).toHaveFocus();
    expect(save.closest("[inert]")).not.toBeNull();
    expect(title.closest("[inert]")).not.toBeNull();
    expect(confirm.closest("[inert]")).toBeNull();

    await user.click(within(confirm).getByRole("button", { name: "Giữ lại" }));
    expect(save.closest("[inert]")).toBeNull();
    expect(title.closest("[inert]")).toBeNull();
  });

  it("trims a pasted title at 200 characters and says so in a toast", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    const title = within(dialog).getByLabelText("Tiêu đề");

    await user.clear(title);
    await user.paste("x".repeat(230));

    expect(title).toHaveValue("x".repeat(200));
    expect(await screen.findByText("Tiêu đề tối đa 200 ký tự")).toBeInTheDocument();
  });

  it("closes without asking on Escape once a confirmed discard is chosen", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    await user.type(within(dialog).getByLabelText("Tiêu đề"), "!");

    await user.keyboard("{Escape}");
    const confirm = await screen.findByRole("alertdialog", { name: "Bỏ thay đổi chưa lưu?" });
    await user.click(within(confirm).getByRole("button", { name: "Bỏ thay đổi" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("closes immediately on Escape when the form is not dirty", async () => {
    const user = userEvent.setup();
    await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.keyboard("{Escape}");

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  });

  it("moves the task to the selected column on save", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.click(within(dialog).getByLabelText("Cột"));
    await user.click(await screen.findByRole("option", { name: "Hoàn thành" }));
    await user.click(within(dialog).getByRole("button", { name: "Lưu" }));

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(
      within(screen.getByRole("listbox", { name: "Hoàn thành" })).getByText(
        "Soạn đề kiểm tra giữa kỳ",
      ),
    ).toBeInTheDocument();
    expect(
      within(screen.getByRole("listbox", { name: "Cần làm" })).queryByText(
        "Soạn đề kiểm tra giữa kỳ",
      ),
    ).not.toBeInTheDocument();
  });

  it("offers an undo toast after deleting, and Hoàn tác brings the task back", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.click(within(dialog).getByRole("button", { name: "Xoá" }));
    const confirmDialog = await screen.findByRole("alertdialog", { name: "Xoá công việc này?" });
    await user.click(within(confirmDialog).getByRole("button", { name: "Xoá" }));

    await waitFor(() =>
      expect(screen.queryByText("Soạn đề kiểm tra giữa kỳ")).not.toBeInTheDocument(),
    );
    expect(await screen.findByText('Đã xoá "Soạn đề kiểm tra giữa kỳ"')).toBeInTheDocument();
    const undoButton = screen.getByRole("button", { name: "Hoàn tác" });

    await user.click(undoButton);

    expect(await screen.findByText("Soạn đề kiểm tra giữa kỳ")).toBeInTheDocument();
  });
});

describe("TaskFormModal quick due chips", () => {
  beforeEach(() => {
    // Freeze only Date (setTimeout stays real for msw/userEvent): the quick
    // chips derive their values from "today".
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-09-18T10:00:00"));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("sets Hạn to today's local date from the Hôm nay chip", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.click(within(dialog).getByRole("button", { name: "Hôm nay" }));

    expect(within(dialog).getByLabelText("Hạn")).toHaveValue("2026-09-18");
  });

  it("renders the chips as plain actions with no pressed state, even when Hạn is empty", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");
    await user.clear(within(dialog).getByLabelText("Hạn"));

    for (const name of ["Hôm nay", "Ngày mai", "Thứ 2 tới", "Bỏ hạn"]) {
      expect(within(dialog).getByRole("button", { name })).not.toHaveAttribute("aria-pressed");
    }
  });

  it("sets Hạn to next Monday from the Thứ 2 tới chip", async () => {
    const user = userEvent.setup();
    const { dialog } = await openTask(user, "Soạn đề kiểm tra giữa kỳ");

    await user.click(within(dialog).getByRole("button", { name: "Thứ 2 tới" }));

    expect(within(dialog).getByLabelText("Hạn")).toHaveValue("2026-09-21");
  });
});

describe("TaskFormModal read-only branches", () => {
  it("lets an assignee with tasks.edit move the task without writing its content", async () => {
    const user = userEvent.setup();
    const { dialog } = await openReadOnlyTask(user, "Kiểm tra học phí tháng 9");

    expect(within(dialog).getByLabelText("Tiêu đề")).toBeDisabled();
    expect(within(dialog).getByText("Bạn chỉ có thể đổi cột của việc này.")).toBeInTheDocument();
    const columnTrigger = within(dialog).getByLabelText("Cột");
    expect(columnTrigger).not.toBeDisabled();
    const saveButton = within(dialog).getByRole("button", { name: "Lưu" });
    expect(saveButton).toBeDisabled();
    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();

    await user.click(columnTrigger);
    await user.click(await screen.findByRole("option", { name: "Hoàn thành" }));
    expect(saveButton).toBeEnabled();

    await user.click(saveButton);

    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    // The server's `CanWriteTask` would 403 an assignee's content edit, so
    // the move must be the only request this branch sends.
    expect(taskWriteRequests).toHaveLength(0);
    expect(
      within(screen.getByRole("listbox", { name: "Hoàn thành" })).getByText(
        "Kiểm tra học phí tháng 9",
      ),
    ).toBeInTheDocument();
  });

  it("shows a view-only footer with a disabled Cột for a caller without tasks.edit", async () => {
    const user = userEvent.setup();
    server.use(memberCenterMe(["tasks.list"]));
    const { dialog } = await openReadOnlyTask(user, "Kiểm tra học phí tháng 9");

    expect(within(dialog).getByLabelText("Cột")).toBeDisabled();
    expect(within(dialog).getByText("Bạn chỉ có thể xem việc này.")).toBeInTheDocument();
    expect(within(dialog).getByRole("button", { name: "Đóng" })).toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Lưu" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Huỷ" })).not.toBeInTheDocument();
    expect(within(dialog).queryByRole("button", { name: "Xoá" })).not.toBeInTheDocument();
  });
});
