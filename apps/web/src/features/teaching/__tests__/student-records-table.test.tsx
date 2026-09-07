import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";

import {
  StudentRecordsTable,
  type StudentRecordSummary,
} from "../components/student-records-table";

const rows: StudentRecordSummary[] = [
  {
    studentId: "s1",
    name: "Nguyễn Văn An",
    average: 8.8,
    scoreCount: 3,
    trend: { arrow: "↗", label: "Tiến bộ", tone: "up" },
    absences: 0,
  },
  {
    studentId: "s2",
    name: "Trần Thị Bình",
    average: null,
    scoreCount: 0,
    trend: { arrow: "→", label: "Chưa đủ dữ liệu", tone: "flat" },
    absences: 2,
  },
];

describe("StudentRecordsTable", () => {
  it("renders plain names and the six-column header without a query", () => {
    render(<StudentRecordsTable rows={rows} onOpen={() => undefined} />);
    expect(screen.getByText("NGÀY SINH")).toBeInTheDocument();
    const name = screen.getByText("Nguyễn Văn An");
    expect(name.querySelector("mark")).toBeNull();
    expect(within(name.parentElement!).getByText("8.8")).toBeInTheDocument();
    expect(screen.getAllByRole("button", { name: "Xem hồ sơ" })).toHaveLength(2);
  });

  it("marks the folded match inside the name", () => {
    const { container } = render(
      <StudentRecordsTable rows={rows} onOpen={() => undefined} query="nguyen" />,
    );
    const marks = container.querySelectorAll("mark");
    expect(marks).toHaveLength(1);
    expect(marks[0]).toHaveTextContent("Nguyễn");
    expect(marks[0]).toHaveClass("bg-sun-200");
  });

  it("shows the no-match state with a clear action", async () => {
    const user = userEvent.setup();
    const onClearSearch = vi.fn();
    render(
      <StudentRecordsTable
        rows={[]}
        onOpen={() => undefined}
        query="zzz"
        onClearSearch={onClearSearch}
      />,
    );
    expect(screen.getByText("Không tìm thấy học sinh nào khớp “zzz”")).toBeInTheDocument();
    expect(screen.getByText("Thử gõ ít ký tự hơn hoặc kiểm tra lại họ tên.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Xoá tìm kiếm" }));
    expect(onClearSearch).toHaveBeenCalledTimes(1);
  });

  it("renders five hidden shimmer rows and announces the loading label", () => {
    const { container } = render(
      <StudentRecordsTable
        rows={[]}
        onOpen={() => undefined}
        loading
        loadingLabel="Đang tải dữ liệu tháng 8…"
      />,
    );
    expect(screen.getByRole("status")).toHaveTextContent("Đang tải dữ liệu tháng 8…");
    expect(container.querySelectorAll('[aria-hidden="true"]')).toHaveLength(5);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
  });

  it("collapses to two-line rows with a short Xem button in compact mode", async () => {
    const user = userEvent.setup();
    const onOpen = vi.fn();
    render(<StudentRecordsTable rows={rows} onOpen={onOpen} compact query="nguyen" />);
    expect(screen.queryByText("NGÀY SINH")).not.toBeInTheDocument();

    const [first, second] = screen
      .getAllByRole("button", { name: "Xem hồ sơ" })
      .map((button) => button.parentElement!);
    expect(first).toHaveTextContent("Nguyễn Văn An");
    expect(first).toHaveTextContent("TB 8.8 · ↗ Tiến bộ · vắng 0");
    expect(first!.querySelector("mark")).toHaveTextContent("Nguyễn");
    expect(second).toHaveTextContent("TB — · → Chưa đủ dữ liệu · vắng 2");

    const view = within(first!).getByRole("button", { name: "Xem hồ sơ" });
    expect(view).toHaveTextContent("Xem");
    await user.click(view);
    expect(onOpen).toHaveBeenCalledWith("s1");
  });
});
