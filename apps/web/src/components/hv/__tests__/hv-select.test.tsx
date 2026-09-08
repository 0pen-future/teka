import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import { HvModal, HvSelect } from "@/components/hv";
import type { HvSelectOption, HvSelectProps } from "@/components/hv";
import { mockViewport } from "@/test/viewport";

function makeOptions(names: string[]): HvSelectOption[] {
  return names.map((name, index) => ({
    value: `k${index + 1}`,
    label: name,
    meta: `${28 - index} HS`,
  }));
}

const classes = makeOptions(["Toán 6A", "Văn 7B"]);
const manyClasses = makeOptions([
  "Toán 6A",
  "Văn 7B",
  "Lý 8C",
  "Hoá 9A",
  "Sinh 6B",
  "Sử 7A",
  "Toán 9C",
]);

function renderSelect(overrides: Partial<HvSelectProps> = {}) {
  const onValueChange = vi.fn();
  const view = render(
    <>
      <span id="class-label">Lớp</span>
      <HvSelect
        labelId="class-label"
        sheetTitle="Chọn lớp"
        groupLabel="Lớp đang dạy"
        searchNoun="lớp"
        options={classes}
        value="k1"
        onValueChange={onValueChange}
        {...overrides}
      />
    </>,
  );
  return {
    ...view,
    onValueChange,
    trigger: screen.getByRole("combobox", { name: /^Lớp/ }),
  };
}

