import { expect, test, type Browser, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// The first seeded teacher owns the center; Thầy Minh is a teaching member
// with his own class and an open billing period; Cô Thu is a member with no
// teaching data — the delegated-send grantee.
const OWNER_PHONE = "0901000001";
const OWNER_PASSWORD = "lan-password";
const TEACHER_PHONE = "0901000002";
const TEACHER_PASSWORD = "minh-password";
const SECRETARY_PHONE = "0901000003";
const SECRETARY_PASSWORD = "thu-password";

async function login(page: Page, phone: string, password: string, displayName: string) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(phone);
  await page.getByLabel("Mật khẩu").fill(password);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(
    page.getByText(new RegExp(`Chào buổi (sáng|trưa|chiều|tối), ${displayName}!`)),
  ).toBeVisible();
}

/**
 * The e2e database is reused between runs, so the grant must be
 * assert-then-set: the member permissions dialog shows the current
 * `reports.send` override, and only a differing state is saved. Returns
 * after the roster badge reflects the requested state.
 */
async function setSendReportsGrant(page: Page, granted: boolean) {
  await page.goto("/center");
  await page.getByRole("button", { name: "Phân quyền cho Cô Thu" }).click();
  const dialog = page.getByRole("dialog");
  const reportsSend = dialog.getByRole("combobox", { name: "Quyền Gửi báo cáo học phí" });
  await expect(reportsSend).toBeVisible();

  const target = granted ? "grant" : "inherit";
  if ((await reportsSend.inputValue()) === target) {
    await dialog.getByRole("button", { name: "Đóng", exact: true }).click();
  } else {
    await reportsSend.selectOption(target);
    await dialog.getByRole("button", { name: "Lưu" }).click();
    await expect(page.getByText("Đã lưu phân quyền")).toBeVisible();
  }
  await expect(dialog).toBeHidden();
  await expect(page.getByText("Thư ký gửi báo cáo")).toHaveCount(granted ? 1 : 0);
}

async function revokeGrant(browser: Browser) {
  const context = await browser.newContext();
  const page = await context.newPage();
  try {
    await login(page, OWNER_PHONE, OWNER_PASSWORD, "Cô Lan");
    await setSendReportsGrant(page, false);
  } finally {
    await context.close();
  }
}

// Leave the flag false for the next run and for unrelated specs, no matter
// where the journey test stopped.
test.afterEach(async ({ browser }) => {
  await revokeGrant(browser);
});

