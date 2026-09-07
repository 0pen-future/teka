import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import type { Class } from "@/features/roster";
import { mockViewport } from "@/test/viewport";

import { RecordsClassSelect } from "../components/records-class-select";
import { classWithSchedule } from "@/features/roster/__tests__/roster-handlers";

function makeClasses(names: string[]): Class[] {
  return names.map((name, index) => ({
    ...classWithSchedule,
    id: `class-${index + 1}`,
    name,
    student_count: 28 - index,
  }));
}

const threeClasses = makeClasses(["Toán 6A", "Văn 7B", "Anh 8C"]);
const sevenClasses = makeClasses([
  "Toán 6A",
  "Văn 7B",
  "Anh 8C",
  "Lý 9D",
  "Hoá 9E",
  "Toán 8 - Tối Thứ Ba",
  "Sinh 7F",
]);

function renderSelect(classes: Class[], selectedId = classes[0]!.id) {
  const onSelect = vi.fn();
  render(
    <>
      <span id="class-label">Lớp</span>
      <RecordsClassSelect
        classes={classes}
        selectedId={selectedId}
        onSelect={onSelect}
        labelId="class-label"
      />
    </>,
  );
  return { onSelect, trigger: screen.getByRole("button", { name: /^Lớp/ }) };
}

describe("RecordsClassSelect (popover, ≥ sm)", () => {
  beforeEach(() => mockViewport(1280));

  it("names the trigger after the label, class and student count", () => {
    const { trigger } = renderSelect(threeClasses);
    expect(trigger).toHaveAccessibleName("Lớp Toán 6A · 28 HS");
    expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
  });

  it("opens a listbox with the current class selected and no dialog", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect(threeClasses, "class-2");
    await user.click(trigger);

    expect(trigger).toHaveAttribute("aria-expanded", "true");
    const listbox = screen.getByRole("listbox", { name: "Chọn lớp" });
    expect(trigger).toHaveAttribute("aria-controls", listbox.id);
    expect(screen.getByRole("option", { name: /Văn 7B/ })).toHaveAttribute("aria-selected", "true");
    expect(screen.getByRole("option", { name: /Văn 7B/ })).toHaveFocus();
    expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveAttribute(
      "aria-selected",
      "false",
    );
    expect(screen.getByText("Lớp đang dạy")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Chọn lớp" })).not.toBeInTheDocument();
    expect(screen.queryByLabelText("Tìm lớp")).not.toBeInTheDocument();
  });

  it("shows the class filter past 5 classes and reports no matches", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect(sevenClasses);
    await user.click(trigger);

    const search = screen.getByLabelText("Tìm lớp");
    expect(search).toHaveFocus();
    await user.type(search, "toán");
    expect(screen.getAllByRole("option")).toHaveLength(2);
    expect(screen.getByRole("option", { name: /Toán 8 - Tối Thứ Ba/ })).toBeInTheDocument();

    await user.clear(search);
    await user.type(search, "zzz");
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText('Không có lớp nào khớp "zzz"')).toBeInTheDocument();
  });

  it("selects a class once on click and closes; re-picking the current class is a no-op", async () => {
    const user = userEvent.setup();
    const { onSelect, trigger } = renderSelect(threeClasses);
    await user.click(trigger);
    await user.click(screen.getByRole("option", { name: /Anh 8C/ }));

    expect(onSelect).toHaveBeenCalledTimes(1);
    expect(onSelect).toHaveBeenCalledWith("class-3");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    expect(trigger).toHaveAttribute("aria-expanded", "false");

    await user.click(trigger);
    await user.click(screen.getByRole("option", { name: /Toán 6A/ }));
    expect(onSelect).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
  });

  it("supports arrow navigation, Enter to pick and Escape to close back to the trigger", async () => {
    const user = userEvent.setup();
    const { onSelect, trigger } = renderSelect(threeClasses);
    await user.click(trigger);
    await user.keyboard("{ArrowDown}{ArrowDown}{Enter}");

    expect(onSelect).toHaveBeenCalledWith("class-3");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());

    await user.click(trigger);
    await user.keyboard("{ArrowUp}");
    expect(screen.getByRole("option", { name: /Anh 8C/ })).toHaveFocus();
    await user.keyboard("{Home}");
    expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveFocus();
    await user.keyboard("{End}");
    expect(screen.getByRole("option", { name: /Anh 8C/ })).toHaveFocus();
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
    expect(onSelect).toHaveBeenCalledTimes(1);
  });

  it("hands typing from an option back to the filter input", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect(sevenClasses);
    await user.click(trigger);
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveFocus();

    await user.keyboard("v");
    const search = screen.getByLabelText("Tìm lớp");
    expect(search).toHaveFocus();
    expect(search).toHaveValue("v");
    expect(screen.getAllByRole("option")).toHaveLength(1);
  });
});

describe("RecordsClassSelect (bottom sheet, < sm)", () => {
  beforeEach(() => mockViewport(375));

  it("opens the same list inside a 'Chọn lớp' sheet and closes after picking", async () => {
    const user = userEvent.setup();
    const { onSelect, trigger } = renderSelect(threeClasses);
    await user.click(trigger);

    const dialog = screen.getByRole("dialog", { name: "Chọn lớp" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(screen.getByRole("listbox", { name: "Chọn lớp" })).toBeInTheDocument();
    expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveFocus();
    expect(dialog).toBeInTheDocument();

    await user.click(screen.getByRole("option", { name: /Văn 7B/ }));
    expect(onSelect).toHaveBeenCalledWith("class-2");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