describe("HvSelect popover (sm and up)", () => {
  beforeEach(() => mockViewport(1280));

  it("names the trigger after the label, selected option and meta", () => {
    const { trigger } = renderSelect();
    expect(trigger).toHaveAccessibleName("Lớp Toán 6A · 28 HS");
    expect(trigger).toHaveAttribute("aria-haspopup", "listbox");
    expect(trigger).toHaveAttribute("aria-expanded", "false");
    expect(trigger).toHaveAttribute("data-value", "k1");
    expect(trigger).not.toHaveAttribute("data-placeholder");
    expect(trigger).toHaveAttribute("aria-controls");
  });

  it("opens a listbox named after the sheet title, focusing the selected option", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect();
    await user.click(trigger);

    const listbox = await screen.findByRole("listbox", { name: "Chọn lớp" });
    expect(trigger).toHaveAttribute("aria-expanded", "true");
    expect(trigger).toHaveAttribute("aria-controls", listbox.id);
    expect(screen.getByText("Lớp đang dạy")).toBeInTheDocument();
    expect(screen.queryByRole("dialog", { name: "Chọn lớp" })).not.toBeInTheDocument();
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
    const selected = screen.getByRole("option", { name: /Toán 6A/ });
    expect(selected).toHaveAttribute("aria-selected", "true");
    await waitFor(() => expect(selected).toHaveFocus());
  });

  it("renders option label and meta as adjacent spans, without a separator", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect();
    expect(trigger).toHaveTextContent("Toán 6A · 28 HS");
    await user.click(trigger);
    const option = await screen.findByRole("option", { name: /Toán 6A/ });
    expect(option.textContent).toBe("Toán 6A28 HS");
    expect(option.textContent).not.toContain("·");
  });

  it("omits the group label and meta when not provided", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({
      groupLabel: undefined,
      options: [{ value: "k1", label: "Toán 6A" }],
    });
    expect(trigger).toHaveAccessibleName("Lớp Toán 6A");
    await user.click(trigger);
    await screen.findByRole("listbox");
    expect(screen.queryByText("Lớp đang dạy")).not.toBeInTheDocument();
  });

  it("filters past the threshold, using the search noun in the empty note", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ options: manyClasses });
    await user.click(trigger);

    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    expect(search).toHaveAttribute("placeholder", "Tìm lớp…");
    await waitFor(() => expect(search).toHaveFocus());
    await user.keyboard("toán");
    expect(screen.getAllByRole("option")).toHaveLength(2);
    await user.clear(search);
    await user.keyboard("zzz");
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText('Không có lớp nào khớp "zzz"')).toBeInTheDocument();
  });

  it("defaults the search noun to mục", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ options: manyClasses, searchNoun: undefined });
    await user.click(trigger);
    const search = await screen.findByRole("searchbox", { name: "Tìm mục" });
    await user.type(search, "zzz");
    expect(screen.getByText('Không có mục nào khớp "zzz"')).toBeInTheDocument();
  });

  it("never shows the filter when searchThreshold is Infinity", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ options: manyClasses, searchThreshold: Infinity });
    await user.click(trigger);
    await screen.findByRole("listbox");
    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
    expect(screen.getAllByRole("option")).toHaveLength(7);
  });

  it("reports the picked value on click and closes, even when re-picking", async () => {
    const user = userEvent.setup();
    const { trigger, onValueChange } = renderSelect();
    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: /Văn 7B/ }));
    expect(onValueChange).toHaveBeenCalledWith("k2");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());

    await user.click(trigger);
    await user.click(await screen.findByRole("option", { name: /Toán 6A/ }));
    expect(onValueChange).toHaveBeenLastCalledWith("k1");
  });

  it("moves with the arrow keys, picks with Enter and dismisses with Escape", async () => {
    const user = userEvent.setup();
    const { trigger, onValueChange } = renderSelect();
    trigger.focus();
    await user.keyboard("{Enter}");
    const first = await screen.findByRole("option", { name: /Toán 6A/ });
    await waitFor(() => expect(first).toHaveFocus());

    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: /Văn 7B/ })).toHaveFocus();
    await user.keyboard("{ArrowDown}");
    expect(first).toHaveFocus();
    await user.keyboard("{End}");
    expect(screen.getByRole("option", { name: /Văn 7B/ })).toHaveFocus();
    await user.keyboard("{Home}");
    expect(first).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    await user.keyboard("{Enter}");
    expect(onValueChange).toHaveBeenCalledWith("k2");
    await waitFor(() => expect(trigger).toHaveFocus());

    await user.keyboard("{Enter}");
    await screen.findByRole("listbox");
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
  });

  it("hands typing from an option over to the filter", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ options: manyClasses, value: "k3" });
    await user.click(trigger);
    const search = await screen.findByRole("searchbox", { name: "Tìm lớp" });
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveFocus();
    await user.keyboard("v");
    expect(search).toHaveFocus();
    expect(search).toHaveValue("v");
    expect(screen.getAllByRole("option")).toHaveLength(1);
  });

  it("activates the focused option with Space", async () => {
    const user = userEvent.setup();
    const { trigger, onValueChange } = renderSelect();
    trigger.focus();
    await user.keyboard("{Enter}");
    await waitFor(() => expect(screen.getByRole("option", { name: /Toán 6A/ })).toHaveFocus());
    await user.keyboard("{ArrowDown} ");
    expect(onValueChange).toHaveBeenCalledWith("k2");
  });

  it("shows the placeholder while nothing is selected", () => {
    const { trigger } = renderSelect({ value: "", placeholder: "Chọn lớp" });
    expect(trigger).toHaveTextContent("Chọn lớp");
    expect(trigger).toHaveAttribute("data-placeholder", "");
    expect(trigger).toHaveAttribute("data-value", "");
  });

  it("does not open while disabled", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ disabled: true });
    expect(trigger).toBeDisabled();
    await user.click(trigger);
    expect(screen.queryByRole("listbox")).not.toBeInTheDocument();
  });

  it("reflects aria-invalid on the trigger", () => {
    const { trigger, rerender } = renderSelect({ "aria-invalid": true });
    expect(trigger).toHaveAttribute("aria-invalid", "true");
    rerender(
      <>
        <span id="class-label">Lớp</span>
        <HvSelect
          labelId="class-label"
          sheetTitle="Chọn lớp"
          options={classes}
          value="k1"
          onValueChange={vi.fn()}
        />
      </>,
    );
    expect(screen.getByRole("combobox")).not.toHaveAttribute("aria-invalid");
  });

  it("skips disabled options with the arrow keys and refuses to pick them", async () => {
    const user = userEvent.setup();
    const options: HvSelectOption[] = [
      { value: "a", label: "Toán 6A" },
      { value: "b", label: "Tùy chỉnh", disabled: true },
      { value: "c", label: "Văn 7B" },
    ];
    const { trigger, onValueChange } = renderSelect({ options, value: "a" });
    trigger.focus();
    await user.keyboard("{Enter}");
    const first = await screen.findByRole("option", { name: "Toán 6A" });
    await waitFor(() => expect(first).toHaveFocus());
    const custom = screen.getByRole("option", { name: "Tùy chỉnh" });
    expect(custom).toBeDisabled();
    expect(custom).toHaveAttribute("aria-disabled", "true");

    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: "Văn 7B" })).toHaveFocus();
    await user.keyboard("{ArrowUp}");
    expect(first).toHaveFocus();

    await user.click(custom);
    expect(onValueChange).not.toHaveBeenCalled();
    expect(screen.getByRole("listbox")).toBeInTheDocument();
  });

  it("takes its accessible name from aria-label", () => {
    render(
      <HvSelect
        aria-label="Giáo viên"
        sheetTitle="Giáo viên"
        options={classes}
        value="k1"
        onValueChange={vi.fn()}
      />,
    );
    expect(screen.getByRole("combobox", { name: "Giáo viên" })).toBeInTheDocument();
  });

  it("takes its accessible name from a label wired through htmlFor", () => {
    render(
      <>
        <label htmlFor="payment-method">Hình thức</label>
        <HvSelect
          id="payment-method"
          sheetTitle="Hình thức"
          options={classes}
          value="k1"
          onValueChange={vi.fn()}
        />
      </>,
    );
    expect(screen.getByRole("combobox", { name: "Hình thức" })).toHaveAttribute(
      "id",
      "payment-method",
    );
  });

  it("keeps the class token set of the records class dropdown", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ className: "min-w-[230px] max-sm:w-full" });
    const tokens = (element: Element) => new Set(element.className.split(/\s+/).filter(Boolean));
    expect(tokens(trigger)).toEqual(
      new Set([
        "inline-flex",
        "min-h-11",
        "min-w-[230px]",
        "items-center",
        "justify-between",
        "gap-2.5",
        "rounded-[14px]",
        "border-2",
        "border-line-200",
        "bg-white",
        "pl-3.5",
        "pr-2.5",
        "text-[14.5px]",
        "font-extrabold",
        "text-ink-900",
        "hover:border-mint-300",
        "data-[state=open]:border-mint-400",
        "focus-visible:ring-4",
        "focus-visible:outline-none",
        "max-sm:w-full",
        "disabled:cursor-not-allowed",
        "disabled:bg-cream-200",
        "disabled:text-ink-300",
        "aria-invalid:border-coral-400",
        "data-placeholder:font-bold",
        "data-placeholder:text-ink-400",
      ]),
    );

    await user.click(trigger);
    const listbox = await screen.findByRole("listbox");
    expect(tokens(screen.getByRole("option", { name: /Toán 6A/ }))).toEqual(
      new Set([
        "flex",
        "min-h-[42px]",
        "w-full",
        "items-center",
        "gap-2.5",
        "rounded-[11px]",
        "px-3",
        "text-left",
        "text-[14px]",
        "font-bold",
        "text-ink-700",
        "hover:bg-cream-100",
        "focus-visible:bg-cream-100",
        "focus-visible:outline-none",
        "aria-selected:bg-mint-50",
        "aria-selected:text-mint-700",
        "aria-disabled:pointer-events-none",
        "aria-disabled:opacity-50",
      ]),
    );
    const content = listbox.closest('[role="dialog"][data-state="open"]');
    expect(content).not.toBeNull();
    expect(tokens(content!)).toEqual(
      new Set([
        "z-50",
        "w-max",
        "min-w-(--radix-popover-trigger-width)",
        "max-w-[320px]",
        "rounded-[16px]",
        "border-2",
        "border-line-200",
        "bg-white",
        "p-1.5",
        "shadow-soft-lg",
        "max-h-(--radix-popover-content-available-height)",
        "overflow-y-auto",
        "origin-(--radix-popover-content-transform-origin)",
        "data-open:animate-in",
        "data-open:fade-in-0",
        "data-open:zoom-in-95",
        "motion-reduce:animate-none",
      ]),
    );
  });

  it("aligns the popover to the trigger end when asked", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ align: "end" });
    await user.click(trigger);
    const listbox = await screen.findByRole("listbox");
    expect(listbox.closest('[role="dialog"]')).toHaveAttribute("data-align", "end");
  });

  it("picks inside an HvModal without closing the modal", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <HvModal open onOpenChange={onOpenChange} title="Ghi nhận thanh toán">
        <HvSelect
          aria-label="Hình thức"
          sheetTitle="Hình thức"
          options={classes}
          value="k1"
          onValueChange={onValueChange}
        />
      </HvModal>,
    );
    const trigger = screen.getByRole("combobox", { name: "Hình thức" });
    await user.click(trigger);
    // Focus must land inside the non-modal popover, not be pulled back by
    // the dialog's focus trap, or the arrow keys are dead.
    const first = await screen.findByRole("option", { name: /Toán 6A/ });
    await waitFor(() => expect(first).toHaveFocus());
    await user.keyboard("{ArrowDown}");
    expect(screen.getByRole("option", { name: /Văn 7B/ })).toHaveFocus();
    await user.keyboard("{Enter}");

    expect(onValueChange).toHaveBeenCalledWith("k2");
    await waitFor(() => expect(screen.queryByRole("listbox")).not.toBeInTheDocument());
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    expect(screen.getByRole("dialog", { name: "Ghi nhận thanh toán" })).toBeInTheDocument();
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});

