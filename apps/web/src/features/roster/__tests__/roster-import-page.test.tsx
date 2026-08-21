import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { delay, http, HttpResponse } from "msw";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { API_URL, fail, ok } from "@/test/msw/handlers";
import { server } from "@/test/msw/server";
import { renderWithProviders, signInAs, testPrimaryTeacher } from "@/test/utils";

import { RosterImportPage } from "../pages/roster-import-page";
import {
  makeImportReport,
  makeRowError,
  mockImportHappyPath,
  mockImportTemplate,
  mockRosterImport,
  rowErrorsResponse,
} from "./roster-import-handlers";

/** jsdom has no object-URL implementation; the download path only needs it to exist. */
const createObjectURL = vi.fn(() => "blob:teka-template");
const revokeObjectURL = vi.fn();

beforeEach(() => {
  vi.stubGlobal("URL", Object.assign(URL, { createObjectURL, revokeObjectURL }));
});

afterEach(() => {
  vi.unstubAllGlobals();
  createObjectURL.mockClear();
  revokeObjectURL.mockClear();
});

/** A non-owner's `GET /centers/me` body: no roster, and no role flag at all. */
function signInAsMember() {
  server.use(
    http.get(`${API_URL}/centers/me`, () =>
      HttpResponse.json(ok({ center_name: "Trung Tâm Bình Minh" })),
    ),
  );
}

function renderImportPage() {
  signInAs(testPrimaryTeacher);
  return renderWithProviders(<RosterImportPage />, {
    route: "/students/import",
    path: "/students/import",
    extraRoutes: [{ path: "/students", element: <p>Màn hình lớp và học sinh</p> }],
  });
}

function workbook(name = "roster.xlsx") {
  return new File(["PKfake-xlsx"], name, {
    type: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
  });
}

/** Picks a file and waits for the check button to unlock. */
async function pickWorkbook(user: ReturnType<typeof userEvent.setup>, file = workbook()) {
  await user.upload(await screen.findByLabelText("Chọn file Excel"), file);
  await waitFor(() => expect(screen.getByRole("button", { name: "Kiểm tra" })).toBeEnabled());
}

function checkButton() {
  return screen.getByRole("button", { name: /Kiểm tra/ });
}

function commitButton() {
  return screen.getByRole("button", { name: /Nhập dữ liệu/ });
}

/** A run that finds a small roster: 2 classes, 3 of everything else. */
const smallRoster = makeImportReport({
  classes: { created: 2, reused: 0 },
  schedules: { created: 3, reused: 0 },
  contacts: { created: 3, reused: 0 },
  students: { created: 3, reused: 0 },
  enrollments: { created: 3, reused: 0 },
});

describe("RosterImportPage — role gate", () => {
  it("shows a loading state while the center resolves", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, async () => {
        await delay("infinite");
        return HttpResponse.json(ok({}));
      }),
    );
    renderImportPage();

    expect(await screen.findByText("Đang tải…")).toBeInTheDocument();
  });

  it("reports a center that cannot be loaded", async () => {
    server.use(
      http.get(`${API_URL}/centers/me`, () =>
        HttpResponse.json(fail("INTERNAL_ERROR", "boom"), { status: 500 }),
      ),
    );
    renderImportPage();

    expect(await screen.findByText("Không tải được thông tin trung tâm.")).toBeInTheDocument();
  });

  it("tells a member the import is owner-only and offers no file picker", async () => {
    signInAsMember();
    renderImportPage();

    expect(await screen.findByText(/Chỉ chủ trung tâm mới nhập được/)).toBeInTheDocument();
    expect(screen.queryByLabelText("Chọn file Excel")).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tải file mẫu" })).not.toBeInTheDocument();
  });
});

