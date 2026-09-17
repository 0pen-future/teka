import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import { TaskDescriptionEditor } from "../components/task-description-editor";

function Harness({
  initial = "",
  onChange = () => undefined,
  invalid = false,
  describedBy,
}: {
  initial?: string;
  onChange?: (html: string) => void;
  invalid?: boolean;
  describedBy?: string;
}) {
  const [value, setValue] = useState(initial);
  return (
    <>
      <span id="desc-label">Mô tả</span>
      <TaskDescriptionEditor
        id="desc"
        value={value}
        onChange={(html) => {
          setValue(html);
          onChange(html);
        }}
        labelId="desc-label"
        invalid={invalid}
        describedBy={describedBy}
      />
      <button type="button" onClick={() => setValue("<p>từ ngoài</p>")}>
        Đặt lại
      </button>
    </>
  );
}

describe("TaskDescriptionEditor", () => {
  it("exposes an accessible multiline textbox named by the label with a toolbar", async () => {
    render(<Harness invalid describedBy="desc-error" />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });
    expect(textbox).toHaveAttribute("aria-multiline", "true");
    expect(textbox).toHaveAttribute("aria-invalid", "true");
    expect(textbox).toHaveAttribute("aria-describedby", "desc-error");
    const toolbar = screen.getByRole("toolbar", { name: "Định dạng mô tả" });
    expect(
      screen
        .getAllByRole("button", { pressed: false })
        .map((button) => button.getAttribute("aria-label")),
    ).toEqual([
      "Đậm",
      "Nghiêng",
      "Gạch chân",
      "Gạch ngang",
      "Danh sách chấm",
      "Danh sách số",
      "Liên kết",
    ]);
    expect(toolbar).toContainElement(screen.getByRole("button", { name: "Đậm" }));
  });

  it("seeds the document from `value` and reflects marks in the toolbar's pressed state", async () => {
    render(<Harness initial="<p><strong>đậm</strong> thường</p>" />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });
    expect(textbox.querySelector("strong")).toHaveTextContent("đậm");
    expect(screen.getByText("2000", { exact: false })).toHaveTextContent("10/2000");
  });

  it("emits the HTML of what is typed and an empty string once the document is emptied", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });

    await user.click(textbox);
    await user.keyboard("xin chào");
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("<p>xin chào</p>"));

    await user.keyboard("{Control>}a{/Control}{Backspace}");
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith(""));
  });

  it("applies bold from the toolbar to the typed text", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });

    await user.click(textbox);
    await user.click(screen.getByRole("button", { name: "Đậm" }));
    expect(screen.getByRole("button", { name: "Đậm" })).toHaveAttribute("aria-pressed", "true");
    await user.keyboard("nặng");
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith("<p><strong>nặng</strong></p>"));
  });

  it("inserts a link from the URL panel and refuses non-http(s)/mailto schemes", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    await screen.findByRole("textbox", { name: "Mô tả" });

    await user.click(screen.getByRole("button", { name: "Liên kết" }));
    const urlInput = screen.getByRole("textbox", { name: "Địa chỉ liên kết" });
    await user.type(urlInput, "javascript:alert(1)");
    await user.click(screen.getByRole("button", { name: "Áp dụng" }));
    expect(screen.getByRole("alert")).toHaveTextContent(
      "Liên kết phải bắt đầu bằng http://, https:// hoặc mailto:",
    );
    expect(onChange).not.toHaveBeenCalled();

    await user.clear(urlInput);
    await user.type(urlInput, "https://teka.vn{Enter}");
    await waitFor(() =>
      expect(onChange).toHaveBeenLastCalledWith(
        '<p><a target="_blank" rel="noopener noreferrer nofollow" href="https://teka.vn">https://teka.vn</a></p>',
      ),
    );
    expect(screen.queryByRole("textbox", { name: "Địa chỉ liên kết" })).not.toBeInTheDocument();
  });

  it("follows an external `value` change without echoing it back", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Harness initial="<p>ban đầu</p>" onChange={onChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });

    await user.click(screen.getByRole("button", { name: "Đặt lại" }));
    await waitFor(() => expect(textbox).toHaveTextContent("từ ngoài"));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("keeps one toolbar button in the tab order and moves between them with the arrow keys", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    await screen.findByRole("textbox", { name: "Mô tả" });
    const toolbar = screen.getByRole("toolbar", { name: "Định dạng mô tả" });
    const tabbable = () =>
      within(toolbar)
        .getAllByRole("button")
        .filter((button) => button.tabIndex === 0)
        .map((button) => button.getAttribute("aria-label"));
    expect(tabbable()).toEqual(["Đậm"]);

    screen.getByRole("button", { name: "Đậm" }).focus();
    await user.keyboard("{ArrowRight}");
    expect(screen.getByRole("button", { name: "Nghiêng" })).toHaveFocus();
    await user.keyboard("{ArrowLeft}{ArrowLeft}");
    expect(screen.getByRole("button", { name: "Liên kết" })).toHaveFocus();
    expect(tabbable()).toEqual(["Liên kết"]);

    // TipTap defers focus to the next animation frame.
    await user.keyboard("{Escape}");
    await waitFor(() => expect(screen.getByRole("textbox", { name: "Mô tả" })).toHaveFocus());
  });

  it("maps pasted HTML onto the allowed subset", async () => {
    const onChange = vi.fn();
    render(<Harness onChange={onChange} />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });

    const html =
      '<h1>Tiêu đề</h1><p style="color:red">có <span class="x">span</span> và <img src="x" onerror="alert(1)"></p><table><tr><td>a</td><td>b</td></tr></table><script>alert(2)</script><p><a href="javascript:alert(3)">xấu</a> <a href="https://teka.vn">tốt</a></p>';
    fireEvent.paste(textbox, {
      clipboardData: {
        types: ["text/html", "text/plain"],
        getData: (type: string) => (type === "text/html" ? html : "Tiêu đề"),
      },
    });

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    const result = onChange.mock.lastCall?.[0] as string;
    expect(result).not.toMatch(/<(h1|span|img|table|tr|td|script)\b/);
    expect(result).not.toContain("style=");
    expect(result).not.toContain("javascript:");
    expect(result).toContain("<p>Tiêu đề</p>");
    expect(result).toContain('href="https://teka.vn"');
  });

  it("returns focus to the text when the link panel closes with Escape", async () => {
    const user = userEvent.setup();
    render(<Harness />);
    const textbox = await screen.findByRole("textbox", { name: "Mô tả" });

    await user.click(screen.getByRole("button", { name: "Liên kết" }));
    const urlInput = screen.getByRole("textbox", { name: "Địa chỉ liên kết" });
    expect(urlInput).toHaveFocus();
    await user.keyboard("{Escape}");
    expect(urlInput).not.toBeInTheDocument();
    await waitFor(() => expect(textbox).toHaveFocus());
  });

  it("announces only the crossing of the limit, not every keystroke", async () => {
    render(<Harness initial={"<p>" + "a".repeat(2001) + "</p>"} />);
    await screen.findByRole("textbox", { name: "Mô tả" });
    expect(screen.getByText("2001/2000")).not.toHaveAttribute("aria-live");
    expect(screen.getByRole("status")).toHaveTextContent("Mô tả vượt quá 2000 ký tự");
  });
});
