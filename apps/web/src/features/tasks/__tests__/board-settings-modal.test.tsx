import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { useKanban } from "@/lib/kanban";
import { server } from "@/test/msw/server";
import { pickOption } from "@/test/pick-option";
import { signInAs, testPrimaryTeacher } from "@/test/utils";
import { useAuthStore } from "@/features/auth";

import { BoardSettingsModal } from "../components/board-settings-modal";
import { useTaskBoard } from "../hooks/use-task-board";
import { useTasksDataSource } from "../hooks/use-tasks-data-source";
import {
  columnOrderRequests,
  columnTodoId,
  resetTasksStore,
  tasksHandlers,
} from "./tasks-handlers";

beforeEach(() => {
  resetTasksStore();
  server.use(...tasksHandlers);
  signInAs(testPrimaryTeacher);
});

afterEach(() => {
  useAuthStore.getState().clearSession();
});

function Harness() {
  const { data } = useTaskBoard({ today: "2026-09-17" });
  const { createColumnMutation, updateColumnMutation, deleteColumnMutation, dataSource } =
    useTasksDataSource({ today: "2026-09-17" });
  const [announcement, setAnnouncement] = useState("");
  const board = data?.board ?? { columns: [], tasks: [] };
  const { tasksByColumn, reorderColumn } = useKanban({ board, dataSource });

  if (!data) return null;

  return (
    <>
      <div role="status" aria-live="polite">
        {announcement}
      </div>
      <BoardSettingsModal
        open
        onOpenChange={() => undefined}
        columns={data.board.columns}
        tasksByColumn={tasksByColumn}
        createColumnMutation={createColumnMutation}
        updateColumnMutation={updateColumnMutation}
        deleteColumnMutation={deleteColumnMutation}
        reorderColumn={reorderColumn}
        onAnnounce={setAnnouncement}
      />
    </>
  );
}

function renderHarness() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return render(
    <QueryClientProvider client={queryClient}>
      <Harness />
    </QueryClientProvider>,
  );
}

async function waitForColumnsLoaded() {
  await screen.findByLabelText("Tên cột Cần làm");
}

describe("BoardSettingsModal", () => {
  it("renames a column when the name input loses focus", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    const input = screen.getByLabelText("Tên cột Cần làm");
    await user.clear(input);
    await user.type(input, "Việc cần làm");
    await user.tab();

    await screen.findByLabelText("Tên cột Việc cần làm");
  });

  it("shows a duplicate-name validation error under the input instead of a toast", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    const input = screen.getByLabelText("Tên cột Cần làm");
    await user.clear(input);
    await user.type(input, "Hoàn thành");
    await user.tab();

    expect(await screen.findByText("Tên cột đã tồn tại")).toBeInTheDocument();
    // The rename is rejected, so the input reverts to the original name.
    expect(screen.getByLabelText("Tên cột Cần làm")).toBeInTheDocument();
  });

  it("creates a column and announces it, updating the column count", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    expect(screen.getByText("2/8 cột")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Tên cột mới"), "Chờ duyệt");
    await user.click(screen.getByRole("button", { name: "Thêm cột" }));

    await screen.findByLabelText("Tên cột Chờ duyệt");
    expect(screen.getByText("3/8 cột")).toBeInTheDocument();
    expect(await screen.findByRole("status")).toHaveTextContent("Đã tạo cột Chờ duyệt.");
  });

  it("disables the add-column input and button once the board has 8 columns", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    for (let index = 0; index < 6; index += 1) {
      const input = screen.getByLabelText("Tên cột mới");
      await user.type(input, `Cột ${index}`);
      await user.click(screen.getByRole("button", { name: "Thêm cột" }));
      await screen.findByLabelText(`Tên cột Cột ${index}`);
    }

    expect(screen.getByText("8/8 cột")).toBeInTheDocument();
    expect(screen.getByLabelText("Tên cột mới")).toBeDisabled();
    expect(screen.getByRole("button", { name: "Thêm cột" })).toBeDisabled();
  });

  it("reorders a column with the up/down buttons, sending the full order to the API", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    // "Cần làm" is first; move it down so "Hoàn thành" becomes first.
    await user.click(screen.getByRole("button", { name: "Chuyển Cần làm xuống sau" }));

    await waitFor(() => {
      expect(columnOrderRequests.at(-1)).toEqual(expect.arrayContaining([columnTodoId]));
    });
    const lastOrder = columnOrderRequests.at(-1);
    expect(lastOrder?.indexOf(columnTodoId)).toBe(1);
  });

  it("disables the up button for the first row and the down button for the last row", async () => {
    renderHarness();
    await waitForColumnsLoaded();

    expect(screen.getByRole("button", { name: "Chuyển Cần làm lên trước" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Chuyển Hoàn thành xuống sau" })).toBeDisabled();
  });

  it("requires choosing a destination column before confirming delete when the column has tasks", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    await user.click(screen.getByRole("button", { name: "Xoá cột Cần làm" }));

    const dialog = await screen.findByRole("dialog", { name: 'Xoá cột "Cần làm"?' });
    const confirmButton = within(dialog).getByRole("button", { name: "Xoá cột" });
    expect(confirmButton).toBeDisabled();

    await pickOption(user, within(dialog), "Chuyển việc sang cột", "Hoàn thành");

    expect(confirmButton).not.toBeDisabled();
    await user.click(confirmButton);

    await waitFor(() => {
      expect(screen.queryByLabelText("Tên cột Cần làm")).not.toBeInTheDocument();
    });
    expect(await screen.findByRole("status")).toHaveTextContent("Đã xoá cột Cần làm.");
  });

  it("disables delete once only one column remains", async () => {
    const user = userEvent.setup();
    renderHarness();
    await waitForColumnsLoaded();

    await user.click(screen.getByRole("button", { name: "Xoá cột Cần làm" }));
    const dialog = await screen.findByRole("dialog", { name: 'Xoá cột "Cần làm"?' });
    await pickOption(user, within(dialog), "Chuyển việc sang cột", "Hoàn thành");
    await user.click(within(dialog).getByRole("button", { name: "Xoá cột" }));

    await waitFor(() => {
      expect(screen.queryByLabelText("Tên cột Cần làm")).not.toBeInTheDocument();
    });

    expect(screen.getByRole("button", { name: "Xoá cột Hoàn thành" })).toBeDisabled();
  });
});
