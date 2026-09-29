import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { HvTableScroll } from "@/components/hv";

describe("HvTableScroll", () => {
  it("renders a named, focusable scroll region", () => {
    render(
      <HvTableScroll aria-label="Bảng danh sách lớp">
        <table />
      </HvTableScroll>,
    );
    const region = screen.getByRole("region", { name: "Bảng danh sách lớp" });
    expect(region).toHaveAttribute("tabindex", "0");
    expect(region).toHaveClass("overflow-x-auto");
  });

  it("is the containing block for absolutely positioned header labels", () => {
    render(<HvTableScroll aria-label="Bảng" />);
    // Without `relative`, an `sr-only` span inside a wide table escapes the
    // scroll box and widens the whole document.
    expect(screen.getByRole("region")).toHaveClass("relative");
  });

  it("merges caller classes and forwards props", () => {
    render(
      <HvTableScroll
        aria-label="Bảng"
        className="max-h-[62vh] overflow-auto"
        data-testid="scroll"
      />,
    );
    const region = screen.getByTestId("scroll");
    expect(region).toHaveClass("max-h-[62vh]", "overflow-auto", "relative");
    expect(region).not.toHaveClass("overflow-x-auto");
  });
});
