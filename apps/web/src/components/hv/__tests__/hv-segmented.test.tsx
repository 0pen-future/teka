import * as React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HvSegmented } from "@/components/hv";

type Tab = "note" | "plan" | "scores";

const OPTIONS = [
  { value: "note", label: "Ghi chú" },
  { value: "plan", label: "Kế hoạch" },
  { value: "scores", label: "Điểm" },
] as const satisfies readonly { value: Tab; label: string }[];

function Harness({
  variant,
  onValueChange,
  disabledValue,
}: {
  variant: "segmented" | "tabs";
  onValueChange?: (value: Tab) => void;
  disabledValue?: Tab;
}) {
  const [value, setValue] = React.useState<Tab>("note");
  return (
    <HvSegmented<Tab>
      variant={variant}
      idBase="session-detail"
      aria-label="Mục chi tiết buổi"
      options={OPTIONS.map((option) => ({
        ...option,
        disabled: option.value === disabledValue,
      }))}
      value={value}
      onValueChange={(next) => {
        setValue(next);
        onValueChange?.(next);
      }}
    />
  );
}

describe("HvSegmented", () => {
  it("styles both variants as a bordered button group with a mint-filled active item", () => {
    const { unmount } = render(<Harness variant="tabs" />);
    expect(screen.getByRole("tablist")).toHaveClass("border-2", "border-line-200", "bg-white");
    const activeTab = screen.getByRole("tab", { name: "Ghi chú" });
    expect(activeTab).toHaveAttribute("data-state", "active");
    expect(activeTab).toHaveClass("data-[state=active]:bg-mint-400");
    expect(screen.getByRole("tab", { name: "Điểm" })).toHaveAttribute("data-state", "inactive");
    unmount();

    render(<Harness variant="segmented" />);
    expect(screen.getByRole("radiogroup")).toHaveClass("border-2", "border-line-200");
    expect(screen.getByRole("radio", { name: "Ghi chú" })).toHaveClass(
      "data-[state=checked]:bg-mint-400",
    );
  });

  it("renders the segmented variant as a radio group", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(<Harness variant="segmented" onValueChange={onValueChange} />);

    expect(screen.getByRole("radiogroup", { name: "Mục chi tiết buổi" })).toBeInTheDocument();
    const note = screen.getByRole("radio", { name: "Ghi chú" });
    expect(note).toHaveAttribute("aria-checked", "true");
    expect(note).toHaveClass("min-h-11");

    await user.click(screen.getByRole("radio", { name: "Điểm" }));
    expect(onValueChange).toHaveBeenCalledWith("scores");
    expect(screen.getByRole("radio", { name: "Điểm" })).toHaveAttribute("aria-checked", "true");
  });

  it("renders the tabs variant with tablist semantics and stable ids", async () => {
    const user = userEvent.setup();
    render(<Harness variant="tabs" />);

    expect(screen.getByRole("tablist", { name: "Mục chi tiết buổi" })).toBeInTheDocument();
    const note = screen.getByRole("tab", { name: "Ghi chú" });
    expect(note).toHaveAttribute("aria-selected", "true");
    expect(note).toHaveAttribute("id", "session-detail-tab-note");
    expect(note).toHaveAttribute("aria-controls", "session-detail-panel-note");
    expect(screen.getByRole("tab", { name: "Điểm" })).toHaveAttribute("tabindex", "-1");

    await user.click(screen.getByRole("tab", { name: "Kế hoạch" }));
    expect(screen.getByRole("tab", { name: "Kế hoạch" })).toHaveAttribute("aria-selected", "true");
    expect(note).toHaveAttribute("aria-selected", "false");
  });

  it("moves between tabs with the arrow keys, skipping disabled ones", async () => {
    const user = userEvent.setup();
    const onValueChange = vi.fn();
    render(<Harness variant="tabs" onValueChange={onValueChange} disabledValue="plan" />);

    const note = screen.getByRole("tab", { name: "Ghi chú" });
    note.focus();
    await user.keyboard("{ArrowRight}");
    expect(onValueChange).toHaveBeenLastCalledWith("scores");
    expect(screen.getByRole("tab", { name: "Điểm" })).toHaveFocus();

    await user.keyboard("{ArrowRight}");
    expect(onValueChange).toHaveBeenLastCalledWith("note");
    expect(note).toHaveFocus();

    await user.keyboard("{End}");
    expect(screen.getByRole("tab", { name: "Điểm" })).toHaveFocus();
  });
});
