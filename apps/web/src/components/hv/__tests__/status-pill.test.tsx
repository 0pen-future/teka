import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { StatusPill, statusPillLabels } from "@/components/hv";
import type { StatusPillStatus } from "@/components/hv";

const CASES: {
  status: StatusPillStatus;
  label: string;
  bgClass: string;
  textClass: string;
}[] = [
  { status: "paid", label: "Đã đóng", bgClass: "bg-mint-50", textClass: "text-mint-600" },
  { status: "partial", label: "Đóng thiếu", bgClass: "bg-sun-100", textClass: "text-sun-600" },
  { status: "unpaid", label: "Chưa đóng", bgClass: "bg-coral-100", textClass: "text-coral-600" },
];

describe("StatusPill", () => {
  it.each(CASES)(
    "renders the $status status with its label and colors",
    ({ status, label, bgClass, textClass }) => {
      render(<StatusPill status={status} />);
      const pill = screen.getByText(label);

      expect(statusPillLabels[status]).toBe(label);
      expect(pill).toHaveClass(bgClass);
      expect(pill).toHaveClass(textClass);
    },
  );

  it("lets callers override the label text via children", () => {
    render(<StatusPill status="paid">Đã thanh toán đủ</StatusPill>);

    expect(screen.getByText("Đã thanh toán đủ")).toBeInTheDocument();
    expect(screen.queryByText(statusPillLabels.paid)).not.toBeInTheDocument();
  });
});
