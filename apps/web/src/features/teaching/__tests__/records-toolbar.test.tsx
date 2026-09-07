import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { createRef } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Class } from "@/features/roster";
import { classWithSchedule } from "@/features/roster/__tests__/roster-handlers";
import { mockViewport } from "@/test/viewport";

import { RecordsToolbar } from "../components/records-toolbar";

const classes: Class[] = [
  { ...classWithSchedule, id: "class-1", name: "Toán 6A", student_count: 28 },
  { ...classWithSchedule, id: "class-2", name: "Văn 7B", student_count: 12 },
];

function renderToolbar(overrides: Partial<Parameters<typeof RecordsToolbar>[0]> = {}) {
  const onQueryChange = vi.fn();
  const onSelectClass = vi.fn();
  const searchRef = createRef<HTMLInputElement>();
  render(
    <RecordsToolbar
      classes={classes}
      selectedClassId="class-1"
      onSelectClass={onSelectClass}
      query=""
      onQueryChange={onQueryChange}
      matched={28}
      total={28}
      searchRef={searchRef}
      compact={false}
      {...overrides}
    />,
  );
  return { onQueryChange, onSelectClass, searchRef };
}

describe("RecordsToolbar", () => {
  beforeEach(() => mockViewport(1280));

  it("labels the class picker and the student search", () => {
    const { searchRef } = renderToolbar();
    expect(screen.getByRole("button", { name: "Lớp Toán 6A · 28 HS" })).toBeInTheDocument();
    const search = screen.getByLabelText("Tìm học sinh");
    expect(search).toHaveAttribute("placeholder", "Gõ tên học sinh…");
    expect(searchRef.current).toBe(search);
  });

  it("shows the / hint while empty and the clear button once a query is typed", async () => {
    const user = userEvent.setup();
    const { onQueryChange } = renderToolbar();
    expect(screen.getByText("/")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Xoá tìm kiếm" })).not.toBeInTheDocument();

    await user.type(screen.getByLabelText("Tìm học sinh"), "an");
    expect(onQueryChange).toHaveBeenNthCalledWith(1, "a");
    expect(onQueryChange).toHaveBeenNthCalledWith(2, "n");
  });

  it("clears the query and refocuses the input from the × button", async () => {
    const user = userEvent.setup();
    const { onQueryChange } = renderToolbar({ query: "an", matched: 3 });
    expect(screen.queryByText("/")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Xoá tìm kiếm" }));
    expect(onQueryChange).toHaveBeenCalledWith("");
    expect(screen.getByLabelText("Tìm học sinh")).toHaveFocus();
  });

  it("reports the match counter as a polite status", () => {
    renderToolbar({ query: "an", matched: 3 });
    const status = screen.getByRole("status");
    expect(status).toHaveAttribute("aria-live", "polite");
    expect(status).toHaveTextContent("3 / 28 học sinh");
  });

  it("reports the plain total without a query", () => {
    renderToolbar();
    expect(screen.getByRole("status")).toHaveTextContent(/^28 học sinh$/);
  });

  it("leaves the counter empty while total is unknown", () => {
    renderToolbar({ total: null, matched: 0 });
    expect(screen.getByRole("status")).toBeEmptyDOMElement();
  });

  it("drops the counter from the toolbar in compact mode", () => {
    renderToolbar({ compact: true });
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Tìm học sinh")).toBeInTheDocument();
  });

  it("forwards class picks", async () => {
    const user = userEvent.setup();
    const { onSelectClass } = renderToolbar();
    await user.click(screen.getByRole("button", { name: /^Lớp/ }));
    await user.click(screen.getByRole("option", { name: /Văn 7B/ }));
    expect(onSelectClass).toHaveBeenCalledWith("class-2");
  });
});
