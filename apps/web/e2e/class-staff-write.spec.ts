import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// Cô Lan owns the center and teaches "Toán 8 - Tối Thứ Ba", staffed with
// Cô Thu as hoc_vu and Thầy Minh as tro_giang. Thầy Minh also teaches his own
// "Lý 7 - Chiều Thứ Năm" — the class handed off (and handed back) below.
//
// These journeys need the earlier specs' state: billing.spec confirms Cô
// Lan's pending sessions and closes her current period, which is what puts
// invoice lines behind "Toán 8 - Tối Thứ Ba" and makes the class period
// discoverable/sendable. A partial run (--grep) on a freshly seeded DB will
// find no closed period.
const STAFF_CLASS = "Toán 8 - Tối Thứ Ba";
const HANDOFF_CLASS = "Lý 7 - Chiều Thứ Năm";
const OWNER = { phone: "0901000001", password: "lan-password", name: "Cô Lan" };
const TRO_GIANG = { phone: "0901000002", password: "minh-password", name: "Thầy Minh" };
const HOC_VU = { phone: "0901000003", password: "thu-password", name: "Cô Thu" };

async function login(page: Page, user: { phone: string; password: string; name: string }) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(user.phone);
  await page.getByLabel("Mật khẩu").fill(user.password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    page.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${user.name}!`)),
  ).toBeVisible();
}

/** Resolves a class id by opening its roster tab and reading the URL. */
async function classIdFromRosterTab(page: Page, className: string): Promise<string> {
  await page.goto("/students");
  await page.getByRole("tab", { name: className }).click();
  await expect(page).toHaveURL(/class_id=(?!none)/);
  const classId = new URL(page.url()).searchParams.get("class_id");
  expect(classId).toBeTruthy();
  return classId ?? "";
}

/**
 * Opens the attendance sheet of a past session of the named class and returns
 * its URL. A cancelled session renders no sheet, so pick a row whose status
 * mentions điểm danh (past session, confirmed or not).
 */
async function openAttendanceSheet(page: Page, className: string, rowText: RegExp) {
  await page.goto("/sessions");
  await page.getByRole("tab", { name: className }).click();
  const from = new Date();
  from.setDate(from.getDate() - 90);
  await page.getByLabel("Từ").fill(from.toISOString().slice(0, 10));
  const sessionRow = page.getByRole("link", { name: rowText }).filter({ hasText: /điểm danh/ });
  await expect(sessionRow.first()).toBeVisible();
  await sessionRow.first().click();
  await expect(page).toHaveURL(/\/sessions\/.+\/attendance$/);
  await expect(page.locator("button[aria-pressed]").first()).toBeVisible();
  return page.url();
}

/**
 * Flips the first roster row and saves the sheet. The live confirm button is
 * the one carrying a "· N vắng" count — the settled "ĐÃ XÁC NHẬN ✓" state and
 * the frozen role-gate label never match it. A successful save toasts the
 * tally and navigates back to /sessions.
 */
async function flipFirstRowAndConfirm(page: Page) {
  const firstRow = page.locator("button[aria-pressed]").first();
  const before = await firstRow.getAttribute("aria-pressed");
  await firstRow.click();
  await expect(firstRow).toHaveAttribute("aria-pressed", before === "true" ? "false" : "true");
  await page
    .getByRole("button", { name: /(XÁC NHẬN BUỔI HỌC|LƯU VÀ TẠO ĐIỀU CHỈNH) · \d+ vắng/ })
    .click();
  await expect(page.getByText(/Đã điểm danh \d+ có mặt, \d+ vắng/)).toBeVisible();
  await expect(page).toHaveURL(/\/sessions$/);
}

// Captured before the tro_giang flip below so the restoring afterEach can put
// the row back even when the journey dies between the flip and its save.
let flipRestore: { sheetUrl: string; pressed: string | null } | null = null;

// Restore the seeded roster after the flip journey: reopen the sheet and, if
// the first row no longer matches its captured state, flip it back and save.
// The check makes it idempotent, so it is safe no matter where the journey
// stopped. Attendance status and the period's totals return to the seeded
// values; on a closed period the two saves do each leave a pair of mutually
// cancelling adjustment lines behind — the stack is restored in balances, not
// in row count.
test.afterEach(async ({ browser }, testInfo) => {
  if (!testInfo.title.includes("records attendance") || !flipRestore) {
    return;
  }
  const { sheetUrl, pressed } = flipRestore;
  flipRestore = null;
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await login(page, TRO_GIANG);
    await page.goto(sheetUrl);
    const firstRow = page.locator("button[aria-pressed]").first();
    await expect(firstRow).toBeVisible();
    if ((await firstRow.getAttribute("aria-pressed")) !== pressed) {
      await flipFirstRowAndConfirm(page);
    }
  } finally {
    await context.close();
  }
});

test("tro_giang records attendance directly on the staffed class", async ({ page }) => {
  await login(page, TRO_GIANG);

  // Thầy Minh holds no giao_vien stint on Toán 8 — this save goes through the
  // tro_giang attendance capability, straight to the server, no approval step.
  const sheetUrl = await openAttendanceSheet(page, STAFF_CLASS, /Toán 8/);
  flipRestore = {
    sheetUrl,
    pressed: await page.locator("button[aria-pressed]").first().getAttribute("aria-pressed"),
  };
  await flipFirstRowAndConfirm(page);
});

test("hoc_vu discovers the class period from the roster and sends the class copies", async ({
  page,
}) => {
  await login(page, HOC_VU);

  // Entry point: the roster screen of the staffed class offers "Gửi báo cáo"
  // to hoc_vu (Cô Thu holds no center-wide send grant — this is the class
  // role's own path).
  await classIdFromRosterTab(page, STAFF_CLASS);
  await page.getByRole("button", { name: "Gửi báo cáo" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByText(`Gửi báo cáo — lớp ${STAFF_CLASS}`)).toBeVisible();

  // Period discovery: the class bills under Cô Lan's period, which Cô Thu
  // could never list center-wide — the dialog resolves it through the class's
  // invoice lines. The ledger fetch swaps the generate button between the
  // header and the empty-state card, so wait for it before clicking.
  const ledgerLoaded = page.waitForResponse(
    (response) =>
      response.url().includes("/notifications?") && response.request().method() === "GET",
  );
  await dialog
    .getByRole("link", { name: /^Gửi báo cáo lớp/ })
    .first()
    .click();
  await expect(page).toHaveURL(/\/notifications\/.+\?.*class_id=/);
  await expect(
    page.getByRole("heading", { name: `Gửi thông báo — lớp ${STAFF_CLASS}` }),
  ).toBeVisible();
  await ledgerLoaded;

  // No Zalo session is linked in e2e, so the manual channel is the default
  // and generating renders one copy-paste card per class contact. Bé An and
  // Bé Bình (Chị Hoa's children) are the class's active enrollments.
  await expect(page.getByRole("radio", { name: "Zalo thủ công" })).toBeChecked();
  await page.getByRole("button", { name: /Tạo (lại|thông báo học phí)/ }).click();
  await expect(page.getByText("Chị Hoa").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Sao chép", exact: true }).first()).toBeVisible();
});

/**
 * Reads the class-settings handoff card and, when the current teacher differs
 * from `targetName`, hands the class over. Assert-then-set keeps it
 * idempotent on a reused database and safe to run from the restoring
 * afterEach no matter where the journey stopped.
 */
async function ensureClassTeacher(
  page: Page,
  classId: string,
  targetName: string,
  targetOptionLabel: string,
) {
  await page.goto(`/classes/${classId}/settings`);
  const card = page.locator("#teacher-handoff");
  const current = card.getByText("Giáo viên hiện tại:");
  await expect(current).toBeVisible();
  if ((await current.innerText()).includes(targetName)) {
    return;
  }
  await card.getByLabel("Bàn giao cho").selectOption({ label: targetOptionLabel });
  await card.getByRole("button", { name: "Bàn giao lớp", exact: true }).click();
  await card.getByRole("button", { name: "Xác nhận bàn giao" }).click();
  await expect(page.getByText(new RegExp(`Đã bàn giao lớp cho ${targetName}`))).toBeVisible();
}

// Hand Lý 7 back to Thầy Minh for the following specs (and the next run), no
// matter where the handoff journey stopped.
test.afterEach(async ({ browser }, testInfo) => {
  if (!testInfo.title.includes("handed-off")) {
    return;
  }
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await login(page, OWNER);
    const classId = await classIdFromRosterTab(page, HANDOFF_CLASS);
    await ensureClassTeacher(page, classId, TRO_GIANG.name, TRO_GIANG.name);
  } finally {
    await context.close();
  }
});

test("a handed-off teacher keeps reading history but loses every write", async ({ browser }) => {
  // Capture Thầy Minh's ids and a past sheet while he still teaches Lý 7 —
  // and prove the sheet is live for him now, so the freeze below is the
  // handoff's doing.
  const minhContext = await browser.newContext();
  const minh = await minhContext.newPage();
  await login(minh, TRO_GIANG);
  const classId = await classIdFromRosterTab(minh, HANDOFF_CLASS);
  const sheetUrl = await openAttendanceSheet(minh, HANDOFF_CLASS, /Lý 7/);
  await expect(
    minh.getByRole("button", { name: /XÁC NHẬN BUỔI HỌC|ĐÃ XÁC NHẬN|LƯU VÀ TẠO ĐIỀU CHỈNH/ }),
  ).toBeEnabled();

  // The owner hands the class to herself through the settings card.
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  await login(owner, OWNER);
  await ensureClassTeacher(owner, classId, OWNER.name, `${OWNER.name} (chủ trung tâm)`);

  // History reads survive the handoff: the roster he taught stays visible.
  await minh.goto(`/students?class_id=${classId}`);
  await expect(minh.getByRole("row").filter({ hasText: "Bé Phúc" })).toBeVisible();

  // Every write freezes — settings save is reserved for the new teacher…
  await minh.goto(`/classes/${classId}/settings`);
  await expect(
    minh.getByText("Chỉ giáo viên phụ trách hoặc chủ trung tâm mới sửa được cài đặt lớp."),
  ).toBeVisible();
  await expect(minh.getByRole("button", { name: "Lưu thay đổi" })).toBeDisabled();

  // …and so is attendance, even on the past session he himself recorded.
  await minh.goto(sheetUrl);
  await expect(minh.locator("button[aria-pressed]").first()).toBeVisible();
  const frozen = minh.getByRole("button", {
    name: "CHỈ GIÁO VIÊN, TRỢ GIẢNG LỚP HOẶC CHỦ TRUNG TÂM MỚI XÁC NHẬN ĐƯỢC",
  });
  await expect(frozen).toBeVisible();
  await expect(frozen).toBeDisabled();
  await expect(minh.getByRole("button", { name: "Huỷ buổi học" })).toHaveCount(0);

  await minhContext.close();
  await ownerContext.close();
});
