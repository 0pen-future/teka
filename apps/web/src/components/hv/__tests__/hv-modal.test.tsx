import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HvModal } from "@/components/hv";

describe("HvModal", () => {
  it("renders the title and children while open", async () => {
    render(
      <HvModal open onOpenChange={vi.fn()} title="Xác nhận thanh toán">
        <p>Nội dung modal</p>
      </HvModal>,
    );

    const dialog = screen.getByRole("dialog");
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText("Xác nhận thanh toán")).toBeInTheDocument();
    expect(screen.getByText("Nội dung modal")).toBeInTheDocument();
    // Radix moves focus inside the panel on open (onto the panel itself or
    // its first focusable descendant, e.g. the close button).
    await waitFor(() => expect(dialog.contains(document.activeElement)).toBe(true));
  });

  it("still exposes an accessible name when no title is given", () => {
    render(
      <HvModal open onOpenChange={vi.fn()}>
        <p>Nội dung modal</p>
      </HvModal>,
    );

    const dialog = screen.getByRole("dialog", { name: "Hộp thoại" });
    expect(dialog).toBeInTheDocument();
    expect(screen.getByText("Hộp thoại")).toHaveClass("sr-only");
  });

  it("hides the content once open is false", () => {
    const onOpenChange = vi.fn();
    const { rerender } = render(
      <HvModal open onOpenChange={onOpenChange} title="Xác nhận">
        <p>Nội dung modal</p>
      </HvModal>,
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();

    rerender(
      <HvModal open={false} onOpenChange={onOpenChange} title="Xác nhận">
        <p>Nội dung modal</p>
      </HvModal>,
    );
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("reports a close request on Escape", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <HvModal open onOpenChange={onOpenChange} title="Xác nhận">
        <p>Nội dung modal</p>
      </HvModal>,
    );

    await user.keyboard("{Escape}");
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });

  it("reports a close request when the close button is activated", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <HvModal open onOpenChange={onOpenChange} title="Xác nhận">
        <p>Nội dung modal</p>
      </HvModal>,
    );

    await user.click(screen.getByRole("button", { name: /đóng/i }));
    expect(onOpenChange).toHaveBeenCalledWith(false);
  });
});

describe("HvModal size", () => {
  it("defaults to the md card width", () => {
    render(
      <HvModal open onOpenChange={vi.fn()} title="Xác nhận">
        <p>Nội dung</p>
      </HvModal>,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("data-size", "md");
    expect(dialog).toHaveClass("sm:max-w-md");
  });

  it("renders xl as a content-height workspace capped at 90dvh with a scrolling body", () => {
    render(
      <HvModal
        open
        onOpenChange={vi.fn()}
        title="Bảng điểm"
        size="xl"
        footer={<button>Đóng</button>}
      >
        <p>Nội dung</p>
      </HvModal>,
    );
    const dialog = screen.getByRole("dialog");
    expect(dialog).toHaveAttribute("data-size", "xl");
    expect(dialog).toHaveClass("sm:max-h-[90dvh]", "flex-col");
    expect(dialog).not.toHaveClass("sm:h-[90dvh]");
    expect(dialog).not.toHaveClass("sm:max-w-md");
    const body = screen.getByText("Nội dung").parentElement;
    expect(body).toHaveClass("flex-1", "min-h-0", "overflow-auto");
  });
});
