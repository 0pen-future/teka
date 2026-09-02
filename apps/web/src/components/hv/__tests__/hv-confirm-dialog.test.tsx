import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import { HvConfirmDialog } from "@/components/hv";

describe("HvConfirmDialog", () => {
  it("focuses cancel first and confirms only through the confirm action", async () => {
    const user = userEvent.setup();
    const onConfirm = vi.fn();
    const onOpenChange = vi.fn();
    render(
      <HvConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Xóa bộ điểm?"
        description="Không thể hoàn tác."
        confirmLabel="Xác nhận xóa"
        tone="danger"
        onConfirm={onConfirm}
      />,
    );

    expect(screen.getByRole("dialog", { name: "Xóa bộ điểm?" })).toBeInTheDocument();
    const cancel = screen.getByRole("button", { name: "Hủy" });
    await waitFor(() => expect(cancel).toHaveFocus());

    await user.click(cancel);
    expect(onOpenChange).toHaveBeenCalledWith(false);
    expect(onConfirm).not.toHaveBeenCalled();

    await user.click(screen.getByRole("button", { name: "Xác nhận xóa" }));
    expect(onConfirm).toHaveBeenCalledTimes(1);
  });

  it("styles the confirm action by tone", () => {
    const { rerender } = render(
      <HvConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="Gán bộ điểm?"
        confirmLabel="Gán"
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Gán" })).toHaveClass("bg-mint-400");

    rerender(
      <HvConfirmDialog
        open
        onOpenChange={vi.fn()}
        title="Xóa?"
        confirmLabel="Xóa"
        tone="danger"
        onConfirm={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "Xóa" })).toHaveClass("bg-coral-400");
  });

  it("disables both actions and ignores dismissal while pending", async () => {
    const user = userEvent.setup();
    const onOpenChange = vi.fn();
    render(
      <HvConfirmDialog
        open
        onOpenChange={onOpenChange}
        title="Xóa?"
        confirmLabel="Xóa"
        pending
        onConfirm={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: "Hủy" })).toBeDisabled();
    expect(screen.getByRole("button", { name: "Xóa" })).toBeDisabled();

    await user.keyboard("{Escape}");
    expect(onOpenChange).not.toHaveBeenCalled();
  });
});
