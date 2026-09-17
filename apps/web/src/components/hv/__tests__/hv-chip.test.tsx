import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HvChip } from "@/components/hv";

describe("HvChip", () => {
  it("renders unpressed by default and toggles aria-pressed on click", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <HvChip onClick={onClick} aria-label="Ưu tiên cao">
        Ưu tiên cao
      </HvChip>,
    );

    const chip = screen.getByRole("button", { name: "Ưu tiên cao" });
    expect(chip).toHaveAttribute("aria-pressed", "false");
    expect(chip).not.toHaveAttribute("aria-checked");
    expect(chip).not.toHaveClass("bg-mint-50");

    await user.click(chip);
    expect(onClick).toHaveBeenCalledTimes(1);
  });

  it("applies the pressed style when pressed is true", () => {
    render(<HvChip pressed>Đã chọn</HvChip>);
    const chip = screen.getByRole("button", { name: "Đã chọn" });
    expect(chip).toHaveAttribute("aria-pressed", "true");
    expect(chip).toHaveClass("bg-mint-50", "text-mint-600");
  });

  it("renders a hidden color dot for the given variant", () => {
    render(<HvChip dot="danger">Quá hạn</HvChip>);
    const chip = screen.getByRole("button", { name: "Quá hạn" });
    const dot = chip.querySelector("[aria-hidden='true']");
    expect(dot).not.toBeNull();
    expect(dot).toHaveClass("bg-coral-600");
  });

  it("shows the count when provided", () => {
    render(<HvChip count={7}>Chưa giao</HvChip>);
    const chip = screen.getByRole("button", { name: /Chưa giao/ });
    expect(chip).toHaveTextContent("Chưa giao7");
  });

  it("switches to aria-checked when used as a radio item", () => {
    render(
      <div role="radiogroup" aria-label="Bộ lọc">
        <HvChip role="radio" pressed>
          Tất cả
        </HvChip>
        <HvChip role="radio" pressed={false}>
          Của tôi
        </HvChip>
      </div>,
    );

    const all = screen.getByRole("radio", { name: "Tất cả" });
    expect(all).toHaveAttribute("aria-checked", "true");
    expect(all).not.toHaveAttribute("aria-pressed");

    const mine = screen.getByRole("radio", { name: "Của tôi" });
    expect(mine).toHaveAttribute("aria-checked", "false");
  });
});
