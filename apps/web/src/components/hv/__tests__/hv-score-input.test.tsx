import * as React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HvScoreInput, SCORE_INPUT_ERROR_TEXT } from "@/components/hv";

function Harness({
  onCommit,
  onNavigate,
  state,
}: {
  onCommit?: (parsed: number | null | "invalid", raw: string) => void;
  onNavigate?: (direction: "up" | "down") => void;
  state?: "idle" | "dirty" | "saved" | "invalid";
}) {
  const [value, setValue] = React.useState("");
  return (
    <HvScoreInput
      aria-label="Điểm Miệng Nguyễn Văn An"
      value={value}
      onChange={setValue}
      onCommit={onCommit}
      onNavigate={onNavigate}
      state={state}
    />
  );
}

describe("HvScoreInput", () => {
  it("renders a decimal text input, never type=number", () => {
    render(<Harness />);
    const input = screen.getByRole("textbox", { name: "Điểm Miệng Nguyễn Văn An" });
    expect(input).toHaveAttribute("type", "text");
    expect(input).toHaveAttribute("inputmode", "decimal");
    expect(input).toHaveAttribute("data-state", "idle");
    expect(input).toHaveClass("min-h-12");
  });

  it("commits the parsed value on blur", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    render(<Harness onCommit={onCommit} />);
    const input = screen.getByRole("textbox");

    await user.type(input, "7,3");
    await user.tab();

    expect(onCommit).toHaveBeenCalledWith(7.5, "7,3");
  });

  it("commits and navigates down on Enter, up on Shift+Enter", async () => {
    const user = userEvent.setup();
    const onCommit = vi.fn();
    const onNavigate = vi.fn();
    render(<Harness onCommit={onCommit} onNavigate={onNavigate} />);
    const input = screen.getByRole("textbox");

    await user.type(input, "8{Enter}");
    expect(onCommit).toHaveBeenLastCalledWith(8, "8");
    expect(onNavigate).toHaveBeenLastCalledWith("down");

    await user.keyboard("{Shift>}{Enter}{/Shift}");
    expect(onNavigate).toHaveBeenLastCalledWith("up");
  });

  it("exposes the invalid state with an error message", () => {
    render(<Harness state="invalid" />);
    const input = screen.getByRole("textbox");
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(input).toHaveAttribute("data-state", "invalid");
    expect(input).toHaveAccessibleDescription(SCORE_INPUT_ERROR_TEXT);
    expect(input).toHaveClass("border-coral-400");
  });

  it("highlights dirty and saved states", () => {
    const { rerender } = render(<Harness state="dirty" />);
    expect(screen.getByRole("textbox")).toHaveClass("bg-sun-100");
    rerender(<Harness state="saved" />);
    expect(screen.getByRole("textbox")).toHaveClass("border-mint-400");
    expect(screen.queryByText(SCORE_INPUT_ERROR_TEXT)).not.toBeInTheDocument();
  });
});
