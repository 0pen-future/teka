import { getDefaultNormalizer, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { formatMoney } from "@/lib/utils";

import type { Statement } from "../types/statement-types";
import { StatementView } from "../components/statement-view";

// `formatMoney` renders a non-breaking space before "₫" (Intl's vi-VN
// currency format). RTL's default text normalizer collapses that into a
// regular space when reading the DOM but leaves the raw search string
// untouched, so an exact `getByText(formatMoney(...))` never matches unless
// whitespace-collapsing is turned off here too.
const moneyMatcher = { normalizer: getDefaultNormalizer({ collapseWhitespace: false }) };

const baseStatement: Statement = {
  contact_name: "Anh Minh",
  period: "08/2026",
  children: [
    {
      student_name: "Trần Thị Bích",
      display_note: "Lớp 6",
      opening_balance: 500_000,
      classes: [
        {
          class_name: "Toán 6A",
          unit_price: 150_000,
          billable_count: 10,
          absent_count: 1,
          amount: 1_500_000,
          sessions: [
            { date: "2026-08-03", status: "present", counted: true },
            { date: "2026-08-05", status: "present", counted: true },
            { date: "2026-08-07", status: "absent", counted: false },
          ],
        },
        {
          class_name: "Lý 6A",
          unit_price: 130_000,
          billable_count: 8,
          absent_count: 0,
          amount: 1_040_000,
          sessions: [{ date: "2026-08-04", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 3_040_000,
    },
    {
      student_name: "Trần Văn Cường",
      display_note: null,
      opening_balance: 0,
      classes: [
        {
          class_name: "Văn 9A",
          unit_price: 160_000,
          billable_count: 9,
          absent_count: 0,
          amount: 1_440_000,
          sessions: [{ date: "2026-08-02", status: "present", counted: true }],
        },
      ],
      adjustments: [],
      carried_adjustment: null,
      subtotal: 1_440_000,
    },
  ],
  totals: {
    opening_balance: 500_000,
    current_charge: 3_980_000,
    adjustment_total: 0,
    total_due: 4_480_000,
    paid: 0,
    outstanding: 4_480_000,
  },
  payments: {
    total_paid: 0,
    by_invoice: [
      { student_name: "Trần Thị Bích", total_due: 3_040_000, paid: 0, outstanding: 3_040_000 },
      { student_name: "Trần Văn Cường", total_due: 1_440_000, paid: 0, outstanding: 1_440_000 },
    ],
  },
  qr: {
    image_url: "https://img.vietqr.io/image/example-minh.png",
    amount: 4_480_000,
    note: "HP Bich Cuong T8",
  },
};

describe("StatementView", () => {
  it("renders one section per child and exactly one grand total", () => {
    render(<StatementView statement={baseStatement} />);

    expect(screen.getByText("Trần Thị Bích")).toBeInTheDocument();
    expect(screen.getByText("Trần Văn Cường")).toBeInTheDocument();
    expect(screen.getByText("Tổng cộng cả gia đình")).toBeInTheDocument();
    expect(screen.getAllByText("Tổng cộng cả gia đình")).toHaveLength(1);
  });

  it("renders two class blocks for a child enrolled in two classes", () => {
    render(<StatementView statement={baseStatement} />);

    expect(screen.getByText("Toán 6A")).toBeInTheDocument();
    expect(screen.getByText("Lý 6A")).toBeInTheDocument();
  });

  it("renders both attended and absent session dates as chips", () => {
    render(<StatementView statement={baseStatement} />);

    expect(screen.getByText("03/08 ✓")).toBeInTheDocument();
    expect(screen.getByText("05/08 ✓")).toBeInTheDocument();
    expect(screen.getByText("07/08 ✕")).toBeInTheDocument();
  });

  it("shows the fee formula with server-supplied count, unit price, and amount only", () => {
    render(<StatementView statement={baseStatement} />);

    expect(
      screen.getByText(
        `10 buổi × ${formatMoney(150_000)} = ${formatMoney(1_500_000)}`,
        moneyMatcher,
      ),
    ).toBeInTheDocument();
  });

  it("renders nợ cũ as its own line, separate from the class fee amount", () => {
    render(<StatementView statement={baseStatement} />);

    expect(screen.getByText(`Nợ cũ: ${formatMoney(500_000)}`, moneyMatcher)).toBeInTheDocument();
  });

  it("renders the copyable note text and no broken image when qr is null", () => {
    render(<StatementView statement={{ ...baseStatement, qr: null }} />);

    expect(
      screen.getByText(
        "Chưa có mã QR chuyển khoản. Vui lòng liên hệ thầy/cô để biết thông tin chuyển khoản.",
      ),
    ).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });

  it("renders the grand total equal to the server's total_due verbatim", () => {
    render(<StatementView statement={baseStatement} />);

    // The QR panel repeats the same figure (a parent transfers exactly the
    // total due), so the grand total's verbatim value may legitimately
    // appear more than once — the requirement is that it appears at all.
    const matches = screen.getAllByText(formatMoney(baseStatement.totals.total_due), moneyMatcher);
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("reconciles a partially-paid family with paid and remaining lines, no paid badge", () => {
    const partiallyPaid: Statement = {
      ...baseStatement,
      totals: { ...baseStatement.totals, paid: 1_000_000, outstanding: 3_480_000 },
      qr: { ...baseStatement.qr!, amount: 3_480_000 },
    };
    render(<StatementView statement={partiallyPaid} />);

    // The full charge still headlines; the paid/remaining lines explain why the
    // QR below asks for a smaller figure. Both are server values, not summed.
    expect(screen.getByText(formatMoney(1_000_000), moneyMatcher)).toBeInTheDocument();
    const remaining = screen.getAllByText(formatMoney(3_480_000), moneyMatcher);
    expect(remaining.length).toBeGreaterThanOrEqual(1);
    expect(screen.getByText("Đã thanh toán")).toBeInTheDocument();
    expect(screen.getByText("Còn lại")).toBeInTheDocument();
    // The fully-paid badge must not appear while money is still outstanding.
    expect(screen.queryByText("✓ Đã thanh toán")).not.toBeInTheDocument();
  });
});
