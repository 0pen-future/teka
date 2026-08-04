/// <reference lib="dom" />
import { expect, test, type Page } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
// The seeded roster gives "Chị Hoa" two children ("Bé An", "Bé Bình") and
// "Chị Mai" two ("Bé Dung", "Bé Em"). These specs target Chị Mai throughout:
// the collections spec earlier in the suite pays Chị Hoa's invoices in full,
// and a fully-paid family's statement token deliberately resolves to the
// neutral 404 — Chị Mai's family stays unpaid, so her link keeps rendering.
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

test.use({ viewport: { width: 375, height: 667 } });

async function login(page: Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();
}

/**
 * Generates the "Học phí" bulk notifications for the current period and
 * returns the public statement URL embedded in each contact's rendered
 * message text, keyed by contact name. The real message template embeds the
 * `/s/:token` link as plain text — this is how a parent actually receives it
 * (a Zalo message), so reading it off the rendered card is the same path a
 * parent's link takes, rather than reaching into the API response directly.
 */
async function collectStatementUrls(page: Page): Promise<Record<string, string>> {
  await page.goto("/billing");
  await expect(page).toHaveURL(/\/billing\/.+$/);
  const periodId = page.url().split("/billing/")[1];

  await page.goto(`/notifications/${periodId}`);
  const generateButton = page.getByRole("button", { name: /Tạo (lại|thông báo học phí)/ });
  await generateButton.click();

  const cards = page.locator('[class*="rounded-\\[var\\(--radius-lg\\)\\]"]').filter({
    has: page.getByRole("button", { name: "Sao chép" }),
  });
  await expect(cards.first()).toBeVisible();

  const count = await cards.count();
  const urlsByContact: Record<string, string> = {};
  for (let i = 0; i < count; i += 1) {
    const card = cards.nth(i);
    const text = await card.innerText();
    const match = /https?:\/\/\S+\/s\/\S+|\/s\/\S+/.exec(text);
    if (!match) {
      continue;
    }
    // The card's first text line is the avatar initial, not the name — read
    // the contact name out of the message template's greeting instead.
    const contactName = /Chào anh\/chị (.+?),/.exec(text)?.[1]?.trim() ?? "";
    urlsByContact[contactName] = match[0].replace(/[.,;]+$/, "");
  }
  return urlsByContact;
}

test("a valid token renders the family total, and an invalid token renders the neutral error", async ({
  page,
  context,
}) => {
  await login(page);
  const urls = await collectStatementUrls(page);
  // A missing URL is a data regression (seed or billing flow broke), not an
  // environment limitation — fail loudly instead of silently skipping.
  const statementUrl = urls["Chị Mai"];
  expect(statementUrl, "Chị Mai's statement URL missing from generated notifications").toBeTruthy();

  // A fresh, cookie-less context stands in for a parent who never logged in.
  const parentPage = await context
    .browser()!
    .newContext()
    .then((c) => c.newPage());
  await parentPage.goto(statementUrl);
  await expect(parentPage.getByText("TỔNG CỘNG CẢ GIA ĐÌNH")).toBeVisible();
  // QR renders when a bank is configured; otherwise the textual fallback
  // does — either way, no broken image.
  const qrImage = parentPage.getByRole("img", { name: /Mã QR/ });
  const qrFallback = parentPage.getByText("Chưa có mã QR chuyển khoản");
  await expect(qrImage.or(qrFallback).first()).toBeVisible();

  await parentPage.goto("/s/not-a-real-token");
  await expect(parentPage.getByText("Không mở được liên kết này.")).toBeVisible();
  await expect(parentPage.getByText(/[0-9]{3}/)).toHaveCount(0);

  await parentPage.close();
});

test("a two-child family's statement shows both children's names", async ({ page, context }) => {
  await login(page);
  const urls = await collectStatementUrls(page);
  const statementUrl = urls["Chị Mai"];
  expect(statementUrl, "Chị Mai's statement URL missing from generated notifications").toBeTruthy();

  const parentPage = await context
    .browser()!
    .newContext()
    .then((c) => c.newPage());
  await parentPage.goto(statementUrl);
  await expect(parentPage.getByText("Bé Dung")).toBeVisible();
  await expect(parentPage.getByText("Bé Em")).toBeVisible();
  await parentPage.close();
});

test("nothing overflows horizontally at 320px width", async ({ page, context }) => {
  await login(page);
  const urls = await collectStatementUrls(page);
  const statementUrl = urls["Chị Mai"];
  expect(statementUrl, "Chị Mai's statement URL missing from generated notifications").toBeTruthy();

  const parentPage = await context
    .browser()!
    .newContext()
    .then((c) => c.newPage());
  await parentPage.setViewportSize({ width: 320, height: 640 });
  await parentPage.goto(statementUrl);
  await expect(parentPage.getByText("TỔNG CỘNG CẢ GIA ĐÌNH")).toBeVisible();

  const overflow = await parentPage.evaluate(
    () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
  );
  expect(overflow).toBe(true);

  await parentPage.close();
});
