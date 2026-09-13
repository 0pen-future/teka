import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import type { KanbanDataSource } from "../data-source";
import { asColumnId, asTaskId, type KanbanBoard, type KanbanTask } from "../types";
import { useKanbanKeyboard } from "../use-kanban-keyboard";

const colA = { id: asColumnId("col-a"), name: "To do", order: 0, isDone: false };
const colB = { id: asColumnId("col-b"), name: "Doing", order: 1, isDone: false };

function makeBoard(): KanbanBoard<KanbanTask> {
  return {
    columns: [colA, colB],
    tasks: [
      { id: asTaskId("a1"), columnId: colA.id, position: 0 },
      { id: asTaskId("a2"), columnId: colA.id, position: 1 },
      { id: asTaskId("b1"), columnId: colB.id, position: 0 },
    ],
  };
}

function Board({
  board,
  moveTask,
}: {
  board: KanbanBoard<KanbanTask>;
  moveTask: (
    taskId: ReturnType<typeof asTaskId>,
    columnId: ReturnType<typeof asColumnId>,
    position: number,
  ) => Promise<KanbanTask>;
}) {
  const dataSource: Pick<KanbanDataSource<KanbanTask>, "moveTask"> = { moveTask };
  const keyboard = useKanbanKeyboard({ board, dataSource });
  return (
    <div>
      <div data-testid="announcement">{keyboard.announcement}</div>
      {board.columns.map((column) => (
        <div
          key={column.id}
          data-testid={`column-${column.id}`}
          {...keyboard.getColumnProps(column.id)}
        >
          {board.tasks
            .filter((task) => task.columnId === column.id)
            .map((task) => (
              <div
                key={task.id}
                data-testid={`task-${task.id}`}
                {...keyboard.getTaskProps(task.id)}
              >
                {task.id}
              </div>
            ))}
        </div>
      ))}
    </div>
  );
}

