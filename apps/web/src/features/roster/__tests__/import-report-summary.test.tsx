import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { ImportReportSummary } from "../components/import-report-summary";
import type { ImportReport } from "../schemas/import-schemas";

const ZERO = { created: 0, reused: 0 };

function report(overrides: Partial<ImportReport> = {}): ImportReport {
  return {
    committed: false,
    classes: ZERO,
    schedules: ZERO,
    contacts: ZERO,
    students: ZERO,
    enrollments: ZERO,
    ...overrides,
  };
}

describe("ImportReportSummary", () => {
  it("shows the success header once a commit created rows", () => {
    render(<ImportReportSummary report={report({ committed: true, classes: { created: 2, reused: 0 } })} />);
    expect(screen.getByText("Đã nhập xong")).toBeInTheDocument();
  });

  it("shows the valid-file header for a check that reuses existing rows", () => {
    // A re-import touches nothing new but still has data rows: reused counts
    // keep it out of the empty-file branch.
    render(<ImportReportSummary report={report({ classes: { created: 0, reused: 2 } })} />);
    expect(screen.getByText("File hợp lệ")).toBeInTheDocument();
  });

  it("warns instead of vouching for an all-zero report", () => {
    render(<ImportReportSummary report={report({ committed: true })} />);
    expect(screen.queryByText("Đã nhập xong")).not.toBeInTheDocument();
    expect(screen.getByText(/không có dòng dữ liệu nào/i)).toBeInTheDocument();
  });
});
