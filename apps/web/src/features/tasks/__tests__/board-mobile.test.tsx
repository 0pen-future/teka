import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { asColumnId, asTaskId, type ColumnId } from "@/lib/kanban";

import { BoardMobile } from "../components/board-mobile";
import type { AppTask } from "../hooks/use-tasks-data-source";

function makeColumn(id: string, name: string, order: number) {
  return { id: asColumnId(id), name, order, isDone: false, color: "none" as const };
}

function makeTask(id: string, columnId: string, title: string): AppTask {
  return {
    id: asTaskId(id),
    columnId: asColumnId(columnId),
    position: 0,
    title,
    description: "",
    priority: "none",
    dueOn: null,
    assigneeId: null,
    createdBy: "creator-1",
    completedAt: null,
    updatedAt: "2026-09-13T00:00:00Z",
  };
}

const noop = {
  getColumnProps: () => ({}),
  getTaskProps: () => ({}),
  dnd: { contextProps: {}, activeTask: null, overColumnId: null, reducedMotion: false },
};

function renderBoard(
  columns: ReturnType<typeof makeColumn>[],
  tasksByColumn: Map<ColumnId, AppTask[]>,
) {
  return render(
    <BoardMobile
      columns={columns}
      tasksByColumn={tasksByColumn}
      assigneeNameFor={() => null}
      canCreate={false}
      canMove={false}
      filtering={false}
      onOpenTask={vi.fn()}
      onCreateTask={vi.fn()}
      onMoveTask={vi.fn()}
      onQuickDone={vi.fn()}
      {...noop}
    />,
  );
}

describe("BoardMobile", () => {
  it("shows a segmented switcher with per-column counts at 4 columns or fewer, one panel at a time", () => {
    const columns = [makeColumn("c1", "Cần làm", 1), makeColumn("c2", "Đang làm", 2)];
    renderBoard(
      columns,
      new Map([
        [asColumnId("c1"), [makeTask("t1", "c1", "Soạn giáo án")]],
        [asColumnId("c2"), []],
      ]),
    );

    expect(screen.getByRole("tablist", { name: "Chọn cột" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Cần làm (1)" })).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "Đang làm (0)" })).toBeInTheDocument();
    // Only the active (first) column's tasks render.
    expect(screen.getByText("Soạn giáo án")).toBeInTheDocument();
  });

  it("switches the active tab and shows that column's tasks", async () => {
    const user = userEvent.setup();
    const columns = [makeColumn("c1", "Cần làm", 1), makeColumn("c2", "Đang làm", 2)];
    renderBoard(
      columns,
      new Map([
        [asColumnId("c1"), []],
        [asColumnId("c2"), [makeTask("t2", "c2", "Gọi phụ huynh")]],
      ]),
    );

    expect(screen.queryByText("Gọi phụ huynh")).not.toBeInTheDocument();
    await user.click(screen.getByRole("tab", { name: "Đang làm (1)" }));
    expect(screen.getByText("Gọi phụ huynh")).toBeInTheDocument();
  });

  it("uses a select instead of a segmented strip once there are more than 4 columns", async () => {
    const user = userEvent.setup();
    const columns = [1, 2, 3, 4, 5].map((n) => makeColumn(`c${n}`, `Cột ${n}`, n));
    const tasksByColumn = new Map(
      columns.map((column) => [
        column.id,
        column.name === "Cột 2" ? [makeTask("t2", "c2", "Việc cột 2")] : [],
      ]),
    );
    renderBoard(columns, tasksByColumn);

    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    const select = screen.getByRole("combobox", { name: "Chọn cột" });
    expect(screen.queryByText("Việc cột 2")).not.toBeInTheDocument();

    await user.click(select);
    await user.click(await screen.findByRole("option", { name: /Cột 2/ }));

    expect(await screen.findByText("Việc cột 2")).toBeInTheDocument();
  });

  it("falls back to the first column when the previously active column disappears", () => {
    const columns = [makeColumn("c1", "Cần làm", 1)];
    const { rerender } = renderBoard(columns, new Map([[asColumnId("c1"), []]]));
    expect(screen.getByRole("tab", { name: "Cần làm (0)" })).toBeInTheDocument();

    rerender(
      <BoardMobile
        columns={[makeColumn("c2", "Đang làm", 1)]}
        tasksByColumn={new Map([[asColumnId("c2"), []]])}
        assigneeNameFor={() => null}
        canCreate={false}
        canMove={false}
        filtering={false}
        onOpenTask={vi.fn()}
        onCreateTask={vi.fn()}
        onMoveTask={vi.fn()}
        onQuickDone={vi.fn()}
        {...noop}
      />,
    );

    expect(screen.getByRole("tab", { name: "Đang làm (0)" })).toBeInTheDocument();
  });

  it("renders a placeholder instead of a switcher when the board has no columns", () => {
    renderBoard([], new Map());
    expect(screen.getByText("Chưa có cột nào.")).toBeInTheDocument();
    expect(screen.queryByRole("tablist")).not.toBeInTheDocument();
    expect(screen.queryByRole("combobox")).not.toBeInTheDocument();
  });
});
