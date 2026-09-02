import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { HvButton, HvNotice, HvStateBlock } from "@/components/hv";

describe("HvStateBlock", () => {
  it("announces loading politely with a visually hidden title", () => {
    render(<HvStateBlock state="loading" title="Đang tải danh sách học sinh…" compact />);
    const block = screen.getByRole("status");
    expect(block).toHaveAttribute("aria-live", "polite");
    expect(block).toHaveAttribute("data-state", "loading");
    expect(screen.getByText("Đang tải danh sách học sinh…")).toHaveClass("sr-only");
    expect(block).toHaveClass("p-[var(--space-3)]");
  });

  it("renders an empty state with copy and an action", () => {
    render(
      <HvStateBlock
        state="empty"
        title="Chưa có bộ điểm nào."
        description="Tạo bộ điểm đầu tiên để bắt đầu."
        action={<HvButton size="sm">Tạo bộ điểm</HvButton>}
      />,
    );
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(screen.getByText("Chưa có bộ điểm nào.")).toBeVisible();
    expect(screen.getByText("Tạo bộ điểm đầu tiên để bắt đầu.")).toBeVisible();
    expect(screen.getByRole("button", { name: "Tạo bộ điểm" })).toBeInTheDocument();
  });

  it("renders errors as an alert", () => {
    render(<HvStateBlock state="error" title="Không tải được danh sách." />);
    expect(screen.getByRole("alert")).toHaveTextContent("Không tải được danh sách.");
  });
});

describe("HvNotice", () => {
  it("defaults to a note, and to an alert for danger", () => {
    const { rerender } = render(<HvNotice tone="info">Lớp đã ghi nhận điểm.</HvNotice>);
    expect(screen.getByRole("note")).toHaveClass("bg-sky-50");

    rerender(<HvNotice tone="danger">Không lưu được.</HvNotice>);
    expect(screen.getByRole("alert")).toHaveClass("bg-coral-100");
  });

  it("accepts an explicit role and title", () => {
    render(
      <HvNotice tone="warning" role="alert" title="Không thể gán">
        Lớp đã có điểm.
      </HvNotice>,
    );
    const notice = screen.getByRole("alert");
    expect(notice).toHaveTextContent("Không thể gán");
    expect(notice).toHaveTextContent("Lớp đã có điểm.");
    expect(notice).toHaveClass("bg-sun-100");
  });
});
