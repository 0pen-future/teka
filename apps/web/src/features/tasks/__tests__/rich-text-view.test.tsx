import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { RichTextView } from "../components/rich-text-view";

describe("RichTextView", () => {
  it("renders stored HTML through the sanitizer", () => {
    const { container } = render(
      <RichTextView html='<p>Xem <a href="https://teka.vn/x">đây</a><script>alert(1)</script></p>' />,
    );
    expect(container.querySelector("script")).toBeNull();
    expect(screen.getByRole("link", { name: "đây" })).toHaveAttribute("target", "_blank");
  });

  it("keeps the line breaks of a description still stored as plain text", () => {
    const { container } = render(<RichTextView html={"Hỏi lớp 6A & 7B\nbáo lại trước thứ 6"} />);
    expect(container.querySelector(".prose-task")?.innerHTML).toBe(
      "<p>Hỏi lớp 6A &amp; 7B<br>báo lại trước thứ 6</p>",
    );
  });

  it("shows the empty text for a blank or markup-only description", () => {
    render(<RichTextView html="   " emptyText="Không có mô tả" />);
    expect(screen.getByText("Không có mô tả")).toBeInTheDocument();
  });
});
