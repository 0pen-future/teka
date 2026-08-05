import { expect, test } from "@playwright/test";

// Dev-only credentials created by the API seeder (`apps/api` seed command).
const TEACHER_PHONE = "0901000001";
const TEACHER_PASSWORD = "lan-password";

async function login(page: import("@playwright/test").Page) {
  await page.goto("/login");
  await page.getByLabel("Số điện thoại").fill(TEACHER_PHONE);
  await page.getByLabel("Mật khẩu").fill(TEACHER_PASSWORD);
  await page.getByRole("button", { name: "Đăng nhập" }).click();
  await expect(page.getByText(/Chào Cô Lan/)).toBeVisible();
}

test("roster flow: contact, two students, a class, enroll both, end one", async ({ page }) => {
  // Timestamp suffix keeps this run's data unique even though the config
  // runs specs sequentially against one seeded stack (`playwright.config.ts`).
  const suffix = Date.now();
  const contactName = `E2E Phụ huynh ${suffix}`;
  const contactPhone = `09${String(suffix).slice(-8)}`;
  const studentOneName = `E2E Học sinh Một ${suffix}`;
  const studentTwoName = `E2E Học sinh Hai ${suffix}`;
  const studentThreeName = `E2E Học sinh Ba ${suffix}`;
  const className = `E2E Lớp ${suffix}`;

  await login(page);

  // 1. Create the contact (phone owner — PRD R1: phone lives on the contact).
  await page.goto("/contacts");
  await page.getByRole("button", { name: "Thêm người liên hệ" }).click();
  await page.getByLabel("Họ và tên").fill(contactName);
  await page.getByLabel("Số điện thoại").fill(contactPhone);
  await page.getByRole("button", { name: "Lưu" }).click();
  await expect(page.getByText(contactName)).toBeVisible();
  await page.getByText(contactName).click();
  await expect(page).toHaveURL(/\/contacts\/.+/);

  // 2. Add two students under that contact (closed field list: name + note only).
  await page.getByRole("button", { name: "Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentOneName);
  await page.getByRole("dialog").getByRole("button", { name: "Thêm học sinh" }).click();
  await expect(page.getByText(studentOneName)).toBeVisible();

  await page.getByRole("button", { name: "Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentTwoName);
  await page.getByRole("dialog").getByRole("button", { name: "Thêm học sinh" }).click();
  await expect(page.getByText(studentTwoName)).toBeVisible();

  // 3. Create a class with one weekly slot (`ClassDialog` create mode).
  await page.goto("/classes");
  await page.getByRole("button", { name: "+ Tạo lớp mới" }).click();
  await page.getByLabel("Tên lớp").fill(className);
  await page.getByLabel("Giờ học").fill("18:00");
  await page.getByLabel("Thời lượng (phút)").fill("90");
  await page.getByLabel("Đơn giá / buổi (đ)").fill("150000");
  await page.getByRole("dialog").getByRole("button", { name: "Tạo lớp" }).click();
  await expect(page.getByText(className)).toBeVisible();
  await page.getByText(className).click();
  await expect(page).toHaveURL(/\/classes\/.+/);

  // 3b. Add-student wizard on the roster screen: create the profile (Bước
  // 1/2), postpone enrollment with "Để sau", find the student on the "Chưa
  // ghi danh" tab, then enroll from there (Bước 2 reused standalone).
  await page.goto("/students");
  await page.getByRole("button", { name: "+ Thêm học sinh" }).click();
  await page.getByLabel("Họ và tên").fill(studentThreeName);
  await page.getByRole("combobox", { name: "Người liên hệ" }).fill(contactName);
  await page.getByRole("option", { name: new RegExp(contactName) }).click();
  await page.getByRole("button", { name: "Tiếp tục: Ghi danh →" }).click();
  await expect(page.getByText("Bước 2/2")).toBeVisible();
  await page.getByRole("button", { name: "Để sau" }).click();
  await expect(page.getByText(/Đã lưu hồ sơ — ghi danh sau/)).toBeVisible();
  await expect(page).toHaveURL(/class_id=none/);

  await page.getByPlaceholder("Tìm theo tên học sinh").fill(studentThreeName);
  const studentThreeRow = page.getByRole("row").filter({ hasText: studentThreeName });
  await expect(studentThreeRow.getByText("Chưa vào lớp nào")).toBeVisible();
  await studentThreeRow.getByRole("button", { name: "Ghi danh vào lớp" }).click();
  await page.getByRole("combobox", { name: "Lớp" }).click();
  await page.getByRole("option", { name: new RegExp(className) }).click();
  await page.getByRole("dialog").getByRole("button", { name: "Ghi danh vào lớp" }).click();
  await expect(page.getByText(/tính tiền từ buổi có mặt đầu tiên/)).toBeVisible();

  // Enrolling moves the roster to that class's tab; the student is on it.
  await expect(page).toHaveURL(/class_id=(?!none)/);
  await expect(page.getByRole("row").filter({ hasText: studentThreeName })).toBeVisible();

  // 4. Enroll both remaining students into the class from the class screen.
  await page.goto("/classes");
  await page.getByText(className).click();
  await expect(page).toHaveURL(/\/classes\/.+/);
  await page.getByRole("button", { name: "Ghi danh học sinh" }).click();
  await page.getByLabel("Tìm học sinh").fill(studentOneName);
  await page.getByRole("option", { name: new RegExp(studentOneName) }).click();
  await page.getByRole("button", { name: "Ghi danh" }).click();
  await expect(page.getByText(/được tính từ buổi học tiếp theo/)).toBeVisible();

  await page.getByRole("button", { name: "Ghi danh học sinh" }).click();
  await page.getByLabel("Tìm học sinh").fill(studentTwoName);
  await page.getByRole("option", { name: new RegExp(studentTwoName) }).click();
  await page.getByRole("button", { name: "Ghi danh" }).click();
  await expect(page.getByText(/được tính từ buổi học tiếp theo/)).toBeVisible();

  // The enrollment row is the card two levels above the name: the name <p>
  // sits inside a text-column <div>, and the row's action button is that
  // div's sibling — one `..` would stop at the text column and miss it.
  const studentOneRow = page.getByText(studentOneName).locator("xpath=../..");
  await expect(studentOneRow.getByRole("button", { name: "Kết thúc ghi danh" })).toBeVisible();
  const studentTwoRow = page.getByText(studentTwoName).locator("xpath=../..");
  await expect(studentTwoRow.getByRole("button", { name: "Kết thúc ghi danh" })).toBeVisible();

  // 5. End one enrollment — existing debt (if any) must be preserved, not written off.
  await studentOneRow.getByRole("button", { name: "Kết thúc ghi danh" }).click();
  await expect(page.getByText("Nợ cũ (nếu có) vẫn được giữ.")).toBeVisible();
  await page.getByRole("button", { name: "Kết thúc" }).click();
  await expect(studentOneRow.getByRole("button", { name: "Kết thúc ghi danh" })).toHaveCount(0);
  await expect(studentTwoRow.getByRole("button", { name: "Kết thúc ghi danh" })).toBeVisible();
});