describe("RosterImportPage — template", () => {
  it("downloads the template and releases the object URL", async () => {
    const user = userEvent.setup();
    mockImportTemplate();
    renderImportPage();

    await user.click(await screen.findByRole("button", { name: "Tải file mẫu" }));

    await waitFor(() => expect(createObjectURL).toHaveBeenCalledTimes(1));
    expect(revokeObjectURL).toHaveBeenCalledWith("blob:teka-template");
  });

  it("surfaces the envelope's message when the download is refused", async () => {
    const user = userEvent.setup();
    // The route answers a blob, so a 403's envelope arrives as binary — the
    // message only survives if the response layer reads it back to JSON.
    mockImportTemplate(() =>
      HttpResponse.json(fail("FORBIDDEN", "chỉ chủ trung tâm được import dữ liệu"), {
        status: 403,
      }),
    );
    renderImportPage();

    await user.click(await screen.findByRole("button", { name: "Tải file mẫu" }));

    expect(await screen.findByText("chỉ chủ trung tâm được import dữ liệu")).toBeInTheDocument();
    expect(createObjectURL).not.toHaveBeenCalled();
  });
});

describe("RosterImportPage — check step", () => {
  it("cannot check or commit before a file is picked", async () => {
    renderImportPage();

    expect(await screen.findByRole("button", { name: "Kiểm tra" })).toBeDisabled();
    expect(commitButton()).toBeDisabled();
  });

  it("checks with dry_run and reports what would be created without committing", async () => {
    const user = userEvent.setup();
    const calls = mockImportHappyPath(smallRoster);
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());

    expect(await screen.findByText("File hợp lệ")).toBeInTheDocument();
    expect(calls).toEqual([{ dryRun: true, hasFile: true }]);
    const summary = screen.getByText("File hợp lệ").closest("div");
    expect(within(summary as HTMLElement).getByText("Lớp")).toBeInTheDocument();
    expect(screen.getByText(/Chưa ghi gì cả/)).toBeInTheDocument();
  });

  it("reports a re-check of an already-imported file as reuse, not creation", async () => {
    const user = userEvent.setup();
    const reused = makeImportReport({
      classes: { created: 0, reused: 2 },
      schedules: { created: 0, reused: 3 },
      contacts: { created: 0, reused: 3 },
      students: { created: 0, reused: 3 },
      enrollments: { created: 0, reused: 3 },
    });
    mockImportHappyPath(reused);
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());

    await screen.findByText("File hợp lệ");
    expect(screen.getAllByText("0")).toHaveLength(5);
    expect(screen.getAllByText("3")).toHaveLength(4);
  });

  it("renders every row defect with its sheet, line and column", async () => {
    const user = userEvent.setup();
    mockRosterImport(() =>
      rowErrorsResponse([
        makeRowError({
          sheet: "Lop",
          line: 3,
          column: "SĐT giáo viên",
          code: "TEACHER_NOT_IN_CENTER",
          message: "số này không thuộc trung tâm của bạn",
        }),
        makeRowError({
          sheet: "HocSinh",
          line: 7,
          column: "Tên lớp",
          code: "CLASS_NOT_IN_FILE",
          message: "lớp này không có trong sheet Lop",
        }),
      ]),
    );
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());

    const table = await screen.findByRole("table");
    const rows = within(table)
      .getAllByRole("row")
      .slice(1)
      .map((row) =>
        within(row)
          .getAllByRole("cell")
          .map((cell) => cell.textContent),
      );
    expect(rows).toEqual([
      ["Lop", "3", "SĐT giáo viên", "số này không thuộc trung tâm của bạn"],
      ["HocSinh", "7", "Tên lớp", "lớp này không có trong sheet Lop"],
    ]);
    // Nothing was written, so the commit must stay locked.
    expect(commitButton()).toBeDisabled();
    expect(screen.queryByText("File hợp lệ")).not.toBeInTheDocument();
  });

  it("says how many defects were left out when the list is capped", async () => {
    const user = userEvent.setup();
    mockRosterImport(() => rowErrorsResponse([makeRowError()], 42));
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());

    expect(await screen.findByText(/Còn 42 lỗi nữa/)).toBeInTheDocument();
  });

  it("falls back to the envelope message for a failure that carries no row list", async () => {
    const user = userEvent.setup();
    mockRosterImport(() =>
      HttpResponse.json(fail("CONFLICT", "đang có một lần import khác chạy cho trung tâm này"), {
        status: 409,
      }),
    );
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());

    expect(
      await screen.findByText("đang có một lần import khác chạy cho trung tâm này"),
    ).toBeInTheDocument();
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    expect(commitButton()).toBeDisabled();
  });
});

