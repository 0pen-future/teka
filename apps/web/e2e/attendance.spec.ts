import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
}

test("records late/excused marks, confirms, then reopens and edits to absent", async ({ page }) => {
  await login(page);

  // The seeder leaves the two most recent past sessions unconfirmed
  // (`apps/api/seeds/seed.go`, `pendingAttendanceCount`) — jump straight into
  // one from the dashboard's pending-attendance alert rather than hand-rolling
  // a session first.
  await expect(page.getByText("buổi đã dạy nhưng chưa điểm danh")).toBeVisible();
  await page.getByRole("link", { name: "Điểm danh ngay" }).first().click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
  const sheetUrl = page.url();

  // Everyone renders Đúng giờ by default — each student is a radiogroup of
  // the four statuses, purely local until the single confirm.
  const rows = page.getByRole("radiogroup");
  await expect(rows.first()).toBeVisible();
  const rowCount = await rows.count();
  expect(rowCount).toBeGreaterThanOrEqual(2);
  await expect(rows.nth(0).getByRole("radio", { name: "Đúng giờ" })).toHaveAttribute(
    "aria-checked",
    "true",
  );

  // Mark one student late and one excused-with-note; the note becomes the
  // "Vắng có phép" subtitle and the excused mark stays out of the button's
  // VẮNG/MUỘN tally.
  await rows.nth(0).getByRole("radio", { name: "Muộn" }).click();
  await rows.nth(1).getByRole("radio", { name: "Có lý do" }).click();
  await page.getByRole("textbox", { name: /^Lý do của/ }).fill("mẹ báo ốm");
  await expect(page.getByText(/Vắng có phép — mẹ báo ốm/)).toBeVisible();

  await page.getByRole("button", { name: /^XÁC NHẬN · 1 MUỘN$/ }).click();
  await expect(page.getByText(/Đã điểm danh .*1 muộn.*1 có lý do/)).toBeVisible();
  await expect(page).toHaveURL(/\/sessions$/);

  // Reopen: the saved four-status sheet comes back, note included, and the
  // bar reports the settled state instead of offering a save.
  await page.goto(sheetUrl);
  await expect(rows.nth(0).getByRole("radio", { name: "Muộn" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await expect(rows.nth(1).getByRole("radio", { name: "Có lý do" })).toHaveAttribute(
    "aria-checked",
    "true",
  );
  await expect(page.getByText(/Vắng có phép — mẹ báo ốm/)).toBeVisible();
  await expect(page.getByRole("button", { name: /ĐÃ XÁC NHẬN/ })).toBeVisible();

  // Edit after confirm: switch the late student to absent and save again.
  await rows.nth(0).getByRole("radio", { name: "Vắng" }).click();
  await page.getByRole("button", { name: /^XÁC NHẬN · 1 VẮNG$/ }).click();
  await expect(page.getByText(/Đã điểm danh .*1 vắng/)).toBeVisible();
  await expect(page).toHaveURL(/\/sessions$/);

  // The dashboard's pending count reflects the just-confirmed session.
  await page.goto("/");
  const pendingBanner = page.getByText(/buổi đã dạy nhưng chưa điểm danh/);
  if (await pendingBanner.isVisible().catch(() => false)) {
    await expect(pendingBanner).toContainText("1 buổi đã dạy nhưng chưa điểm danh");
  } else {
    await expect(pendingBanner).not.toBeVisible();
  }
});

test("moves between sessions with the trio arrows and the month calendar", async ({ page }) => {
  await login(page);
  await page.goto("/sessions");

  // The trio picker anchors on today's session (or the nearest upcoming one)
  // without any date filtering; ‹ steps the anchor back one session.
  await expect(page.getByText(/HÔM NAY|ĐANG XEM/)).toBeVisible();
  await page.getByRole("button", { name: "Buổi trước" }).click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);

  // The month calendar is the long-jump shortcut: any dotted day navigates
  // straight to that day's session.
  await page.getByRole("button", { name: "Mở lịch tháng" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(/Tháng \d+\/\d{4}/)).toBeVisible();
  const sessionDay = dialog.getByRole("button", { name: /, \d{2}\/\d{2}$/ }).first();
  await expect(sessionDay).toBeVisible();
  await sessionDay.click();
  await expect(dialog).not.toBeVisible();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
});

test("cancelling a session takes a reason and bills nobody", async ({ page }) => {
  await login(page);

  // Cancel an upcoming session rather than a pending one: the pending feed is
  // shared suite state the first test already touched, while the weekly
  // schedule always materializes future sessions — the trio's "Sắp tới" card
  // reaches one without any date filtering.
  await page.goto("/sessions");
  const upcoming = page.getByRole("link").filter({ hasText: "Sắp tới" }).first();
  await expect(upcoming).toBeVisible();
  await upcoming.click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
  const sessionUrl = page.url();

  await page.getByRole("button", { name: "Huỷ buổi học" }).click();
  await page.getByLabel("Lý do huỷ").fill("Nghỉ lễ Quốc khánh");
  await page.getByRole("button", { name: "Xác nhận huỷ" }).click();

  // Cancelling navigates back to the session list; the cancelled state lives
  // on the session page itself, so revisit the saved URL to assert it.
  await expect(page).toHaveURL(/\/sessions$/);
  await page.goto(sessionUrl);
  await expect(page.getByText("Buổi học đã huỷ")).toBeVisible();
  await expect(page.getByText("Nghỉ lễ Quốc khánh")).toBeVisible();
  await expect(page.getByText("Không tính tiền cho học sinh nào.")).toBeVisible();
});
