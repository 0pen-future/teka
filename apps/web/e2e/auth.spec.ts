import { expect, test, type Page, type Request } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

/** Collects every request whose URL contains `fragment` for the page's lifetime. */
function trackRequests(page: Page, fragment: string): Request[] {
  const seen: Request[] = [];
  page.on("request", (request) => {
    if (request.url().includes(fragment)) {
      seen.push(request);
    }
  });
  return seen;
}

test("visiting a protected page unauthenticated redirects to login", async ({ page }) => {
  await page.goto("/");
  await expect(page).toHaveURL(/\/login$/);
});

test("rejected credentials show the server message on the form", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill("definitely-wrong-password");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText("invalid phone or password")).toBeVisible();
  await expect(page).toHaveURL(/\/login$/);
});

test("logs in, keeps the session across a reload, and logs out", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();

  // The access token lives in memory only; a reload must restore the session
  // from the httpOnly refresh cookie without bouncing to /login.
  await page.reload();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
  await expect(page).not.toHaveURL(/\/login/);

  await page.getByRole("button", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // The refresh cookie is revoked: a reload must not resurrect the session.
  await page.reload();
  await expect(page).toHaveURL(/\/login$/);
});

test("client-side phone validation blocks the login request entirely", async ({ page }) => {
  const loginCalls = trackRequests(page, "/auth/login");
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill("12345");
  await page.getByLabel("Mật khẩu").fill("whatever-password");
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText("Số điện thoại không hợp lệ")).toBeVisible();
  expect(loginCalls).toHaveLength(0);
});

test("a reload restores the session via exactly one refresh and rotates the cookie", async ({
  page,
  context,
}) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();

  const cookieBefore = (await context.cookies()).find((c) => c.name === "refresh_token");
  expect(cookieBefore).toBeDefined();

  const refreshCalls = trackRequests(page, "/auth/refresh");
  await page.reload();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();
  expect(refreshCalls).toHaveLength(1);
  expect((await refreshCalls[0].response())?.status()).toBe(200);

  // Every refresh rotates the token within its family; a stable value would
  // mean rotation silently stopped working.
  const cookieAfter = (await context.cookies()).find((c) => c.name === "refresh_token");
  expect(cookieAfter?.value).not.toBe(cookieBefore?.value);
});

test("after logout the next load attempts refresh exactly once and gives up", async ({ page }) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào buổi (sáng|trưa|chiều|tối), Cô Lan!/)).toBeVisible();

  await page.getByRole("button", { name: "Đăng xuất" }).click();
  await expect(page).toHaveURL(/\/login$/);

  // The revoked family must yield exactly one failed refresh on the next
  // load — more than one would mean the dead-session gate loops.
  const refreshCalls = trackRequests(page, "/auth/refresh");
  await page.reload();
  await expect(page.getByRole("button", { name: "Đăng nhập" })).toBeVisible();
  expect(refreshCalls).toHaveLength(1);
  expect((await refreshCalls[0].response())?.status()).toBe(401);
});

test("the public statement route never attempts a session refresh", async ({ page }) => {
  const refreshCalls = trackRequests(page, "/auth/refresh");
  await page.goto("/s/unknown-token");
  // The page settles into the statement feature's own neutral error state…
  await expect(page.getByText(/không tìm thấy|không hợp lệ|hết hạn|không đúng/i)).toBeVisible();
  // …without ever pinging the auth stack.
  expect(refreshCalls).toHaveLength(0);
});

test("a logged-in teacher with pending sessions sees the attendance alert on /", async ({
  page,
}) => {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();

  // The seeder leaves its most recent past sessions unconfirmed
  // (`apps/api/seeds/seed.go`, `pendingAttendanceCount`). How many of them the
  // dashboard still counts depends on how far the run date has drifted from the
  // seeded timeline, so the count stays out of the matcher.
  await expect(page.getByText(/buổi đã dạy nhưng chưa điểm danh/)).toBeVisible();
  await expect(page.getByRole("link", { name: "Điểm danh ngay" }).first()).toBeVisible();
});