test("owner grants send-reports; secretary sends another teacher's period; audit attributes her", async ({
  browser,
}) => {
  // — Owner grants through the UI (exercises the phase-3 management surface).
  const ownerContext = await browser.newContext();
  const owner = await ownerContext.newPage();
  await login(owner, OWNER_PHONE, OWNER_PASSWORD, "Cô Lan");
  await setSendReportsGrant(owner, true);

  // — Secretary journey in her own session.
  const secretaryContext = await browser.newContext();
  const secretary = await secretaryContext.newPage();
  await login(secretary, SECRETARY_PHONE, SECRETARY_PASSWORD, "Cô Thu");

  // The flag unlocks exactly one nav entry; the owner-only surfaces stay out
  // of reach for her.
  await expect(secretary.getByRole("link", { name: "Gửi báo cáo" }).first()).toBeVisible();
  await expect(secretary.getByRole("link", { name: "Duyệt giáo án" })).toHaveCount(0);
  await expect(secretary.getByRole("link", { name: "Nhập từ Excel" })).toHaveCount(0);
  await expect(secretary.getByRole("link", { name: "Nhật ký hoạt động" })).toHaveCount(0);

  await secretary.goto("/reports");
  await expect(secretary.getByRole("heading", { name: "Gửi báo cáo" })).toBeVisible();

  // Center-wide read: Thầy Minh's seeded open period is listed under his name
  // even though the secretary teaches nothing. Newest period first.
  await expect(secretary.getByText("Thầy Minh")).toBeVisible();
  // The generate button swaps between the header and the empty-state card
  // depending on whether the ledger came back empty; wait for the ledger
  // fetch so the click lands on the settled button.
  const ledgerLoaded = secretary.waitForResponse(
    (response) =>
      response.url().includes("/notifications?") && response.request().method() === "GET",
  );
  await secretary
    .getByRole("link", { name: /^Gửi báo cáo tháng \d+\/\d+ của Thầy Minh$/ })
    .first()
    .click();
  await expect(secretary).toHaveURL(/\/notifications\/.+$/);
  const periodId = new URL(secretary.url()).pathname.split("/").pop() ?? "";
  await ledgerLoaded;

  // Manual channel is the default (no Zalo session is linked in e2e), and the
  // full send UI is present for the flag holder.
  await expect(secretary.getByRole("radio", { name: "Zalo thủ công" })).toBeChecked();
  const generateButton = secretary.getByRole("button", { name: "Tạo thông báo học phí" });
  await expect(generateButton).toBeVisible();
  await generateButton.click();

  // One copy-paste card per contact of Minh's roster, rendered from the bulk
  // response — the delegated send actually produced messages. `.first()`
  // because a reused DB's ledger can carry the same names from earlier runs.
  await expect(secretary.getByText("Chị Yến").first()).toBeVisible();
  await expect(secretary.getByText("Anh Sơn").first()).toBeVisible();
  await expect(secretary.getByRole("button", { name: "Sao chép", exact: true })).toHaveCount(2);

  // — Audit attribution: the owner sees the grant and the bulk send, with the
  // secretary as the bulk send's actor. Capture is an async batched flush, so
  // reload until the row lands instead of sleeping.
  await owner.getByRole("link", { name: "Nhật ký hoạt động" }).click();
  await expect(owner).toHaveURL(/\/audit$/);
  await expect(async () => {
    await owner.reload();
    await expect(
      owner.getByRole("row").filter({ hasText: "notification.bulk_send" }).first(),
    ).toBeVisible({ timeout: 2_000 });
  }).toPass({ timeout: 10_000 });

  const bulkRow = owner.getByRole("row").filter({ hasText: "notification.bulk_send" }).first();
  await expect(bulkRow.getByText("Cô Thu", { exact: true })).toBeVisible();
  // The expanded request line carries this run's period id, pinning the row
  // to the send performed above rather than a leftover from a reused DB.
  await bulkRow.getByRole("button", { name: "Chi tiết notification.bulk_send" }).click();
  await expect(
    owner.getByText(`POST /api/v1/billing-periods/${periodId}/notifications/bulk`),
  ).toBeVisible();

  // The grant itself is captured too (the override save is the audited write).
  await expect(
    owner.getByRole("row").filter({ hasText: "center.member.overrides_update" }).first(),
  ).toBeVisible();

  await secretaryContext.close();
  await ownerContext.close();
});

test("plain teacher keeps a read-only ledger and no send entry points (D8)", async ({ page }) => {
  await login(page, TEACHER_PHONE, TEACHER_PASSWORD, "Thầy Minh");

  // No delegated-send entry: the nav link belongs to flag holders only.
  await expect(page.getByRole("link", { name: "Gửi báo cáo" })).toHaveCount(0);

  // His own period's review (seeded closed) never offers a send link.
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/.+$/);
  const periodId = new URL(page.url()).pathname.split("/").pop() ?? "";
  await expect(page.getByRole("link", { name: "Gửi thông báo →" })).toHaveCount(0);

  // The notifications page for his own period is a send-control-free ledger:
  // he sees what the secretary sent for him, but cannot create sends.
  await page.goto(`/notifications/${periodId}`);
  await expect(
    page.getByText(/Việc gửi báo cáo do người được giao quyền hoặc chủ trung tâm/),
  ).toBeVisible();
  // The previous test's delegated send left visible ledger rows. This spec
  // runs single-worker in file order, so a full run always has them; a
  // partial run (--grep D8) on a freshly seeded DB will not.
  await expect(page.getByText("Zalo thủ công").first()).toBeVisible();
  await expect(page.getByText("Chị Yến").first()).toBeVisible();
  await expect(page.getByRole("button", { name: "Tạo thông báo học phí" })).toHaveCount(0);
  await expect(page.getByRole("button", { name: "Sao chép tất cả chưa gửi" })).toHaveCount(0);
  await expect(page.getByRole("radio")).toHaveCount(0);

  // Collections for the same period: payment collection stays his work,
  // reminder sends do not exist for him.
  await page.goto(`/collections/${periodId}`);
  await expect(page.getByRole("tab", { name: "Theo phụ huynh" })).toBeVisible();
  await expect(page.getByRole("button", { name: "Nhắc nợ" })).toHaveCount(0);
});