describe("useKanbanKeyboard", () => {
  it("gives exactly one task per column a tabIndex of 0 by default", () => {
    render(<Board board={makeBoard()} moveTask={vi.fn()} />);
    expect(screen.getByTestId("task-a1").tabIndex).toBe(0);
    expect(screen.getByTestId("task-a2").tabIndex).toBe(-1);
    expect(screen.getByTestId("task-b1").tabIndex).toBe(0);
  });

  it("ArrowDown/ArrowUp move the roving tabindex within a column only", () => {
    render(<Board board={makeBoard()} moveTask={vi.fn()} />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "ArrowDown" });
    expect(screen.getByTestId("task-a1").tabIndex).toBe(-1);
    expect(screen.getByTestId("task-a2").tabIndex).toBe(0);
    expect(screen.getByTestId("task-b1").tabIndex).toBe(0); // unaffected

    fireEvent.keyDown(screen.getByTestId("task-a2"), { key: "ArrowUp" });
    expect(screen.getByTestId("task-a1").tabIndex).toBe(0);
    expect(screen.getByTestId("task-a2").tabIndex).toBe(-1);
  });

  it("Home and End jump to the column's first and last task", () => {
    render(<Board board={makeBoard()} moveTask={vi.fn()} />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "End" });
    expect(screen.getByTestId("task-a2").tabIndex).toBe(0);

    fireEvent.keyDown(screen.getByTestId("task-a2"), { key: "Home" });
    expect(screen.getByTestId("task-a1").tabIndex).toBe(0);
  });

  it("`]` calls dataSource.moveTask with the adjacent column to the right", async () => {
    const moveTask = vi
      .fn()
      .mockResolvedValue({ id: asTaskId("a1"), columnId: colB.id, position: 1 });
    render(<Board board={makeBoard()} moveTask={moveTask} />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "]" });

    await waitFor(() => expect(moveTask).toHaveBeenCalledWith(asTaskId("a1"), colB.id, 1));
    await waitFor(() =>
      expect(screen.getByTestId("announcement")).toHaveTextContent(`Moved task to ${colB.name}.`),
    );
  });

  it("uses the caller's `messages` option instead of the English defaults", async () => {
    const moveTask = vi
      .fn()
      .mockResolvedValue({ id: asTaskId("a1"), columnId: colB.id, position: 1 });

    function BoardWithMessages() {
      const board = makeBoard();
      const dataSource: Pick<KanbanDataSource<KanbanTask>, "moveTask"> = { moveTask };
      const keyboard = useKanbanKeyboard({
        board,
        dataSource,
        messages: {
          moved: (columnName) => `Đã chuyển việc sang cột ${columnName}.`,
          moveFailed: "Không chuyển được việc.",
        },
      });
      return (
        <div>
          <div data-testid="announcement">{keyboard.announcement}</div>
          <div data-testid="column-a" {...keyboard.getColumnProps(colA.id)}>
            {board.tasks
              .filter((task) => task.columnId === colA.id)
              .map((task) => (
                <div
                  key={task.id}
                  data-testid={`task-${task.id}`}
                  {...keyboard.getTaskProps(task.id)}
                />
              ))}
          </div>
        </div>
      );
    }
    render(<BoardWithMessages />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "]" });

    await waitFor(() =>
      expect(screen.getByTestId("announcement")).toHaveTextContent(
        `Đã chuyển việc sang cột ${colB.name}.`,
      ),
    );
  });

  it("`[` at the first column is a no-op (no adjacent column to the left)", () => {
    const moveTask = vi.fn();
    render(<Board board={makeBoard()} moveTask={moveTask} />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "[" });

    expect(moveTask).not.toHaveBeenCalled();
  });

  it("`]` at the last column is a no-op (no adjacent column to the right)", () => {
    const moveTask = vi.fn();
    render(<Board board={makeBoard()} moveTask={moveTask} />);

    fireEvent.keyDown(screen.getByTestId("task-b1"), { key: "]" });

    expect(moveTask).not.toHaveBeenCalled();
  });

  it("keeps focus on the moved task once the app feeds back the updated board", async () => {
    const moveTask = vi
      .fn()
      .mockResolvedValue({ id: asTaskId("a1"), columnId: colB.id, position: 1 });
    const initialBoard = makeBoard();
    const { rerender } = render(<Board board={initialBoard} moveTask={moveTask} />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "]" });
    await waitFor(() => expect(moveTask).toHaveBeenCalled());

    const updatedBoard: KanbanBoard<KanbanTask> = {
      columns: [colA, colB],
      tasks: [
        { id: asTaskId("a2"), columnId: colA.id, position: 0 },
        { id: asTaskId("b1"), columnId: colB.id, position: 0 },
        { id: asTaskId("a1"), columnId: colB.id, position: 1 },
      ],
    };
    rerender(<Board board={updatedBoard} moveTask={moveTask} />);

    expect(screen.getByTestId("task-a1")).toHaveFocus();
    expect(screen.getByTestId("task-a1").tabIndex).toBe(0);
    expect(screen.getByTestId("task-b1").tabIndex).toBe(-1);
  });

  it("merges the app's extra onKeyDown before the internal handler, respecting defaultPrevented", () => {
    function BoardWithExtra() {
      const board = makeBoard();
      const keyboard = useKanbanKeyboard({ board, dataSource: { moveTask: vi.fn() } });
      return (
        <div
          data-testid="col"
          {...keyboard.getColumnProps(colA.id, {
            onKeyDown: (event) => {
              if (event.key === "ArrowDown") {
                event.preventDefault();
              }
            },
          })}
        >
          {board.tasks
            .filter((task) => task.columnId === colA.id)
            .map((task) => (
              <div
                key={task.id}
                data-testid={`task-${task.id}`}
                {...keyboard.getTaskProps(task.id)}
              >
                {task.id}
              </div>
            ))}
        </div>
      );
    }
    render(<BoardWithExtra />);

    fireEvent.keyDown(screen.getByTestId("task-a1"), { key: "ArrowDown" });

    // The app's handler called preventDefault, so the internal roving-tabindex
    // move must have been skipped.
    expect(screen.getByTestId("task-a1").tabIndex).toBe(0);
    expect(screen.getByTestId("task-a2").tabIndex).toBe(-1);
  });
});