describe("RosterImportPage — commit step", () => {
  it("commits only after a clean check and reports what was written", async () => {
    const user = userEvent.setup();
    const calls = mockImportHappyPath(smallRoster);
    renderImportPage();
    await pickWorkbook(user);

    expect(commitButton()).toBeDisabled();
    await user.click(checkButton());
    await screen.findByText("File hợp lệ");
    expect(commitButton()).toBeEnabled();

    await user.click(commitButton());

    expect(await screen.findByText("Đã nhập xong")).toBeInTheDocument();
    expect(calls).toEqual([
      { dryRun: true, hasFile: true },
      { dryRun: false, hasFile: true },
    ]);
    // Re-committing the same file would only report reuse; the button locks.
    expect(commitButton()).toBeDisabled();
    expect(screen.getByRole("button", { name: "Xem lớp & học sinh" })).toBeInTheDocument();
  });

  it("keeps the commit button disabled while the import is in flight", async () => {
    const user = userEvent.setup();
    mockRosterImport(async (call) => {
      if (call.dryRun) {
        return HttpResponse.json(ok(smallRoster));
      }
      await delay(50);
      return HttpResponse.json(ok({ ...smallRoster, committed: true }));
    });
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());
    await screen.findByText("File hợp lệ");
    await user.click(commitButton());

    expect(await screen.findByRole("button", { name: "Đang nhập dữ liệu…" })).toBeDisabled();
    expect(checkButton()).toBeDisabled();
    expect(screen.getByText(/đừng đóng trang/)).toBeInTheDocument();
    await screen.findByText("Đã nhập xong");
  });

  it("drops the check result when the commit fails, forcing a re-check", async () => {
    const user = userEvent.setup();
    mockRosterImport((call) =>
      call.dryRun
        ? HttpResponse.json(ok(smallRoster))
        : rowErrorsResponse([
            makeRowError({
              code: "CLASS_EXISTS_MISMATCH",
              message:
                'lớp "Toán 9A" đã có trong hệ thống với Đơn giá/buổi là 150000, file ghi 160000',
            }),
          ]),
    );
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());
    await screen.findByText("File hợp lệ");
    await user.click(commitButton());

    expect(await screen.findByRole("table")).toBeInTheDocument();
    expect(screen.queryByText("File hợp lệ")).not.toBeInTheDocument();
    expect(commitButton()).toBeDisabled();
  });

  it("clears a previous report when a different file is picked", async () => {
    const user = userEvent.setup();
    mockImportHappyPath(smallRoster);
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());
    await screen.findByText("File hợp lệ");

    // A "hợp lệ" badge next to an unchecked file would invite committing it.
    await user.upload(screen.getByLabelText("Chọn file Excel"), workbook("roster-sua.xlsx"));

    await waitFor(() => expect(screen.queryByText("File hợp lệ")).not.toBeInTheDocument());
    expect(commitButton()).toBeDisabled();
  });

  it("resets the whole flow when the operator starts over", async () => {
    const user = userEvent.setup();
    mockImportHappyPath(smallRoster);
    renderImportPage();
    await pickWorkbook(user);

    await user.click(checkButton());
    await screen.findByText("File hợp lệ");
    await user.click(screen.getByRole("button", { name: "Chọn file khác" }));

    expect(screen.queryByText("File hợp lệ")).not.toBeInTheDocument();
    expect(checkButton()).toBeDisabled();
    expect(commitButton()).toBeDisabled();
  });
});