describe("HvSelect bottom sheet (below sm)", () => {
  beforeEach(() => mockViewport(375));

  it("opens the list in a sheet, closes on pick and returns focus", async () => {
    const user = userEvent.setup();
    const { trigger, onValueChange } = renderSelect();
    await user.click(trigger);

    const dialog = await screen.findByRole("dialog", { name: "Chọn lớp" });
    const listbox = within(dialog).getByRole("listbox", { name: "Chọn lớp" });
    expect(trigger).toHaveAttribute("aria-controls", listbox.id);
    await waitFor(() =>
      expect(within(dialog).getByRole("option", { name: /Toán 6A/ })).toHaveFocus(),
    );

    await user.click(within(dialog).getByRole("option", { name: /Văn 7B/ }));
    expect(onValueChange).toHaveBeenCalledWith("k2");
    await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
    expect(trigger).toHaveFocus();
  });

  it("keeps focus on the listbox when there is nothing to pick", async () => {
    const user = userEvent.setup();
    const { trigger } = renderSelect({ options: [], value: "", placeholder: "Chọn lớp…" });
    await user.click(trigger);
    const dialog = await screen.findByRole("dialog", { name: "Chọn lớp" });
    const listbox = within(dialog).getByRole("listbox", { name: "Chọn lớp" });
    await waitFor(() => expect(listbox).toHaveFocus());
    expect(within(dialog).queryAllByRole("option")).toHaveLength(0);
  });

  it("stacks a sheet above an HvModal and Escape only closes the sheet", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <HvModal open onOpenChange={onOpenChange} title="Ghi nhận thanh toán">
        <HvSelect
          aria-label="Hình thức"
          sheetTitle="Hình thức"
          options={classes}
          value="k1"
          onValueChange={vi.fn()}
        />
      </HvModal>,
    );
    const trigger = screen.getByRole("combobox", { name: "Hình thức" });
    await user.click(trigger);

    await screen.findByRole("dialog", { name: "Hình thức" });
    expect(screen.getAllByRole("dialog", { hidden: true })).toHaveLength(2);

    await user.keyboard("{Escape}");
    await waitFor(() =>
      expect(screen.queryByRole("dialog", { name: "Hình thức" })).not.toBeInTheDocument(),
    );
    expect(screen.getByRole("dialog", { name: "Ghi nhận thanh toán" })).toBeInTheDocument();
    expect(onOpenChange).not.toHaveBeenCalledWith(false);
    await waitFor(() => expect(trigger).toHaveFocus());
  });
});
